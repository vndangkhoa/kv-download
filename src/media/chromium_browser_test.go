package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeOmniboxURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", defaultChromiumStartPage},
		{"youtube.com", "https://youtube.com"},
		{"https://www.tiktok.com/@user/video/123", "https://www.tiktok.com/@user/video/123"},
		{"funny cats compilation", "https://www.google.com/search?q=funny+cats+compilation"},
		{"av01", "https://www.google.com/search?q=av01"},
		{"http://localhost:9292", "http://localhost:9292"},
	}
	for _, tc := range cases {
		got := normalizeOmniboxURL(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeOmniboxURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDetectPlayablePageMediaAndNetworkFilter(t *testing.T) {
	ytItems := detectPlayablePageMedia("https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=5s", "Never Gonna Give You Up - YouTube")
	if len(ytItems) != 1 || ytItems[0].Format != "YOUTUBE" || ytItems[0].URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("unexpected YouTube watch items: %+v", ytItems)
	}

	shortItems := detectPlayablePageMedia("https://youtube.com/shorts/AbCdEfGhIjK", "")
	if len(shortItems) != 1 || shortItems[0].URL != "https://www.youtube.com/watch?v=AbCdEfGhIjK" {
		t.Fatalf("unexpected YouTube shorts items: %+v", shortItems)
	}

	ttItems := detectPlayablePageMedia("https://www.tiktok.com/@creator/video/7400000000000000001", "Cool clip")
	if len(ttItems) != 1 || ttItems[0].Format != "TIKTOK" {
		t.Fatalf("unexpected TikTok items: %+v", ttItems)
	}

	if _, _, _, ok := isCapturableNetworkMedia("https://r3---sn.googlevideo.com/videoplayback?id=123", "video/mp4"); ok {
		t.Fatal("expected raw googlevideo chunk URL to be skipped in favor of canonical watch URL")
	}
	if _, _, _, ok := isCapturableNetworkMedia("https://cdn.example.com/seg-12.ts", "video/mp2t"); ok {
		t.Fatal("expected .ts segment to be skipped")
	}
	mt, fmtLabel, isStream, ok := isCapturableNetworkMedia("https://cdn.example.com/live/master.m3u8?tok=1", "application/vnd.apple.mpegurl")
	if !ok || mt != "video" || fmtLabel != "M3U8 HLS" || !isStream {
		t.Fatalf("expected master.m3u8 to be captured, got ok=%v mt=%s fmt=%s stream=%v", ok, mt, fmtLabel, isStream)
	}
	mtTT, fmtTT, _, okTT := isCapturableNetworkMedia("https://v16-webapp-prime.tiktokcdn.com/video/tos/useast2a/123/?a=1988&mime_type=video_mp4", "video/mp4")
	if !okTT || mtTT != "video" || fmtTT != "MP4 VIDEO" {
		t.Fatalf("expected TikTok CDN video/mp4 to be captured, got ok=%v mt=%s fmt=%s", okTT, mtTT, fmtTT)
	}
}

func TestFormatNetscapeCookiesAndStatusHandler(t *testing.T) {
	formatted := formatNetscapeCookies([]cdpCookie{
		{Name: "SID", Value: "abc", Domain: ".youtube.com", Path: "/", Expires: 1900000000, Secure: true},
	})
	if !strings.Contains(formatted, "# Netscape HTTP Cookie File") ||
		!strings.Contains(formatted, ".youtube.com\tTRUE\t/\tTRUE\t1900000000\tSID\tabc") {
		t.Fatalf("unexpected Netscape cookie formatting:\n%s", formatted)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/browser/chromium/status", nil)
	rec := httptest.NewRecorder()
	ChromiumStatusHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ChromiumStatusHandler HTTP = %d, want 200", rec.Code)
	}
	var st ChromiumStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("failed to decode ChromiumStatus: %v", err)
	}
	if st.Running {
		t.Fatal("Chromium should not be running before a session is started")
	}
}

func TestBuildChromiumArgsLowCPU(t *testing.T) {
	args := buildChromiumArgs(1360, 780, 9222, "/tmp/prof", "https://www.youtube.com")
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "--autoplay-policy=no-user-gesture-required") {
		t.Fatal("expected Chromium args NOT to auto-play video without user activation")
	}
	required := []string{
		"--autoplay-policy=document-user-activation-required",
		"--renderer-process-limit=1",
		"--num-raster-threads=1",
		"--disable-webgl",
		"--disable-webgl2",
		"--disable-3d-apis",
		"--disable-software-rasterizer",
		"--disable-crash-reporter",
		"--disable-breakpad",
		"--window-size=1360,780",
	}
	for _, flag := range required {
		if !strings.Contains(joined, flag) {
			t.Fatalf("expected Chromium args to contain %q, got: %s", flag, joined)
		}
	}
}

func TestClampChromiumViewportAndAckPacing(t *testing.T) {
	w, h := clampChromiumViewport(0, -10)
	if w != defaultChromiumWidth || h != defaultChromiumHeight {
		t.Fatalf("clampChromiumViewport(0,-10) = %dx%d, want %dx%d", w, h, defaultChromiumWidth, defaultChromiumHeight)
	}
	w, h = clampChromiumViewport(100, 100)
	if w != minChromiumWidth || h != minChromiumHeight {
		t.Fatalf("clampChromiumViewport(100,100) = %dx%d, want %dx%d", w, h, minChromiumWidth, minChromiumHeight)
	}
	w, h = clampChromiumViewport(4000, 3000)
	if w != maxChromiumWidth || h != maxChromiumHeight {
		t.Fatalf("clampChromiumViewport(4000,3000) = %dx%d, want %dx%d", w, h, maxChromiumWidth, maxChromiumHeight)
	}
	w, h = clampChromiumViewport(1360, 780)
	if w != 1360 || h != 780 {
		t.Fatalf("clampChromiumViewport(1360,780) = %dx%d, want 1360x780", w, h)
	}

	base := time.Unix(1700000000, 0)
	if d := nextScreencastAckDelay(base, time.Time{}, screencastMinInterval); d != 0 {
		t.Fatalf("expected zero delay when lastAck is zero, got %v", d)
	}
	if d := nextScreencastAckDelay(base.Add(200*time.Millisecond), base, screencastMinInterval); d != 0 {
		t.Fatalf("expected zero delay after interval elapsed, got %v", d)
	}
	d := nextScreencastAckDelay(base.Add(40*time.Millisecond), base, screencastMinInterval)
	if d != 100*time.Millisecond {
		t.Fatalf("nextScreencastAckDelay(+40ms) = %v, want 100ms", d)
	}
}
