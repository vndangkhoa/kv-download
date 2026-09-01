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

// BuildYtDlpAria2Args returns yt-dlp downloader flags for multi-connection acceleration.
func BuildYtDlpAria2Args(connections int) []string {
	if !HasAria2c() {
		return nil
	}
	c := connections
	if c < 1 {
		c = 8
	} else if c > 16 {
		c = 16
	}
	return []string{
		"--downloader", "aria2c",
		"--downloader-args", fmt.Sprintf("aria2c:-x %d -s %d -k 1M -j %d", c, c, c),
	}
}

// DownloadDirectFileAria downloads a single file using parallel chunks with aria2c.
func DownloadDirectFileAria(ctx context.Context, downloadURL, outputDir, filename string, connections int) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	c := connections
	if c < 1 {
		c = 16
	} else if c > 32 {
		c = 32
	}

	args := []string{
		"-d", outputDir,
		"-x", strconv.Itoa(c),
		"-s", strconv.Itoa(c),
		"-k", "1M",
		"-j", strconv.Itoa(c),
		"--connect-timeout=10",
		"--timeout=30",
		"--max-tries=3",
		"--retry-wait=2",
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
