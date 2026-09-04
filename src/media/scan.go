package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"kv-download/src/anpan"

	"github.com/rs/zerolog/log"
)

type ScanThumbnail struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type ScanEntry struct {
	Id          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Track       string          `json:"track,omitempty"`
	Url         string          `json:"url"`
	Thumbnail   string          `json:"thumbnail"`
	Thumbnails  []ScanThumbnail `json:"thumbnails,omitempty"`
	Duration    float64         `json:"duration,omitempty"`
	Uploader    string          `json:"uploader,omitempty"`
	Channel     string          `json:"channel,omitempty"`
	ViewCount   int64           `json:"view_count,omitempty"`
	Category    string          `json:"category,omitempty"`
}

type ScanInfo struct {
	Title      string      `json:"title"`
	Count      int         `json:"count"`
	TotalCount int         `json:"totalCount,omitempty"`
	IsPlaylist bool        `json:"isPlaylist"`
	Entries    []ScanEntry `json:"entries"`
	Thumbnail  string      `json:"thumbnail"`
	Channel    string      `json:"channel,omitempty"`
	Uploader   string      `json:"uploader,omitempty"`
	Start      int         `json:"start,omitempty"`
	Limit      int         `json:"limit,omitempty"`
	HasMore    bool        `json:"hasMore"`
	NextStart  int         `json:"nextStart,omitempty"`
}

type RawScanInfo struct {
	Id         string          `json:"id"`
	Type       string          `json:"_type"`
	Title      string          `json:"title"`
	Uploader   string          `json:"uploader"`
	Channel    string          `json:"channel"`
	Count      int             `json:"playlist_count"`
	Entries    []ScanEntry     `json:"entries"`
	Thumbnail  string          `json:"thumbnail"`
	Thumbnails []ScanThumbnail `json:"thumbnails"`
	Url        string          `json:"url"`
	WebpageUrl string          `json:"webpage_url"`
}

// ScanUrl inspects a URL with default pagination (batch of 24)
func ScanUrl(inputUrl string, cookies string) (*ScanInfo, string, error) {
	return ScanUrlWithPagination(inputUrl, cookies, 1, 24)
}

// ScanUrlWithPagination inspects a URL with specified start and limit batch
func ScanUrlWithPagination(inputUrl string, cookies string, start int, limit int) (*ScanInfo, string, error) {
	if start <= 0 {
		start = 1
	}
	if limit <= 0 {
		limit = 24
	}
	url := strings.TrimSpace(inputUrl)
	if url == "" {
		return nil, "", errors.New("missing URL")
	}

	if IsFacebookShareURL(url) {
		// Share links need to be resolved to the actual profile URL first
		profileHandle, normalizedURL := extractFbProfileFromShareURL(url, cookies)
		if profileHandle != "Facebook" && normalizedURL != "" {
			log.Info().Msgf("Resolved Facebook share link: %s -> profile=%s, URL=%s", url, profileHandle, normalizedURL)
			return ScrapeFacebookVideosFromNormalized(normalizedURL, cookies, profileHandle, start, limit)
		}
		// If resolution failed, try the original URL as a fallback
		return ScrapeFacebookVideos(url, cookies, start, limit)
	}

	if IsFacebookProfileURL(url) {
		return ScrapeFacebookVideos(url, cookies, start, limit)
	}

	log.Info().Msgf("Scanning %s (batch: %d:%d, hasCustomCookies: %t)", url, start, start+limit-1, cookies != "")

	// 1. Check with Anpan router for galleries, art posts, cloud files & digital libraries
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	if target, err := anpan.InspectTarget(ctx, url); err == nil && target != nil {
		if target.Type == anpan.TargetArchive && target.ArchivePost != nil && len(target.ArchivePost.Files) > 0 {
			var entries []ScanEntry
			firstThumb := ""
			for _, f := range target.ArchivePost.Files {
				thumb := ""
				lowerName := strings.ToLower(f.Name)
				if strings.HasSuffix(lowerName, ".jpg") ||
					strings.HasSuffix(lowerName, ".png") ||
					strings.HasSuffix(lowerName, ".webp") ||
					strings.HasSuffix(lowerName, ".jpeg") ||
					strings.HasSuffix(lowerName, ".gif") {
					thumb = f.URL
				}
				if firstThumb == "" && thumb != "" {
					firstThumb = thumb
				}
				entries = append(entries, ScanEntry{
					Id:        f.Name,
					Title:     f.Name,
					Url:       f.URL,
					Thumbnail: thumb,
				})
			}
			return &ScanInfo{
				Title:      target.ArchivePost.Title,
				Count:      len(entries),
				IsPlaylist: len(entries) > 1,
				Entries:    entries,
				Thumbnail:  firstThumb,
			}, "", nil
		}
		if target.Type == anpan.TargetDirect && target.URL != "" {
			fn := target.Filename
			if fn == "" {
				fn = "Direct Download File"
			}
			return &ScanInfo{
				Title:      fn,
				Count:      1,
				IsPlaylist: false,
				Entries: []ScanEntry{
					{
						Id:        target.URL,
						Title:     fn,
						Url:       target.URL,
						Thumbnail: "",
					},
				},
			}, "", nil
		}
	}

	// Try flat-playlist first (fast for batch playlist items)
	info, stderr, err := runScanAttempt(url, cookies, true, start, limit)
	if err == nil {
		return info, "", nil
	}

	// If flat-playlist fails (e.g. single video or dynamic site like TikTok), try full extraction
	log.Info().Msgf("flat scan failed (%s) — trying full metadata extraction for %s", stderr, url)
	info, stderr2, err2 := runScanAttempt(url, cookies, false, start, limit)
	if err2 == nil {
		return info, "", nil
	}

	// Try deep webpage inspection for embedded m3u8 and video players
	req, errHttp := http.NewRequest("GET", url, nil)
	if errHttp == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		if resp, errDo := httpClient.Do(req); errDo == nil && resp.StatusCode == 200 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			links := ExtractPageLinks(string(bodyBytes), url)
			if len(links) > 0 {
				var entries []ScanEntry
				for _, l := range links {
					title := "Stream: " + l
					if len(title) > 60 {
						title = title[:57] + "..."
					}
					entries = append(entries, ScanEntry{
						Id:        l,
						Title:     title,
						Url:       l,
						Thumbnail: "",
					})
				}
				return &ScanInfo{
					Title:      "Extracted Streams & Media",
					Count:      len(entries),
					IsPlaylist: len(entries) > 1,
					Entries:    entries,
					Thumbnail:  "",
				}, "", nil
			}
		}
	}

	// Fallback for single video links if metadata scan fails completely
	log.Warn().Msgf("Scan metadata fallback for %s (error: %s)", url, stderr2)
	return &ScanInfo{
		Title:      url,
		Count:      1,
		IsPlaylist: false,
		Entries: []ScanEntry{
			{
				Id:        url,
				Title:     url,
				Url:       url,
				Thumbnail: "",
			},
		},
	}, "", nil
}

func runScanAttempt(url string, cookies string, flatPlaylist bool, start int, limit int) (*ScanInfo, string, error) {
	cleanURL := strings.TrimSpace(url)
	if strings.Contains(cleanURL, "tiktok.com/@") {
		if idx := strings.Index(cleanURL, "?"); idx != -1 {
			cleanURL = cleanURL[:idx]
		}
	}

	args := []string{
		"--dump-single-json",
		"--skip-download",
		"--ignore-errors",
		"--no-abort-on-error",
		"--no-warnings",
		"--no-check-certificates",
		"--extractor-args", "instagram:image_persist=1;threads:app_version=30.0.0",
	}

	if flatPlaylist {
		args = append(args, "--flat-playlist", "--yes-playlist")
		if start > 0 && limit > 0 {
			end := start + limit - 1
			args = append(args, "--playlist-items", fmt.Sprintf("%d:%d", start, end))
		}
	}

	if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
		args = append(args, "--impersonate", impersonate)
	}

	if cookies != "" {
		tmpCookie, cleanup, err := CreateEphemeralCookieFile(cookies, cleanURL)
		if err == nil && tmpCookie != "" {
			defer cleanup()
			args = append(args, "--cookies", tmpCookie)
		}
	} else {
		cp := getCookiesPath()
		if workingCookies := getWorkingCookiesPath(cp); workingCookies != "" {
			args = append(args, "--cookies", workingCookies)
		}
	}

	for arg, value := range getEnvVars() {
		args = append(args, arg)
		if value != "" {
			args = append(args, value)
		}
	}

	args = append(args, cleanURL)

	cmd := exec.Command("yt-dlp", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		msg := err.Error()
		log.Error().Msgf("error starting scan: %s", msg)
		return nil, msg, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return nil, "scan timed out", errors.New("scan timed out")
	}

	if waitErr != nil && stdout.Len() == 0 {
		msg := stderr.String()
		log.Error().Msgf("error during scan: %s", msg)
		return nil, msg, waitErr
	}

	var raw RawScanInfo
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		log.Error().Msgf("error unmarshalling scan output: %s", err.Error())
		return nil, "error parsing media scan data", err
	}

	info := &ScanInfo{
		Title:      raw.Title,
		Count:      len(raw.Entries),
		TotalCount: raw.Count,
		IsPlaylist: raw.Type == "playlist" || len(raw.Entries) > 0,
		Entries:    raw.Entries,
		Thumbnail:  raw.Thumbnail,
		Channel:    raw.Channel,
		Uploader:   raw.Uploader,
		Start:      start,
		Limit:      limit,
		HasMore:    len(raw.Entries) >= limit && (raw.Count == 0 || (start+len(raw.Entries)-1) < raw.Count),
		NextStart:  start + len(raw.Entries),
	}

	if info.IsPlaylist {
		info.Count = len(raw.Entries)
	}

	tiktokUser := ""
	if strings.Contains(cleanURL, "tiktok.com") {
		if m := regexp.MustCompile(`tiktok\.com/@([a-zA-Z0-9_.]+)`).FindStringSubmatch(cleanURL); len(m) > 1 {
			tiktokUser = m[1]
		}
	}

	ytRegex := regexp.MustCompile(`(?:youtu\.be/|youtube\.com/(?:watch\?v=|shorts/|embed/))([a-zA-Z0-9_-]{11})`)
	for i := range info.Entries {
		e := &info.Entries[i]
		if e.Title == "" {
			if e.Track != "" {
				e.Title = e.Track
			} else if e.Description != "" {
				e.Title = e.Description
			} else if e.Id != "" {
				e.Title = "TikTok video #" + e.Id
			}
		}
		if e.Url == "" && e.Id != "" {
			if tiktokUser != "" || strings.Contains(cleanURL, "tiktok.com") {
				user := tiktokUser
				if user == "" && e.Uploader != "" {
					user = strings.TrimPrefix(e.Uploader, "@")
				}
				if user == "" {
					user = "video"
				}
				e.Url = "https://www.tiktok.com/@" + user + "/video/" + e.Id
			} else if len(e.Id) == 11 {
				e.Url = "https://www.youtube.com/watch?v=" + e.Id
			} else {
				e.Url = e.Id
			}
		}
		if e.Thumbnail == "" && len(e.Thumbnails) > 0 {
			e.Thumbnail = e.Thumbnails[len(e.Thumbnails)-1].URL
		}
		if e.Thumbnail == "" {
			if len(e.Id) == 11 {
				e.Thumbnail = "https://i.ytimg.com/vi/" + e.Id + "/hqdefault.jpg"
			} else if e.Url != "" {
				if m := ytRegex.FindStringSubmatch(e.Url); len(m) > 1 {
					e.Thumbnail = "https://i.ytimg.com/vi/" + m[1] + "/hqdefault.jpg"
				}
			}
		}
		if e.Uploader == "" && raw.Uploader != "" {
			e.Uploader = raw.Uploader
		}
		if e.Channel == "" && raw.Channel != "" {
			e.Channel = raw.Channel
		}
	}
	if info.Thumbnail == "" && len(info.Entries) > 0 && info.Entries[0].Thumbnail != "" {
		info.Thumbnail = info.Entries[0].Thumbnail
	}

	if raw.Count > info.Count {
		info.Count = raw.Count
	}
	if info.Count == 0 {
		info.Count = 1
	}
	if info.Title == "" {
		if tiktokUser != "" {
			info.Title = "@" + tiktokUser + " (TikTok Channel)"
		} else {
			info.Title = url
		}
	}

	if len(info.Entries) == 0 {
		videoUrl := raw.WebpageUrl
		if videoUrl == "" {
			videoUrl = raw.Url
		}
		if videoUrl == "" {
			videoUrl = url
		}
		thumb := raw.Thumbnail
		if thumb == "" && len(raw.Thumbnails) > 0 {
			thumb = raw.Thumbnails[len(raw.Thumbnails)-1].URL
		}
		if thumb == "" {
			if m := ytRegex.FindStringSubmatch(videoUrl); len(m) > 1 {
				thumb = "https://i.ytimg.com/vi/" + m[1] + "/hqdefault.jpg"
			}
		}
		info.Entries = []ScanEntry{
			{
				Id:        raw.Id,
				Title:     info.Title,
				Url:       videoUrl,
				Thumbnail: thumb,
				Uploader:  raw.Uploader,
				Channel:   raw.Channel,
			},
		}
		if info.Thumbnail == "" {
			info.Thumbnail = thumb
		}
	} else {
		for i := range info.Entries {
			if info.Entries[i].Url == "" {
				if info.Entries[i].Id != "" {
					if tiktokUser != "" {
						info.Entries[i].Url = "https://www.tiktok.com/@" + tiktokUser + "/video/" + info.Entries[i].Id
					} else {
						info.Entries[i].Url = "https://www.youtube.com/watch?v=" + info.Entries[i].Id
					}
				} else {
					info.Entries[i].Url = url
				}
			}
			if info.Entries[i].Title == "" {
				info.Entries[i].Title = info.Title
			}
		}
	}

	return info, "", nil
}

type RawScanEntry struct {
	Id            string          `json:"id"`
	Title         string          `json:"title"`
	Description   string          `json:"description,omitempty"`
	Track         string          `json:"track,omitempty"`
	Url           string          `json:"url"`
	WebpageUrl    string          `json:"webpage_url"`
	Thumbnail     string          `json:"thumbnail"`
	Thumbnails    []ScanThumbnail `json:"thumbnails"`
	Duration      float64         `json:"duration,omitempty"`
	Uploader      string          `json:"uploader,omitempty"`
	Channel       string          `json:"channel,omitempty"`
	ViewCount     int64           `json:"view_count,omitempty"`
	PlaylistTitle string          `json:"playlist_title,omitempty"`
	PlaylistCount int             `json:"playlist_count,omitempty"`
}

// StreamScanUrl runs yt-dlp flat-playlist extraction and emits each entry live as it arrives
func StreamScanUrl(ctx context.Context, inputUrl string, cookies string, onEntry func(entry ScanEntry, count int), onMeta func(title, uploader, channel, thumbnail string, total int)) error {
	cleanURL := strings.TrimSpace(inputUrl)
	if strings.Contains(cleanURL, "tiktok.com/@") {
		if idx := strings.Index(cleanURL, "?"); idx != -1 {
			cleanURL = cleanURL[:idx]
		}
	}

	if IsFacebookShareURL(cleanURL) {
		profileHandle, normalizedURL := extractFbProfileFromShareURL(cleanURL, cookies)
		if profileHandle != "Facebook" && normalizedURL != "" {
			log.Info().Msgf("Resolved Facebook share link for stream: %s -> profile=%s, URL=%s", cleanURL, profileHandle, normalizedURL)
			return StreamFacebookVideosFromNormalized(ctx, normalizedURL, cookies, profileHandle, onEntry, onMeta)
		}
		return StreamFacebookVideos(ctx, cleanURL, cookies, onEntry, onMeta)
	}

	if IsFacebookProfileURL(cleanURL) {
		return StreamFacebookVideos(ctx, cleanURL, cookies, onEntry, onMeta)
	}

	args := []string{
		"--lazy-playlist",
		"--dump-json",
		"--flat-playlist",
		"--skip-download",
		"--ignore-errors",
		"--no-abort-on-error",
		"--no-warnings",
		"--no-check-certificates",
		"--extractor-args", "instagram:image_persist=1;threads:app_version=30.0.0",
	}

	if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
		args = append(args, "--impersonate", impersonate)
	}

	if cookies != "" {
		tmpCookie, cleanup, err := CreateEphemeralCookieFile(cookies, cleanURL)
		if err == nil && tmpCookie != "" {
			defer cleanup()
			args = append(args, "--cookies", tmpCookie)
		}
	} else {
		cp := getCookiesPath()
		if workingCookies := getWorkingCookiesPath(cp); workingCookies != "" {
			args = append(args, "--cookies", workingCookies)
		}
	}

	for arg, value := range getEnvVars() {
		args = append(args, arg)
		if value != "" {
			args = append(args, value)
		}
	}

	args = append(args, cleanURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdoutPipe)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	count := 0
	firstItem := true

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw RawScanEntry
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		title := strings.TrimSpace(raw.Title)
		if title == "" {
			if raw.Track != "" {
				title = raw.Track
			} else if raw.Description != "" {
				title = raw.Description
			} else if raw.Id != "" {
				title = "Video #" + raw.Id
			} else {
				title = cleanURL
			}
		}

		thumb := raw.Thumbnail
		if thumb == "" && len(raw.Thumbnails) > 0 {
			thumb = raw.Thumbnails[len(raw.Thumbnails)-1].URL
		}

		targetUrl := raw.Url
		if targetUrl == "" && raw.Id != "" {
			if strings.Contains(cleanURL, "tiktok.com") {
				if raw.Uploader != "" {
					targetUrl = fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", raw.Uploader, raw.Id)
				} else {
					targetUrl = fmt.Sprintf("https://www.tiktok.com/video/%s", raw.Id)
				}
			} else if strings.Contains(cleanURL, "youtube.com") || strings.Contains(cleanURL, "youtu.be") {
				targetUrl = fmt.Sprintf("https://www.youtube.com/watch?v=%s", raw.Id)
			} else {
				targetUrl = raw.Id
			}
		}

		entry := ScanEntry{
			Id:         raw.Id,
			Title:      title,
			Description: raw.Description,
			Track:      raw.Track,
			Url:        targetUrl,
			Thumbnail:  thumb,
			Thumbnails: raw.Thumbnails,
			Duration:   raw.Duration,
			Uploader:   raw.Uploader,
			Channel:    raw.Channel,
			ViewCount:  raw.ViewCount,
		}

		count++

		if firstItem {
			firstItem = false
			channelTitle := raw.PlaylistTitle
			if channelTitle == "" {
				channelTitle = raw.Channel
			}
			if channelTitle == "" {
				channelTitle = raw.Uploader
			}
			onMeta(channelTitle, raw.Uploader, raw.Channel, thumb, raw.PlaylistCount)
		}

		onEntry(entry, count)
	}

	_ = cmd.Wait()
	return nil
}

func isReadableFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
