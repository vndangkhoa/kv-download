package media

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseAV01VideoURL(t *testing.T) {
	cases := []struct {
		url      string
		wantID   string
		wantSlug string
		wantOK   bool
	}{
		{
			url:      "https://av01.media/jp/video/226601/fc2-ppv-5011616",
			wantID:   "226601",
			wantSlug: "fc2-ppv-5011616",
			wantOK:   true,
		},
		{
			url:      "https://av01.media/en/video/226601",
			wantID:   "226601",
			wantSlug: "",
			wantOK:   true,
		},
		{
			url:      "https://www.av01.tv/zh-tw/video/99123/abw-123",
			wantID:   "99123",
			wantSlug: "abw-123",
			wantOK:   true,
		},
		{
			url:      "https://customers.iw01.xyz/api/v1/videos/226601/manifest/master.m3u8?access_token=abc",
			wantID:   "226601",
			wantSlug: "",
			wantOK:   true,
		},
		{
			url:    "https://av01.media/",
			wantOK: false,
		},
		{
			url:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		id, slug, ok := ParseAV01VideoURL(tc.url)
		if ok != tc.wantOK || id != tc.wantID || slug != tc.wantSlug {
			t.Errorf("ParseAV01VideoURL(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.url, id, slug, ok, tc.wantID, tc.wantSlug, tc.wantOK)
		}
	}
}

func TestExtractAV01CodeFromJWT(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"/fmp4/20261121f/FC2-PPV-5011616/map/","exp":1800000000}`))
	token := "eyJhbGciOiJIUzI1NiJ9." + payload + ".sig"
	if got := extractAV01CodeFromJWT(token); got != "FC2-PPV-5011616" {
		t.Fatalf("extractAV01CodeFromJWT = %q, want %q", got, "FC2-PPV-5011616")
	}
}

func TestParseAV01MasterM3U8AndSelectVariant(t *testing.T) {
	master := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
index360-sv3-v1-a1.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2400000,RESOLUTION=1280x720
index720-sv3-v1-a1.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5200000,RESOLUTION=1920x1080
index1080-sv3-v1-a1.m3u8`

	variants := parseAV01MasterM3U8(master, "https://customers.iw01.xyz/api/v1/videos/226601/manifest/", "tok123", "")
	if len(variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(variants))
	}
	if variants[0].Label != "1080p" || variants[1].Label != "720p" || variants[2].Label != "360p" {
		t.Fatalf("unexpected variant order/labels: %+v", variants)
	}
	if !strings.Contains(variants[0].URL, "access_token=tok123") {
		t.Fatalf("expected access_token appended to variant URL, got %q", variants[0].URL)
	}

	if sel := SelectAV01Variant(variants, "highest"); sel.Label != "1080p" {
		t.Errorf("SelectAV01Variant(highest) = %q, want 1080p", sel.Label)
	}
	if sel := SelectAV01Variant(variants, "720p"); sel.Label != "720p" {
		t.Errorf("SelectAV01Variant(720p) = %q, want 720p", sel.Label)
	}
	if sel := SelectAV01Variant(variants, "480p"); sel.Label != "360p" {
		t.Errorf("SelectAV01Variant(480p) = %q, want 360p", sel.Label)
	}
}

func TestRewriteM3U8Content(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-MAP:URI="init-sv3-v1-a1.mp4?access_token=tok"
#EXT-X-KEY:METHOD=AES-128,URI="https://keys.iw01.xyz/k.bin",IV=0x01
#EXTINF:4.0,
https://customers.iw01.xyz/fmp4/226601/file-1.m4s?access_token=tok
segment-2.m4s`

	baseURL := "https://customers.iw01.xyz/api/v1/videos/226601/manifest/index1080.m3u8?access_token=tok"
	rewritten := rewriteM3U8Content(playlist, baseURL, "https://av01.media/")

	if !strings.Contains(rewritten, `#EXT-X-MAP:URI="/api/browser/proxy-media?asset=1&referer=https%3A%2F%2Fav01.media%2F&url=https%3A%2F%2Fcustomers.iw01.xyz%2Fapi%2Fv1%2Fvideos%2F226601%2Fmanifest%2Finit-sv3-v1-a1.mp4%3Faccess_token%3Dtok"`) {
		t.Fatalf("expected #EXT-X-MAP URI to be proxy-rewritten, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, `#EXT-X-KEY:METHOD=AES-128,URI="/api/browser/proxy-media?asset=1&referer=https%3A%2F%2Fav01.media%2F&url=https%3A%2F%2Fkeys.iw01.xyz%2Fk.bin",IV=0x01`) {
		t.Fatalf("expected #EXT-X-KEY URI to be proxy-rewritten, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, `/api/browser/proxy-media?asset=1&referer=https%3A%2F%2Fav01.media%2F&url=https%3A%2F%2Fcustomers.iw01.xyz%2Ffmp4%2F226601%2Ffile-1.m4s%3Faccess_token%3Dtok`) {
		t.Fatalf("expected segment URL to be proxy-rewritten, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, `access_token%3Dtok`) {
		t.Fatalf("expected access_token preserved for relative segment, got:\n%s", rewritten)
	}
}
