package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type YouTubeVideoItem struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Uploader       string `json:"uploader"`
	ChannelURL     string `json:"channelUrl,omitempty"`
	DurationString string `json:"durationString"`
	DurationSec    int64  `json:"durationSec"`
	ViewCount      int64  `json:"viewCount"`
	Thumbnail      string `json:"thumbnail"`
	URL            string `json:"url"`
	IsLive         bool   `json:"isLive"`
}

type ytDlpFlatEntry struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Uploader       string `json:"uploader"`
	Channel        string `json:"channel"`
	ChannelURL     string `json:"channel_url"`
	Duration       int64  `json:"duration"`
	DurationString string `json:"duration_string"`
	ViewCount      int64  `json:"view_count"`
	WebpageURL     string `json:"webpage_url"`
	IsLive         bool   `json:"is_live"`
	Thumbnails     []struct {
		URL    string `json:"url"`
		Height int    `json:"height"`
		Width  int    `json:"width"`
	} `json:"thumbnails"`
}

var (
	ytSearchCacheMu sync.RWMutex
	ytSearchCache   = make(map[string]ytCacheEntry)
)

type ytCacheEntry struct {
	Items     []YouTubeVideoItem
	ExpiresAt time.Time
}

// YouTubeSearchHandler handles GET /api/youtube/search?q=...&limit=...
func YouTubeSearchHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		query = "trending music"
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 16
	if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 30 {
		limit = n
	}

	cacheKey := fmt.Sprintf("%s:%d", strings.ToLower(query), limit)
	ytSearchCacheMu.RLock()
	if cached, ok := ytSearchCache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
		ytSearchCacheMu.RUnlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"query":  query,
			"cached": true,
			"items":  cached.Items,
		})
		return
	}
	ytSearchCacheMu.RUnlock()

	items, err := executeYouTubeSearch(r.Context(), query, limit)
	if err != nil {
		log.Warn().Err(err).Str("query", query).Msg("YouTube search failed")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"query":  query,
			"error":  err.Error(),
			"items":  []YouTubeVideoItem{},
		})
		return
	}

	// Cache successful result for 15 minutes
	ytSearchCacheMu.Lock()
	ytSearchCache[cacheKey] = ytCacheEntry{
		Items:     items,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	ytSearchCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"query":  query,
		"cached": false,
		"items":  items,
	})
}

func executeYouTubeSearch(ctx context.Context, query string, limit int) ([]YouTubeVideoItem, error) {
	searchTarget := fmt.Sprintf("ytsearch%d:%s", limit, query)

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, "yt-dlp",
		searchTarget,
		"--dump-json",
		"--flat-playlist",
		"--no-playlist",
		"--skip-download",
		"--no-warnings",
		"--no-check-certificate",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("yt-dlp search error: %v, stderr: %s", err, stderr.String())
	}

	var items []YouTubeVideoItem
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var raw ytDlpFlatEntry
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if raw.ID == "" {
			continue
		}

		uploader := raw.Uploader
		if uploader == "" {
			uploader = raw.Channel
		}
		if uploader == "" {
			uploader = "YouTube Creator"
		}

		// Pick thumbnail
		thumb := fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", raw.ID)
		if len(raw.Thumbnails) > 0 {
			for i := len(raw.Thumbnails) - 1; i >= 0; i-- {
				if raw.Thumbnails[i].URL != "" {
					thumb = raw.Thumbnails[i].URL
					break
				}
			}
		}

		durStr := raw.DurationString
		if durStr == "" && raw.Duration > 0 {
			durStr = formatDurationSec(raw.Duration)
		}
		if raw.IsLive {
			durStr = "LIVE"
		}

		targetURL := raw.WebpageURL
		if targetURL == "" {
			targetURL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", raw.ID)
		}

		items = append(items, YouTubeVideoItem{
			ID:             raw.ID,
			Title:          raw.Title,
			Uploader:       uploader,
			ChannelURL:     raw.ChannelURL,
			DurationString: durStr,
			DurationSec:    raw.Duration,
			ViewCount:      raw.ViewCount,
			Thumbnail:      thumb,
			URL:            targetURL,
			IsLive:         raw.IsLive,
		})
	}

	return items, nil
}

func formatDurationSec(sec int64) string {
	if sec <= 0 {
		return "0:00"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
