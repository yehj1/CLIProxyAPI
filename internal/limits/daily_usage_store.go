package limits

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type dailyUsageSnapshot struct {
	Tokens  map[string]map[string]int64 `json:"tokens,omitempty"`
	Credits map[string]map[string]int64 `json:"credits,omitempty"`
}

// UsageStorePath returns the default path for persisting daily usage data.
func UsageStorePath(authDir string) string {
	if authDir == "" {
		return ""
	}
	return filepath.Join(authDir, "usage", "usage.json")
}

// LoadDailyUsage restores persisted daily usage data into memory.
func LoadDailyUsage(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var snapshot dailyUsageSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	GetDailyTokenLimiter().ReplaceTotals(snapshot.Tokens)
	GetDailyCreditLimiter().ReplaceTotals(snapshot.Credits)
	return nil
}

// SaveDailyUsage persists in-memory daily usage data to disk.
func SaveDailyUsage(path string) error {
	if path == "" {
		return nil
	}
	snapshot := dailyUsageSnapshot{
		Tokens:  GetDailyTokenLimiter().Snapshot(),
		Credits: GetDailyCreditLimiter().Snapshot(),
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "usage-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpName, path)
}
