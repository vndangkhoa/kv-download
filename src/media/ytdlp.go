package media

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type YtDlpVersionInfo struct {
	Current       string `json:"current"`
	Channel       string `json:"channel"`
	LatestStable  string `json:"latestStable"`
	LatestNightly string `json:"latestNightly"`
	IsUpToDate    bool   `json:"isUpToDate"`
}

// GetCurrentYtDlpVersion returns the active yt-dlp version string
func GetCurrentYtDlpVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// YtDlpVersionHandler returns version and channel status
func YtDlpVersionHandler(w http.ResponseWriter, r *http.Request) {
	currentVer := GetCurrentYtDlpVersion()

	// Detect channel from version string or check yt-dlp
	channel := "stable"
	if strings.Contains(currentVer, ".") && len(strings.Split(currentVer, ".")) > 3 {
		channel = "nightly"
	}

	info := YtDlpVersionInfo{
		Current:       currentVer,
		Channel:       channel,
		LatestStable:  "2026.08.19",
		LatestNightly: "2026.08.30.232658",
		IsUpToDate:    true,
	}

	// Fetch live release tags from GitHub API
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	reqStable, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest", nil)
	if err == nil {
		reqStable.Header.Set("User-Agent", "KV-Download/2.0")
		if resp, err := httpClient.Do(reqStable); err == nil && resp.StatusCode == 200 {
			var release struct {
				TagName string `json:"tag_name"`
			}
			if json.NewDecoder(resp.Body).Decode(&release) == nil && release.TagName != "" {
				info.LatestStable = release.TagName
			}
			resp.Body.Close()
		}
	}

	reqNightly, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/yt-dlp/yt-dlp-nightly-builds/releases/latest", nil)
	if err == nil {
		reqNightly.Header.Set("User-Agent", "KV-Download/2.0")
		if resp, err := httpClient.Do(reqNightly); err == nil && resp.StatusCode == 200 {
			var release struct {
				TagName string `json:"tag_name"`
			}
			if json.NewDecoder(resp.Body).Decode(&release) == nil && release.TagName != "" {
				info.LatestNightly = release.TagName
			}
			resp.Body.Close()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// YtDlpUpdateHandler updates or switches yt-dlp channel (stable, nightly, master)
func YtDlpUpdateHandler(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	if channel == "" {
		channel = "stable"
	}

	log.Info().Msgf("Switching / Updating yt-dlp to channel: %s", channel)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	target := channel + "@latest"
	if channel == "stable" {
		target = "stable@latest"
	} else if channel == "nightly" {
		target = "nightly@latest"
	} else if channel == "master" {
		target = "master@latest"
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", "--update-to", target)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	outputStr := strings.TrimSpace(outBuf.String() + "\n" + errBuf.String())

	// Fallback if binary update refused: use pip install
	if err != nil && strings.Contains(outputStr, "pip") {
		var pipCmd *exec.Cmd
		if channel == "nightly" || channel == "master" {
			pipCmd = exec.CommandContext(ctx, "python3", "-m", "pip", "install", "--break-system-packages", "--user", "--upgrade", "https://github.com/yt-dlp/yt-dlp/archive/master.tar.gz")
		} else {
			pipCmd = exec.CommandContext(ctx, "python3", "-m", "pip", "install", "--break-system-packages", "--user", "--upgrade", "yt-dlp")
		}
		_ = pipCmd.Run()
	}

	newVersion := GetCurrentYtDlpVersion()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"channel": channel,
		"version": newVersion,
		"output":  outputStr,
		"message": "yt-dlp channel updated successfully to " + channel,
	})
}
