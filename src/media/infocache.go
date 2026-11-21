package media

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kv-download/src/utils"

	"github.com/rs/zerolog/log"
)

var (
	infoCacheDir string
	cacheOnce    sync.Once
	cacheMu      sync.RWMutex
)

// GetInfoCacheDir returns and ensures the directory for cached yt-dlp info JSON files.
func GetInfoCacheDir() string {
	cacheOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), "kv-info-cache")
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Error().Err(err).Msgf("Failed to create info cache dir: %s", dir)
		}
		infoCacheDir = dir
	})
	return infoCacheDir
}

// InfoCachePath returns the deterministic cache file path for a given URL.
func InfoCachePath(rawURL string) string {
	clean := utils.NormalizeUrl(strings.TrimSpace(rawURL))
	hash := fmt.Sprintf("%x", md5.Sum([]byte(clean)))
	return filepath.Join(GetInfoCacheDir(), fmt.Sprintf("info-%s.json", hash))
}

// SaveCachedInfoJSON validates and atomically stores yt-dlp raw info JSON on disk.
func SaveCachedInfoJSON(rawURL string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty info-json data")
	}

	// Quick sanity check to ensure it's valid JSON
	if !json.Valid(data) {
		return "", fmt.Errorf("invalid json payload for info-json cache")
	}

	targetPath := InfoCachePath(rawURL)
	tmpPath := fmt.Sprintf("%s.%d.tmp", targetPath, time.Now().UnixNano())

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write tmp info-json: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to commit info-json cache: %w", err)
	}

	return targetPath, nil
}

// GetCachedInfoJSON retrieves the cached info JSON file path if it exists and is still fresh.
// Max TTL is 4 hours to ensure pre-signed CDN tokens don't expire mid-download.
func GetCachedInfoJSON(rawURL string) (string, bool) {
	targetPath := InfoCachePath(rawURL)

	cacheMu.RLock()
	defer cacheMu.RUnlock()

	fi, err := os.Stat(targetPath)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return "", false
	}

	// Verify freshness (< 4 hours)
	if time.Since(fi.ModTime()) > 4*time.Hour {
		go func(p string) {
			cacheMu.Lock()
			_ = os.Remove(p)
			cacheMu.Unlock()
		}(targetPath)
		return "", false
	}

	return targetPath, true
}

// InvalidateCachedInfoJSON deletes the cached info JSON file for a URL (e.g. after a download error).
func InvalidateCachedInfoJSON(rawURL string) {
	targetPath := InfoCachePath(rawURL)
	cacheMu.Lock()
	defer cacheMu.Unlock()
	_ = os.Remove(targetPath)
}

// PruneExpiredInfoCache removes cached JSON files older than the specified duration.
func PruneExpiredInfoCache(maxAge time.Duration) {
	dir := GetInfoCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	now := time.Now()
	removedCount := 0

	cacheMu.Lock()
	defer cacheMu.Unlock()

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		fi, err := e.Info()
		if err == nil && now.Sub(fi.ModTime()) > maxAge {
			if err := os.Remove(path); err == nil {
				removedCount++
			}
		}
	}

	if removedCount > 0 {
		log.Debug().Msgf("[InfoCache] Pruned %d expired metadata cache files", removedCount)
	}
}

// StartInfoCachePruner starts a background ticker to clean up stale info JSON cache files.
func StartInfoCachePruner() {
	PruneExpiredInfoCache(6 * time.Hour)
	ticker := time.NewTicker(2 * time.Hour)
	go func() {
		for range ticker.C {
			PruneExpiredInfoCache(6 * time.Hour)
		}
	}()
}
