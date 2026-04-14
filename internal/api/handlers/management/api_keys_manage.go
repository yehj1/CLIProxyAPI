package management

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/limits"
)

type apiKeyPlan struct {
	Name            string
	DailyTokenLimit int64
	ExpiryFn        func(base time.Time) time.Time
}

var apiKeyPlans = map[string]apiKeyPlan{
	"day": {
		Name:            "day",
		DailyTokenLimit: 1000,
		ExpiryFn: func(base time.Time) time.Time {
			return base.AddDate(0, 0, 2)
		},
	},
	"week": {
		Name:            "week",
		DailyTokenLimit: 2000,
		ExpiryFn: func(base time.Time) time.Time {
			return base.AddDate(0, 0, 8)
		},
	},
	"month": {
		Name:            "month",
		DailyTokenLimit: 2000,
		ExpiryFn: func(base time.Time) time.Time {
			return base.AddDate(0, 1, 1)
		},
	},
}

// ListAPIKeys returns API keys with optional token filtering.
func (h *Handler) ListAPIKeys(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not available"})
		return
	}
	filter := strings.TrimSpace(c.Query("token"))
	filterLower := strings.ToLower(filter)
	entries := config.BuildAPIKeyEntries(h.cfg.APIKeys, h.cfg.APIKeyEntries)
	loc := limits.DailyTokenLimitLocation()
	now := time.Now().In(loc)
	out := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		if entry.APIKey == "" {
			continue
		}
		if filterLower != "" && !strings.Contains(strings.ToLower(entry.APIKey), filterLower) {
			continue
		}
		out = append(out, buildAPIKeyUsageResponse(entry, now, loc))
	}
	c.JSON(http.StatusOK, gin.H{
		"count":    len(out),
		"api-keys": out,
	})
}

// CreateAPIKey creates a new API key with predefined plan defaults.
func (h *Handler) CreateAPIKey(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not available"})
		return
	}

	var body struct {
		Plan   string  `json:"plan"`
		APIKey *string `json:"api-key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	planName := strings.ToLower(strings.TrimSpace(body.Plan))
	plan, ok := apiKeyPlans[planName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan"})
		return
	}

	key := ""
	if body.APIKey != nil {
		key = strings.TrimSpace(*body.APIKey)
	}
	if key == "" {
		key = generateAPIKey()
	}
	if key == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate api key"})
		return
	}

	existing := config.BuildAPIKeyEntries(h.cfg.APIKeys, h.cfg.APIKeyEntries)
	for _, entry := range existing {
		if entry.APIKey == key {
			c.JSON(http.StatusConflict, gin.H{"error": "api key exists"})
			return
		}
	}

	loc := limits.DailyTokenLimitLocation()
	now := time.Now().In(loc)
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	expiresAt := plan.ExpiryFn(base).Format("2006-01-02 15:04:05")

	newEntry := config.APIKeyEntry{
		APIKey:          key,
		DailyTokenLimit: plan.DailyTokenLimit,
		ExpiresAt:       expiresAt,
	}

	h.cfg.APIKeys = append(h.cfg.APIKeys, key)
	h.cfg.APIKeyEntries = append(h.cfg.APIKeyEntries, newEntry)

	h.mu.Lock()
	err := config.SaveConfigPreserveComments(h.configFilePath, h.cfg)
	h.mu.Unlock()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write_failed", "message": "failed to write config"})
		return
	}
	item := buildAPIKeyUsageResponse(newEntry, now, loc)
	item["plan"] = plan.Name
	c.JSON(http.StatusOK, item)
}

func generateAPIKey() string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return "key-" + hex.EncodeToString(buf[:])
}

// DeleteAPIKey removes an API key from config.
func (h *Handler) DeleteAPIKey(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not available"})
		return
	}
	key := strings.TrimSpace(c.Query("api-key"))
	if key == "" {
		key = strings.TrimSpace(c.Query("token"))
	}
	if key == "" {
		key = strings.TrimSpace(c.Query("key"))
	}
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing api key"})
		return
	}

	removed := false
	if len(h.cfg.APIKeys) > 0 {
		newKeys := make([]string, 0, len(h.cfg.APIKeys))
		for _, existing := range h.cfg.APIKeys {
			if strings.TrimSpace(existing) == key {
				removed = true
				continue
			}
			newKeys = append(newKeys, existing)
		}
		h.cfg.APIKeys = newKeys
	}
	if len(h.cfg.APIKeyEntries) > 0 {
		newEntries := make([]config.APIKeyEntry, 0, len(h.cfg.APIKeyEntries))
		for _, entry := range h.cfg.APIKeyEntries {
			if strings.TrimSpace(entry.APIKey) == key {
				removed = true
				continue
			}
			newEntries = append(newEntries, entry)
		}
		h.cfg.APIKeyEntries = newEntries
	}

	if !removed {
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
		return
	}

	h.mu.Lock()
	err := config.SaveConfigPreserveComments(h.configFilePath, h.cfg)
	h.mu.Unlock()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write_failed", "message": "failed to write config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "api-key": key})
}

func buildAPIKeyUsageResponse(entry config.APIKeyEntry, now time.Time, loc *time.Location) gin.H {
	usedTokens := limits.GetDailyTokenLimiter().TokensUsed(entry.APIKey, now)
	usedCredits := limits.GetDailyCreditLimiter().CreditsUsed(entry.APIKey, now)
	creditMode := limits.CreditPerMillionTokens() > 0
	dailyCreditLimit := entry.DailyCreditLimit
	if creditMode && dailyCreditLimit <= 0 && entry.DailyTokenLimit > 0 {
		dailyCreditLimit = entry.DailyTokenLimit
	}
	limitEnabled := entry.DailyTokenLimit > 0
	remainingTokens := int64(0)
	if limitEnabled {
		remainingTokens = entry.DailyTokenLimit - usedTokens
		if remainingTokens < 0 {
			remainingTokens = 0
		}
	}
	displayLimit := entry.DailyTokenLimit
	displayUsed := usedTokens
	displayRemaining := remainingTokens
	if creditMode {
		displayLimit = dailyCreditLimit
		displayUsed = usedCredits
		displayRemaining = remainingCredits(dailyCreditLimit, usedCredits)
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

	return gin.H{
		"api-key":                   entry.APIKey,
		"token":                     entry.APIKey,
		"used-tokens-today":         displayUsed,
		"daily-token-limit":         displayLimit,
		"limit-enabled":             limitEnabled,
		"remaining-tokens-today":    displayRemaining,
		"daily-credit-limit":        dailyCreditLimit,
		"used-credits-today":        usedCredits,
		"remaining-credits-today":   remainingCredits(dailyCreditLimit, usedCredits),
		"credit-per-million-tokens": limits.CreditPerMillionTokens(),
		"credit-unit-tokens":        limits.CreditUnitTokens(),
		"credit-mode":               creditMode,
		"expires-at":                expiresAtRaw,
		"expires-at-parsed":         expiresAtParsed,
		"expired":                   expired,
		"timezone":                  loc.String(),
		"date":                      now.Format("2006-01-02"),
		"next-reset-at":             nextReset.Format(time.RFC3339),
		"seconds-until-reset":       secondsUntilReset,
	}
}
