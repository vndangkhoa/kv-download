package media

import (
	"bytes"
	"encoding/json"
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

func TestServeMediaByQueueTaskUUIDAndReorganizedModes(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MR_DOWNLOAD_DIR", tmpDir)

	origQueue := GlobalQueue
	q := &QueueManager{
		tasks: make(map[string]*DownloadTask),
	}
	GlobalQueue = q
	defer func() { GlobalQueue = origQueue }()

	payload := []byte("ftypisom-mock-video-bytes-for-queue-uuid-test")

	// Case 1: category_channel mode (videos/nocc.2026/<file>)
	catChannelDir := filepath.Join(tmpDir, "videos", "nocc.2026")
	if err := os.MkdirAll(catChannelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fileName := "3 comments Anh muốn hiệp sau vẫn còn lực Thì xài Trà Ba Kích Tím Feelex nha 🫧💦 https s.sho.mp4"
	if err := os.WriteFile(filepath.Join(catChannelDir, fileName), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	taskUUID := "502f2593-0bce-4df0-9dc0-80e5019e64c1"
	mediaID := "3faf55e3913cb4054bdbf3aba9f2487c/" + fileName
	q.mu.Lock()
	q.tasks[taskUUID] = &DownloadTask{
		ID:        taskUUID,
		Status:    StatusCompleted,
		Title:     strings.TrimSuffix(fileName, ".mp4"),
		MediaID:   mediaID,
		MediaName: fileName,
		Channel:   "nocc.2026",
	}
	q.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/download?id="+url.QueryEscape(taskUUID)+"&inline=true", nil)
	rec := httptest.NewRecorder()
	ServeMedia(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeMedia by taskUUID (category_channel) HTTP = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), payload) {
		t.Fatalf("ServeMedia payload mismatch")
	}

	// Case 2: channel_category mode (<Channel>/videos/<file>) where candidateFilePaths alone misses
	tmpDir2 := t.TempDir()
	t.Setenv("MR_DOWNLOAD_DIR", tmpDir2)
	chanCatDir := filepath.Join(tmpDir2, "nocc.2026", "videos")
	if err := os.MkdirAll(chanCatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chanCatDir, fileName), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/download?id="+url.QueryEscape(taskUUID)+"&inline=true", nil)
	rec2 := httptest.NewRecorder()
	ServeMedia(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("ServeMedia by taskUUID (channel_category) HTTP = %d, want 200 (body=%q)", rec2.Code, rec2.Body.String())
	}

	// Case 3: mediaId query fallback after Clear Completed removes task from GlobalQueue
	req3 := httptest.NewRequest(http.MethodGet, "/download?id="+url.QueryEscape("missing-cleared-task-id")+"&mediaId="+url.QueryEscape(mediaID)+"&inline=true", nil)
	rec3 := httptest.NewRecorder()
	ServeMedia(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("ServeMedia mediaId fallback HTTP = %d, want 200 (body=%q)", rec3.Code, rec3.Body.String())
	}

	// Case 4: /api/app/version endpoint
	t.Setenv("APP_VERSION", "1.0.43")
	vReq := httptest.NewRequest(http.MethodGet, "/api/app/version", nil)
	vRec := httptest.NewRecorder()
	AppVersionHandler(vRec, vReq)
	if vRec.Code != http.StatusOK {
		t.Fatalf("AppVersionHandler HTTP = %d, want 200", vRec.Code)
	}
	var verInfo AppVersionInfo
	if err := json.Unmarshal(vRec.Body.Bytes(), &verInfo); err != nil {
		t.Fatalf("failed to unmarshal /api/app/version JSON: %v", err)
	}
	if verInfo.Version != "1.0.43" {
		t.Fatalf("version = %q, want 1.0.43", verInfo.Version)
	}
	if !strings.Contains(verInfo.ApkURL, "kvdownload-v1.0.43.apk") {
		t.Fatalf("unexpected apkUrl: %q", verInfo.ApkURL)
	}
}

func TestCPUProtectionCompatPolicyCacheCleanupAndSniffGuard(t *testing.T) {
	chromeUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	edgeUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0"
	firefoxUA := "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0"
	safariUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Safari/605.1.15"
	exoUA := "KVDownloadAndroid/1.0.44 okhttp/4.12.0 ExoPlayerLib/2.19"

	// 1. Browser-aware compat rules
	if needsBrowserCompatTranscodeForUA(chromeUA, ".mp4", "vp9", "aac", false) {
		t.Fatal("Chrome should play VP9+AAC MP4 natively without CPU transcode")
	}
	if needsBrowserCompatTranscodeForUA(chromeUA, ".mp4", "av1", "opus", false) {
		t.Fatal("Chrome should play AV1+Opus MP4 natively without CPU transcode")
	}
	if needsBrowserCompatTranscodeForUA(edgeUA, ".mp4", "hevc", "aac", false) {
		t.Fatal("Edge should attempt direct MP4 playback before compat fallback")
	}
	if !needsBrowserCompatTranscodeForUA(chromeUA, ".mp4", "vp9", "aac", true) {
		t.Fatal("forceCompat=true should enable compat transcode even on Chrome")
	}
	if !needsBrowserCompatTranscodeForUA(firefoxUA, ".mp4", "vp9", "aac", false) {
		t.Fatal("Firefox must receive compat transcode for VP9 inside MP4")
	}
	if !needsBrowserCompatTranscodeForUA(safariUA, ".mp4", "vp9", "aac", false) {
		t.Fatal("Safari must receive compat transcode for VP9 inside MP4")
	}
	if needsBrowserCompatTranscodeForUA(exoUA, ".mkv", "hevc", "eac3", false) {
		t.Fatal("ExoPlayer/Android native player should not be transcoded unless forced")
	}

	// 2. Bounded ffmpeg args (-threads 2 and <=720p scale for live fragmented streams)
	argsSync := strings.Join(buildCompatFFmpegArgs("/tmp/in.mp4", "vp9", "aac", false, "/tmp/out.mp4"), " ")
	if !strings.Contains(argsSync, "-threads 2") || !strings.Contains(argsSync, "threads=2") {
		t.Fatalf("expected bounded threads in sync ffmpeg args, got: %s", argsSync)
	}
	argsFrag := strings.Join(buildCompatFFmpegArgs("/tmp/in.mp4", "vp9", "aac", true, "pipe:1"), " ")
	if !strings.Contains(argsFrag, "-threads 2") || !strings.Contains(argsFrag, "min(1280,iw)") {
		t.Fatalf("expected bounded threads + min(1280,iw) downscale in fragmented args, got: %s", argsFrag)
	}

	// 3. CleanupCompatCache deletes .tmp.mp4 files while keeping valid .mp4 files
	tmpDir := t.TempDir()
	t.Setenv("MR_DOWNLOAD_DIR", tmpDir)
	cacheDir := filepath.Join(tmpDir, ".compat_cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tmpArtifact := filepath.Join(cacheDir, "stale.tmp.mp4")
	goodArtifact := filepath.Join(cacheDir, "valid.mp4")
	if err := os.WriteFile(tmpArtifact, []byte("incomplete"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goodArtifact, []byte("valid-mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	CleanupCompatCache()
	if _, err := os.Stat(tmpArtifact); !os.IsNotExist(err) {
		t.Fatalf("expected CleanupCompatCache to remove %s", tmpArtifact)
	}
	if _, err := os.Stat(goodArtifact); err != nil {
		t.Fatalf("expected CleanupCompatCache to preserve %s: %v", goodArtifact, err)
	}

	// 4. BrowserSniffHandler rejects custom URI schemes immediately without spawning yt-dlp
	for _, bad := range []string{
		"snssdk1180://aweme/detail/123",
		"https://snssdk1180://aweme/detail/7705854963582389522?refer=web",
		"intent://scan/#Intent;end",
	} {
		sReq := httptest.NewRequest(http.MethodGet, "/api/browser/sniff?url="+url.QueryEscape(bad), nil)
		sRec := httptest.NewRecorder()
		BrowserSniffHandler(sRec, sReq)
		if sRec.Code != http.StatusBadRequest {
			t.Fatalf("BrowserSniffHandler(%q) HTTP = %d, want 400", bad, sRec.Code)
		}
	}
	if !isBareRootHomepage("https://tinhte.vn") || !isBareRootHomepage("https://www.tiktok.com/") {
		t.Fatal("expected bare root homepages to match isBareRootHomepage")
	}
	if isBareRootHomepage("https://av01.media/jp/video/226601") {
		t.Fatal("video path should not be treated as bare root homepage")
	}
}
