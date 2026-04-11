package management

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/limits"
)

// GetAPIKeyUsage returns today's token usage and expiry for a specific API key.
func (h *Handler) GetAPIKeyUsage(c *gin.Context) {
	apiKey := strings.TrimSpace(c.Query("api-key"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(c.Query("token"))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(c.Query("key"))
	}
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_api_key"})
		return
	}
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api_key_not_found"})
		return
	}

	entries := config.BuildAPIKeyEntries(h.cfg.APIKeys, h.cfg.APIKeyEntries)
	var entry *config.APIKeyEntry
	for i := range entries {
		if entries[i].APIKey == apiKey {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api_key_not_found"})
		return
	}

	loc := limits.DailyTokenLimitLocation()
	now := time.Now().In(loc)
	used := limits.GetDailyTokenLimiter().TokensUsed(apiKey, now)
	limitEnabled := entry.DailyTokenLimit > 0
	remaining := int64(0)
	if limitEnabled {
		remaining = entry.DailyTokenLimit - used
		if remaining < 0 {
			remaining = 0
		}
	}

	expiresAtRaw := strings.TrimSpace(entry.ExpiresAt)
	expired := false
	expiresAtParsed := ""
	if expiresAtRaw != "" {
		if exp, ok := parseExpiresAtInLocation(expiresAtRaw, loc); ok {
			expired = now.After(exp)
			expiresAtParsed = exp.In(loc).Format(time.RFC3339)
		}
	}

	nextReset := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)
	secondsUntilReset := int64(nextReset.Sub(now).Seconds())
	if secondsUntilReset < 0 {
		secondsUntilReset = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"api-key":                 apiKey,
		"used-tokens-today":       used,
		"daily-token-limit":       entry.DailyTokenLimit,
		"limit-enabled":           limitEnabled,
		"remaining-tokens-today":  remaining,
		"expires-at":              expiresAtRaw,
		"expires-at-parsed":       expiresAtParsed,
		"expired":                 expired,
		"timezone":                loc.String(),
		"date":                    now.Format("2006-01-02"),
		"next-reset-at":           nextReset.Format(time.RFC3339),
		"seconds-until-reset":     secondsUntilReset,
	})
}

func parseExpiresAtInLocation(value string, loc *time.Location) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	if loc == nil {
		loc = time.Local
	}
	for _, format := range formats {
		if t, err := time.ParseInLocation(format, value, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
