package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
