package usage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type usageSnapshotFile struct {
	Version    int                `json:"version"`
	ExportedAt time.Time          `json:"exported_at"`
	Usage      StatisticsSnapshot `json:"usage"`
}

// UsageStatsStorePath returns the default path for persisting request statistics.
func UsageStatsStorePath(authDir string) string {
	if authDir == "" {
		return ""
	}
	return filepath.Join(authDir, "usage", "requests.json")
}

// LoadUsageStatistics merges persisted request statistics into memory.
func LoadUsageStatistics(path string) error {
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
	var payload usageSnapshotFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	GetRequestStatistics().MergeSnapshot(payload.Usage)
	return nil
}

// SaveUsageStatistics persists in-memory request statistics to disk.
func SaveUsageStatistics(path string) error {
	if path == "" {
		return nil
	}
	payload := usageSnapshotFile{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      GetRequestStatistics().Snapshot(),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
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
