package management

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

type usageExportPayload struct {
	Version    int                      `json:"version"`
	ExportedAt time.Time                `json:"exported_at"`
	Usage      usage.StatisticsSnapshot `json:"usage"`
}

type usageImportPayload struct {
	Version int                      `json:"version"`
	Usage   usage.StatisticsSnapshot `json:"usage"`
}

// GetUsageStatistics returns the in-memory request statistics snapshot.
func (h *Handler) GetUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, gin.H{
		"usage":           snapshot,
		"failed_requests": snapshot.FailureCount,
	})
}

// ExportUsageStatistics returns a complete usage snapshot for backup/migration.
func (h *Handler) ExportUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, usageExportPayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	})
}

// ImportUsageStatistics merges a previously exported usage snapshot into memory.
func (h *Handler) ImportUsageStatistics(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var payload usageImportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if payload.Version != 0 && payload.Version != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
		return
	}

	result := h.usageStats.MergeSnapshot(payload.Usage)
	snapshot := h.usageStats.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"added":           result.Added,
		"skipped":         result.Skipped,
		"total_requests":  snapshot.TotalRequests,
		"failed_requests": snapshot.FailureCount,
	})
}

// GetUsageDetailsPublic returns request details for a specific API key (public allowlist).
func (h *Handler) GetUsageDetailsPublic(c *gin.Context) {
	if h == nil || h.cfg == nil || h.usageStats == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usage statistics unavailable"})
		return
	}

	accessKey := strings.TrimSpace(bearerToken(c.GetHeader("Authorization")))
	if accessKey == "" {
		accessKey = strings.TrimSpace(c.Query("access-key"))
	}
	if accessKey == "" {
		accessKey = strings.TrimSpace(c.Query("access_key"))
	}
	if accessKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing access key"})
		return
	}
	if !h.isUsagePublicKeyAllowed(accessKey) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access key not allowed"})
		return
	}

	targetKey := strings.TrimSpace(c.Query("api-key"))
	if targetKey == "" {
		targetKey = strings.TrimSpace(c.Query("token"))
	}
	if targetKey == "" {
		targetKey = strings.TrimSpace(c.Query("key"))
	}
	if targetKey == "" {
		targetKey = accessKey
	}
	if subtle.ConstantTimeCompare([]byte(targetKey), []byte(accessKey)) != 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "access key mismatch"})
		return
	}

	limit := int64(200)
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 2000 {
		limit = 2000
	}

	order := strings.ToLower(strings.TrimSpace(c.Query("order")))
	if order == "" {
		order = "desc"
	}

	since := parseUsageTime(c.Query("since"))
	until := parseUsageTime(c.Query("until"))
	modelFilter := strings.TrimSpace(c.Query("model"))

	snapshot := h.usageStats.Snapshot()
	apiSnapshot, ok := snapshot.APIs[targetKey]
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"api-key":          targetKey,
			"total_requests":   0,
			"total_tokens":     0,
			"filtered_requests": 0,
			"filtered_tokens":   0,
			"details":          []usageDetailEntry{},
		})
		return
	}

	details := make([]usageDetailEntry, 0, limit)
	var filteredTokens int64
	for modelName, modelSnapshot := range apiSnapshot.Models {
		if modelFilter != "" && modelName != modelFilter {
			continue
		}
		for _, detail := range modelSnapshot.Details {
			if !since.IsZero() && detail.Timestamp.Before(since) {
				continue
			}
			if !until.IsZero() && detail.Timestamp.After(until) {
				continue
			}
			details = append(details, usageDetailEntry{
				Model:         modelName,
				RequestDetail: detail,
			})
		}
	}

	sort.Slice(details, func(i, j int) bool {
		if order == "asc" {
			return details[i].Timestamp.Before(details[j].Timestamp)
		}
		return details[i].Timestamp.After(details[j].Timestamp)
	})
	if int64(len(details)) > limit {
		details = details[:limit]
	}
	for i := range details {
		filteredTokens += details[i].Tokens.TotalTokens
	}

	c.JSON(http.StatusOK, gin.H{
		"api-key":           targetKey,
		"total_requests":    apiSnapshot.TotalRequests,
		"total_tokens":      apiSnapshot.TotalTokens,
		"filtered_requests": len(details),
		"filtered_tokens":   filteredTokens,
		"details":           details,
	})
}

type usageDetailEntry struct {
	Model string `json:"model"`
	usage.RequestDetail
}

func parseUsageTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		return t
	}
	return time.Time{}
}

func (h *Handler) isUsagePublicKeyAllowed(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || h == nil || h.cfg == nil {
		return false
	}
	for _, allowed := range h.cfg.UsagePublicKeys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(strings.TrimSpace(allowed))) == 1 {
			return true
		}
	}
	return false
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
