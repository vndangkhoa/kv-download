package anpan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// HasAria2c checks if aria2c binary exists in PATH.
func HasAria2c() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "aria2c", "--version")
	return cmd.Run() == nil
}

// FormatYtDlpAria2Args builds yt-dlp aria2c downloader arguments without probing PATH.
func FormatYtDlpAria2Args(connections int, rateLimit string) []string {
	c := connections
	if c < 1 {
		c = 16
	} else if c > 16 {
		c = 16
	}

	dlArgs := fmt.Sprintf(
		"aria2c:-x %d -s %d -k 1M -j %d --file-allocation=none --disk-cache=64M --max-tries=5 --retry-wait=1 --connect-timeout=10 --timeout=30 --summary-interval=1 --console-log-level=notice --auto-file-renaming=false",
		c, c, c,
	)

	rl := strings.TrimSpace(rateLimit)
	lowerRL := strings.ToLower(rl)
	if rl != "" && lowerRL != "unlimited" && lowerRL != "0" && lowerRL != "none" && lowerRL != "max" {
		dlArgs += " --max-download-limit=" + rl
	}

	return []string{
		"--downloader", "aria2c",
		"--downloader-args", dlArgs,
	}
}

// BuildYtDlpAria2ArgsWithLimit returns yt-dlp downloader flags for multi-connection acceleration
// with optional per-task bandwidth limit.
func BuildYtDlpAria2ArgsWithLimit(connections int, rateLimit string) []string {
	if !HasAria2c() {
		return nil
	}
	return FormatYtDlpAria2Args(connections, rateLimit)
}

// BuildYtDlpAria2Args returns yt-dlp downloader flags for multi-connection acceleration.
func BuildYtDlpAria2Args(connections int) []string {
	return BuildYtDlpAria2ArgsWithLimit(connections, "")
}

// DownloadDirectFileAria downloads a single file using parallel chunks with aria2c.
func DownloadDirectFileAria(ctx context.Context, downloadURL, outputDir, filename string, connections int) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	c := connections
	if c < 1 {
		c = 16
	} else if c > 16 {
		c = 16
	}

	args := []string{
		"-d", outputDir,
		"-x", strconv.Itoa(c),
		"-s", strconv.Itoa(c),
		"-k", "1M",
		"-j", strconv.Itoa(c),
		"--file-allocation=none",
		"--disk-cache=64M",
		"--connect-timeout=10",
		"--timeout=30",
		"--max-tries=5",
		"--retry-wait=1",
		"--summary-interval=1",
		"--auto-file-renaming=false",
		"--allow-overwrite=true",
	}

	if filename != "" {
		args = append(args, "-o", filename)
	}
	args = append(args, downloadURL)

	cmd := exec.CommandContext(ctx, "aria2c", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("aria2c error: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DownloadTorrentAria downloads magnet links or torrent files via aria2c BitTorrent engine.
func DownloadTorrentAria(ctx context.Context, target, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	args := []string{
		"-d", outputDir,
		"--seed-time=0",
		"--summary-interval=2",
		"--bt-stop-timeout=60",
		target,
	}
	cmd := exec.CommandContext(ctx, "aria2c", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("aria2c torrent error: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
