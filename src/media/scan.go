package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	Id         string          `json:"id"`
	Title      string          `json:"title"`
	Url        string          `json:"url"`
	Thumbnail  string          `json:"thumbnail"`
	Thumbnails []ScanThumbnail `json:"thumbnails"`
}

type ScanInfo struct {
	Title      string      `json:"title"`
	Count      int         `json:"count"`
	IsPlaylist bool        `json:"isPlaylist"`
	Entries    []ScanEntry `json:"entries"`
	Thumbnail  string      `json:"thumbnail"`
}

// ScanUrl inspects a URL with anpan extractors and yt-dlp metadata extraction.
func ScanUrl(inputUrl string, cookies string) (*ScanInfo, string, error) {
	url := strings.TrimSpace(inputUrl)
	if url == "" {
		return nil, "", errors.New("missing URL")
	}

	log.Info().Msgf("Scanning %s (hasCustomCookies: %t)", url, cookies != "")

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

	// Try flat-playlist first (fast for large playlists)
	info, stderr, err := runScanAttempt(url, cookies, true)
	if err == nil {
		return info, "", nil
	}

	// If flat-playlist fails (e.g. single video or dynamic site like TikTok), try full extraction
	log.Info().Msgf("flat scan failed (%s) — trying full metadata extraction for %s", stderr, url)
	info, stderr2, err2 := runScanAttempt(url, cookies, false)
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

func runScanAttempt(url string, cookies string, flatPlaylist bool) (*ScanInfo, string, error) {
	args := []string{
		"--dump-single-json",
		"--skip-download",
		"--no-warnings",
		"--no-check-certificates",
		"--extractor-args", "instagram:image_persist=1;threads:app_version=30.0.0",
	}

	if flatPlaylist {
		args = append(args, "--flat-playlist", "--yes-playlist")
	}

	if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
		args = append(args, "--impersonate", impersonate)
	}

	if cookies != "" {
		tmpCookie, cleanup, err := CreateEphemeralCookieFile(cookies, url)
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

	args = append(args, url)

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

	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		log.Warn().Msgf("scan failed for %s: %s", url, msg)
		return nil, msg, waitErr
	}

	var raw struct {
		Id         string          `json:"id"`
		Type       string          `json:"_type"`
		Title      string          `json:"title"`
		Count      int             `json:"playlist_count"`
		Entries    []ScanEntry     `json:"entries"`
		Thumb      string          `json:"thumbnail"`
		Thumbnails []ScanThumbnail `json:"thumbnails"`
		Url        string          `json:"url"`
		WebpageUrl string          `json:"webpage_url"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		msg := "unable to parse scan result"
		log.Error().Msgf("%s: %v", msg, err)
		return nil, msg, err
	}

	info := &ScanInfo{
		Title:      raw.Title,
		IsPlaylist: raw.Type == "playlist" || len(raw.Entries) > 1,
		Thumbnail:  raw.Thumb,
	}
	if info.Thumbnail == "" && len(raw.Thumbnails) > 0 {
		info.Thumbnail = raw.Thumbnails[len(raw.Thumbnails)-1].URL
	}
	if raw.Entries != nil {
		info.Entries = raw.Entries
		info.Count = len(raw.Entries)
	}

	ytRegex := regexp.MustCompile(`(?:youtu\.be/|youtube\.com/(?:watch\?v=|shorts/|embed/))([a-zA-Z0-9_-]{11})`)
	for i := range info.Entries {
		e := &info.Entries[i]
		if e.Url == "" && e.Id != "" {
			if len(e.Id) == 11 {
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
		info.Title = url
	}

	if len(info.Entries) == 0 {
		videoUrl := raw.WebpageUrl
		if videoUrl == "" {
			videoUrl = raw.Url
		}
		if videoUrl == "" {
			videoUrl = url
		}
		thumb := raw.Thumb
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
			},
		}
		if info.Thumbnail == "" {
			info.Thumbnail = thumb
		}
	} else {
		for i := range info.Entries {
			if info.Entries[i].Url == "" {
				if info.Entries[i].Id != "" {
					info.Entries[i].Url = "https://www.youtube.com/watch?v=" + info.Entries[i].Id
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

func isReadableFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
