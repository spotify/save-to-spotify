package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

var (
	updateCheckCacheTTL     = 24 * time.Hour
	updateNoticeWaitTimeout = 500 * time.Millisecond
)

type updateCheckCache struct {
	LatestVersion string    `json:"latest_version"`
	AssetURL      string    `json:"asset_url,omitempty"`
	SHA256        string    `json:"sha256,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
}

type updateCheckResult struct {
	LatestVersion string
	Err           error
}

// StartUpdateCheck begins a background version check. Non-blocking.
func StartUpdateCheck() func() {
	if os.Getenv(config.EnvVarNoUpdateCheck) != "" {
		return func() {}
	}

	cache, err := loadUpdateCheckCache()
	if err == nil && cache != nil && time.Since(cache.CheckedAt) < updateCheckCacheTTL {
		return func() {
			printUpdateNotice(cache.LatestVersion)
		}
	}

	resultCh := make(chan updateCheckResult, 1)
	go func() {
		release, err := fetchLatestVersion()
		if err == nil {
			_ = saveUpdateCheckCache(&updateCheckCache{
				LatestVersion: release.Version,
				AssetURL:      release.AssetURL,
				SHA256:        release.SHA256,
				CheckedAt:     time.Now().UTC(),
			})
		}
		resultCh <- updateCheckResult{LatestVersion: release.Version, Err: err}
	}()

	return func() {
		select {
		case result := <-resultCh:
			if result.Err == nil {
				printUpdateNotice(result.LatestVersion)
			}
		case <-time.After(updateNoticeWaitTimeout):
		}
	}
}

func loadUpdateCheckCache() (*updateCheckCache, error) {
	path, err := updateCheckCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read update check cache: %w", err)
	}

	var cache updateCheckCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse update check cache: %w", err)
	}

	return &cache, nil
}

func saveUpdateCheckCache(cache *updateCheckCache) error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal update check cache: %w", err)
	}

	path := filepath.Join(dir, "update-check.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write update check cache: %w", err)
	}

	return nil
}

func updateCheckCachePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "update-check.json"), nil
}

func printUpdateNotice(latestVersion string) {
	if !isNewer(version, latestVersion) {
		return
	}

	fmt.Fprintf(os.Stderr, "A newer version of %s is available: %s (current: %s)\n", binName, normalizeVersion(latestVersion), version)
	fmt.Fprintf(os.Stderr, "Run `%s update` to install.\n", binName)
}
