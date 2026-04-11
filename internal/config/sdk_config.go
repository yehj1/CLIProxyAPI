// Package config provides configuration management for the CLI Proxy API server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

import "strings"

// SDKConfig represents the application's configuration, loaded from a YAML file.
type SDKConfig struct {
	// ProxyURL is the URL of an optional proxy server to use for outbound requests.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// ForceModelPrefix requires explicit model prefixes (e.g., "teamA/gemini-3-pro-preview")
	// to target prefixed credentials. When false, unprefixed model requests may use prefixed
	// credentials as well.
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog enables or disables detailed request logging functionality.
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// APIKeys is a list of keys for authenticating clients to this proxy server.
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// APIKeyEntries provides per-key metadata such as expiry and daily token limits.
	APIKeyEntries []APIKeyEntry `yaml:"api-key-entries,omitempty" json:"api-key-entries,omitempty"`

	// CreditPerMillionTokens sets the conversion rate from tokens to credits.
	// Example: 6 means every 1,000,000 tokens costs 6 credits (rounded up per request).
	CreditPerMillionTokens int64 `yaml:"credit-per-million-tokens,omitempty" json:"credit-per-million-tokens,omitempty"`

	// PassthroughHeaders controls whether upstream response headers are forwarded to downstream clients.
	// Default is false (disabled).
	PassthroughHeaders bool `yaml:"passthrough-headers" json:"passthrough-headers"`

	// Streaming configures server-side streaming behavior (keep-alives and safe bootstrap retries).
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// NonStreamKeepAliveInterval controls how often blank lines are emitted for non-streaming responses.
	// <= 0 disables keep-alives. Value is in seconds.
	NonStreamKeepAliveInterval int `yaml:"nonstream-keepalive-interval,omitempty" json:"nonstream-keepalive-interval,omitempty"`
}

// APIKeyEntry defines metadata for a client API key.
type APIKeyEntry struct {
	APIKey          string `yaml:"api-key" json:"api-key"`
	ExpiresAt       string `yaml:"expires-at,omitempty" json:"expires-at,omitempty"`
	DailyTokenLimit int64  `yaml:"daily-token-limit,omitempty" json:"daily-token-limit,omitempty"`
	DailyCreditLimit int64 `yaml:"daily-credit-limit,omitempty" json:"daily-credit-limit,omitempty"`
}

// BuildAPIKeyEntries merges legacy api-keys with api-key-entries, trimming and de-duplicating.
// Explicit entries take precedence over legacy keys.
func BuildAPIKeyEntries(keys []string, entries []APIKeyEntry) []APIKeyEntry {
	result := make([]APIKeyEntry, 0, len(keys)+len(entries))
	index := make(map[string]int, len(keys)+len(entries))

	add := func(entry APIKeyEntry, allowReplace bool) {
		key := strings.TrimSpace(entry.APIKey)
		if key == "" {
			return
		}
		entry.APIKey = key
		entry.ExpiresAt = strings.TrimSpace(entry.ExpiresAt)
		if entry.DailyTokenLimit < 0 {
			entry.DailyTokenLimit = 0
		}
		if entry.DailyCreditLimit < 0 {
			entry.DailyCreditLimit = 0
		}
		if idx, exists := index[key]; exists {
			if allowReplace {
				result[idx] = entry
			}
			return
		}
		index[key] = len(result)
		result = append(result, entry)
	}

	for _, key := range keys {
		add(APIKeyEntry{APIKey: key}, false)
	}
	for _, entry := range entries {
		add(entry, true)
	}

	return result
}

// StreamingConfig holds server streaming behavior configuration.
type StreamingConfig struct {
	// KeepAliveSeconds controls how often the server emits SSE heartbeats (": keep-alive\n\n").
	// <= 0 disables keep-alives. Default is 0.
	KeepAliveSeconds int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries controls how many times the server may retry a streaming request before any bytes are sent,
	// to allow auth rotation / transient recovery.
	// <= 0 disables bootstrap retries. Default is 0.
	BootstrapRetries int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}
