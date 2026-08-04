package media

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type ScanEntry struct {
	Id        string `json:"id"`
	Title     string `json:"title"`
	Url       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
}

type ScanInfo struct {
	Title      string      `json:"title"`
	Count      int         `json:"count"`
	IsPlaylist bool        `json:"isPlaylist"`
	Entries    []ScanEntry `json:"entries"`
	Thumbnail  string      `json:"thumbnail"`
}

// ScanUrl inspects a URL with yt-dlp metadata-only extraction (no download).
// For playlists/profiles it returns the entry list without fetching each video.
func ScanUrl(inputUrl string) (*ScanInfo, string, error) {
	url := strings.TrimSpace(inputUrl)
	if url == "" {
		return nil, "", errors.New("missing URL")
	}

	log.Info().Msgf("Scanning %s", url)

	const maxAttempts = 3
	var lastStderr string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		info, stderr, err := runScanAttempt(url)
		if err == nil {
			return info, "", nil
		}
		lastStderr = stderr
		if attempt < maxAttempts {
			log.Warn().Msgf("scan attempt %d/%d failed for %s: %s — retrying", attempt, maxAttempts, url, stderr)
			time.Sleep(2 * time.Second)
		}
	}

	return nil, lastStderr, errors.New(lastStderr)
}

func runScanAttempt(url string) (*ScanInfo, string, error) {
	args := []string{
		"--dump-single-json",
		"--flat-playlist",
		"--skip-download",
		"--no-warnings",
		"--no-check-certificates",
	}

	if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
		args = append(args, "--impersonate", impersonate)
	} else {
		args = append(args, "--impersonate", "chrome")
	}

	cp := getCookiesPath()
	if workingCookies := getWorkingCookiesPath(cp); workingCookies != "" {
		args = append(args, "--cookies", workingCookies)
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
		Id         string      `json:"id"`
		Type       string      `json:"_type"`
		Title      string      `json:"title"`
		Count      int         `json:"playlist_count"`
		Entries    []ScanEntry `json:"entries"`
		Thumb      string      `json:"thumbnail"`
		Url        string      `json:"url"`
		WebpageUrl string      `json:"webpage_url"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		msg := "unable to parse scan result"
		log.Error().Msgf("%s: %v", msg, err)
		return nil, msg, err
	}

	info := &ScanInfo{
		Title:      raw.Title,
		IsPlaylist: raw.Type == "playlist",
		Thumbnail:  raw.Thumb,
	}
	if raw.Entries != nil {
		info.Entries = raw.Entries
		info.Count = len(raw.Entries)
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
		info.Entries = []ScanEntry{
			{
				Id:        raw.Id,
				Title:     info.Title,
				Url:       videoUrl,
				Thumbnail: raw.Thumb,
			},
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
