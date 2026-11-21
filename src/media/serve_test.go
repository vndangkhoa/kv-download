package media

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestASCIISafeFilenameAndNeedsCompat(t *testing.T) {
	got := asciiSafeFilename("Con máy cạo râu mini 😆👌.mp4")
	for _, r := range got {
		if r < 0x20 || r > 0x7E {
			t.Fatalf("asciiSafeFilename contained non-ASCII rune %U in %q", r, got)
		}
	}
	if !strings.HasSuffix(got, ".mp4") {
		t.Fatalf("expected .mp4 suffix, got %q", got)
	}

	emojiOnly := asciiSafeFilename("😆👌🔥.mp4")
	if emojiOnly != "media.mp4" {
		t.Fatalf("expected emojiOnly -> media.mp4, got %q", emojiOnly)
	}

	if !needsBrowserCompatTranscode(".mp4", "vp9", "aac", false) {
		t.Fatal("expected vp9 inside .mp4 to trigger compat transcode")
	}
	if !needsBrowserCompatTranscode(".mp4", "hevc", "aac", false) {
		t.Fatal("expected hevc inside .mp4 to trigger compat transcode")
	}
	if needsBrowserCompatTranscode(".mp4", "h264", "aac", false) {
		t.Fatal("h264+aac inside .mp4 should not transcode unless forced")
	}
	if !needsBrowserCompatTranscode(".mp4", "h264", "aac", true) {
		t.Fatal("forceCompat=true should transcode")
	}
}

func TestServeMediaVP9InMP4CompatTranscode(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	tmpDir := t.TempDir()
	t.Setenv("MR_DOWNLOAD_DIR", tmpDir)

	relDir := filepath.Join("videos", "nocc.2026")
	if err := os.MkdirAll(filepath.Join(tmpDir, relDir), 0o755); err != nil {
		t.Fatal(err)
	}

	sampleName := "Con máy cạo râu mini 😆.mp4"
	samplePath := filepath.Join(tmpDir, relDir, sampleName)

	// Synthesize a 1-second VP9 + AAC MP4 file (the exact codec mix Firefox rejects in <video>)
	genCmd := exec.Command(
		"ffmpeg",
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libvpx-vp9",
		"-c:a", "aac",
		"-strict", "-2",
		samplePath,
	)
	if out, err := genCmd.CombinedOutput(); err != nil {
		t.Skipf("unable to encode VP9 test clip (%v): %s", err, string(out))
	}

	relQuery := filepath.ToSlash(filepath.Join(relDir, sampleName))
	req := httptest.NewRequest(http.MethodGet, "/download?inline=true&file="+url.QueryEscape(relQuery), nil)
	rec := httptest.NewRecorder()
	ServeMedia(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ServeMedia HTTP status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Fatalf("Content-Type = %q, want video/mp4", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.HasPrefix(cd, "inline;") || !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("unexpected Content-Disposition: %q", cd)
	}

	outFile := filepath.Join(t.TempDir(), "served.mp4")
	if err := os.WriteFile(outFile, rec.Body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(outFile)
	if err != nil {
		t.Fatal(err)
	}
	vCodec, _ := probeMediaCodecs(outFile, fi)
	if vCodec != "h264" {
		t.Fatalf("served video codec = %q, want h264", vCodec)
	}

	// Ensure .compat_cache is excluded from ScanGallery
	gallery, err := ScanGallery()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range gallery.Items {
		if strings.Contains(it.Path, ".compat_cache") {
			t.Fatalf(".compat_cache leaked into ScanGallery: %s", it.Path)
		}
	}
}
