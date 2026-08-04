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
	"golang.org/x/sys/unix"
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

	args := []string{
		"--dump-single-json",
		"--flat-playlist",
		"--skip-download",
		"--no-warnings",
	}

	if cp := getCookiesPath(); isWritableFile(cp) {
		args = append(args, "--cookies", cp)
	}
	args = append(args, url)

	log.Info().Msgf("Scanning %s", url)

	cmd := exec.Command("yt-dlp", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		log.Error().Msgf("error starting scan: %v", err)
		return nil, err.Error(), err
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
		Type    string      `json:"_type"`
		Title   string      `json:"title"`
		Count   int         `json:"playlist_count"`
		Entries []ScanEntry `json:"entries"`
		Thumb   string      `json:"thumbnail"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		log.Error().Msgf("failed to parse scan result: %v", err)
		return nil, "unable to parse scan result", err
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

	return info, "", nil
}

func isWritableFile(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return unix.Access(path, unix.W_OK) == nil
}
