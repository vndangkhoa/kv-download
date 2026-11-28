package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseXStatusURL(t *testing.T) {
	cases := []struct {
		url      string
		wantUser string
		wantID   string
		wantIdx  int
		wantOK   bool
	}{
		{
			url:      "https://x.com/qqzzllyy/status/2126583986168488147?a=6E90jXRCBI7YPx1mZbo&s=19",
			wantUser: "qqzzllyy",
			wantID:   "2126583986168488147",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			url:      "https://mobile.x.com/qqzzllyy/status/2126583986168488147/video/2",
			wantUser: "qqzzllyy",
			wantID:   "2126583986168488147",
			wantIdx:  2,
			wantOK:   true,
		},
		{
			url:      "https://twitter.com/jack/statuses/1234567890",
			wantUser: "jack",
			wantID:   "1234567890",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			url:      "https://fxtwitter.com/i/status/2126583986168488147",
			wantUser: "i",
			wantID:   "2126583986168488147",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			url:      "https://x.com/i/web/status/2126583986168488147",
			wantUser: "i",
			wantID:   "2126583986168488147",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			url:      "https://vxtwitter.com/someuser/status/987654321",
			wantUser: "someuser",
			wantID:   "987654321",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			url:      "https://fixupx.com/someuser/status/987654321",
			wantUser: "someuser",
			wantID:   "987654321",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			url:    "https://x.com/home",
			wantOK: false,
		},
		{
			url:    "https://x.com/qqzzllyy",
			wantOK: false,
		},
		{
			url:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		user, id, idx, ok := ParseXStatusURL(tc.url)
		if ok != tc.wantOK || user != tc.wantUser || id != tc.wantID || idx != tc.wantIdx {
			t.Errorf("ParseXStatusURL(%q) = (%q, %q, %d, %v), want (%q, %q, %d, %v)",
				tc.url, user, id, idx, ok, tc.wantUser, tc.wantID, tc.wantIdx, tc.wantOK)
		}
	}
}

func TestResolveXVideoFxTwitterPrimaryAndVariants(t *testing.T) {
	fxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/qqzzllyy/status/2126583986168488147" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != xTwitterUserAgent {
			t.Fatalf("expected User-Agent %q, got %q", xTwitterUserAgent, ua)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"message": "OK",
			"tweet": {
				"url": "https://x.com/qqzzllyy/status/2126583986168488147",
				"id": "2126583986168488147",
				"text": "Test caption https://t.co/RZEEUrFil1",
				"author": {"name": "Test Author", "screen_name": "qqzzllyy"},
				"media": {
					"videos": [{
						"id": "2126583831310548992",
						"url": "https://video.twimg.com/amplify_video/2126583831310548992/vid/avc1/448x796/7BA28qwJLCkMsM2h.mp4?tag=29",
						"thumbnail_url": "https://pbs.twimg.com/amplify_video_thumb/2126583831310548992/img/by3amp8JGwFkgXD4.jpg",
						"duration": 46.788,
						"width": 448,
						"height": 796,
						"format": "video/mp4",
						"type": "video",
						"variants": [
							{"url": "https://video.twimg.com/amplify_video/2126583831310548992/pl/NCL4wt0QLwK-n-Xn.m3u8?tag=29", "bitrate": 0, "content_type": "application/x-mpegURL"},
							{"url": "https://video.twimg.com/amplify_video/2126583831310548992/vid/avc1/320x568/awFr8Ml0ZgPFns4O.mp4?tag=29", "bitrate": 632000, "content_type": "video/mp4"},
							{"url": "https://video.twimg.com/amplify_video/2126583831310548992/vid/avc1/448x796/7BA28qwJLCkMsM2h.mp4?tag=29", "bitrate": 950000, "content_type": "video/mp4"}
						]
					}]
				}
			}
		}`))
	}))
	defer fxSrv.Close()

	origFx, origVx := fxTwitterAPIBase, vxTwitterAPIBase
	fxTwitterAPIBase = fxSrv.URL
	vxTwitterAPIBase = "http://127.0.0.1:0"
	defer func() {
		fxTwitterAPIBase = origFx
		vxTwitterAPIBase = origVx
	}()

	resolved, err := ResolveXVideo(context.Background(), "https://x.com/qqzzllyy/status/2126583986168488147?a=6E90jXRCBI7YPx1mZbo&s=19")
	if err != nil {
		t.Fatalf("ResolveXVideo failed: %v", err)
	}
	if resolved.Channel != "@qqzzllyy" {
		t.Errorf("Channel = %q, want @qqzzllyy", resolved.Channel)
	}
	if resolved.Title != "Test caption" {
		t.Errorf("Title = %q, want %q", resolved.Title, "Test caption")
	}
	if len(resolved.Videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(resolved.Videos))
	}
	vid := resolved.Videos[0]
	if len(vid.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(vid.Variants))
	}
	if vid.Variants[0].Label != "448p" || vid.Variants[0].IsHLS {
		t.Errorf("expected top variant 448p MP4, got %+v", vid.Variants[0])
	}
	if vid.Variants[1].Label != "320p" {
		t.Errorf("expected second variant 320p, got %+v", vid.Variants[1])
	}
	if !vid.Variants[2].IsHLS {
		t.Errorf("expected HLS variant sorted last, got %+v", vid.Variants[2])
	}

	if sel := SelectXVariant(vid.Variants, "highest"); sel.Label != "448p" {
		t.Errorf("SelectXVariant(highest) = %q, want 448p", sel.Label)
	}
	if sel := SelectXVariant(vid.Variants, "720p"); sel.Label != "448p" {
		t.Errorf("SelectXVariant(720p) = %q, want 448p", sel.Label)
	}
	if sel := SelectXVariant(vid.Variants, "480p"); sel.Label != "448p" {
		t.Errorf("SelectXVariant(480p) = %q, want 448p", sel.Label)
	}
	if sel := SelectXVariant(vid.Variants, "360p"); sel.Label != "320p" {
		t.Errorf("SelectXVariant(360p) = %q, want 320p", sel.Label)
	}
}

func TestResolveXVideoVxTwitterFallback(t *testing.T) {
	fxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream error", http.StatusBadGateway)
	}))
	defer fxSrv.Close()

	vxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != xTwitterUserAgent {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tweetID": "2126583986168488147",
			"text": "Fallback tweet",
			"user_name": "Author",
			"user_screen_name": "qqzzllyy",
			"media_extended": [{
				"duration_millis": 46788,
				"id_str": "2126583831310548992",
				"size": {"width": 448, "height": 796},
				"thumbnail_url": "https://pbs.twimg.com/thumb.jpg",
				"type": "video",
				"url": "https://video.twimg.com/amplify_video/2126583831310548992/vid/avc1/448x796/7BA28qwJLCkMsM2h.mp4"
			}]
		}`))
	}))
	defer vxSrv.Close()

	origFx, origVx := fxTwitterAPIBase, vxTwitterAPIBase
	fxTwitterAPIBase = fxSrv.URL
	vxTwitterAPIBase = vxSrv.URL
	defer func() {
		fxTwitterAPIBase = origFx
		vxTwitterAPIBase = origVx
	}()

	resolved, err := ResolveXVideo(context.Background(), "https://x.com/qqzzllyy/status/2126583986168488147")
	if err != nil {
		t.Fatalf("expected VxTwitter fallback to succeed, got %v", err)
	}
	if len(resolved.Videos) != 1 || !strings.Contains(resolved.Videos[0].URL, "448x796") {
		t.Fatalf("unexpected fallback videos: %+v", resolved.Videos)
	}
	if resolved.Videos[0].Duration < 46.7 || resolved.Videos[0].Duration > 46.8 {
		t.Errorf("Duration = %v, want ~46.788", resolved.Videos[0].Duration)
	}
}

func TestResolveXVideoQuoteAndMultiVideoSelection(t *testing.T) {
	fxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"tweet": {
				"id": "555",
				"text": "Check this out",
				"author": {"name": "Quoter", "screen_name": "quoter"},
				"media": {"all": [], "videos": []},
				"quote": {
					"id": "444",
					"text": "Quoted",
					"media": {
						"videos": [
							{
								"id": "v1",
								"url": "https://video.twimg.com/ext_tw_video/1/pu/vid/avc1/1280x720/a.mp4",
								"width": 1280, "height": 720, "type": "video"
							},
							{
								"id": "v2",
								"url": "https://video.twimg.com/ext_tw_video/2/pu/vid/avc1/1920x1080/b.mp4",
								"width": 1920, "height": 1080, "type": "gif"
							}
						]
					}
				}
			}
		}`))
	}))
	defer fxSrv.Close()

	origFx, origVx := fxTwitterAPIBase, vxTwitterAPIBase
	fxTwitterAPIBase = fxSrv.URL
	vxTwitterAPIBase = "http://127.0.0.1:0"
	defer func() {
		fxTwitterAPIBase = origFx
		vxTwitterAPIBase = origVx
	}()

	resolved, err := ResolveXVideo(context.Background(), "https://x.com/quoter/status/555/video/2")
	if err != nil {
		t.Fatalf("ResolveXVideo quote fallback failed: %v", err)
	}
	if len(resolved.Videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(resolved.Videos))
	}
	v2, ok := SelectXVideo(resolved, 2)
	if !ok || v2.ID != "v2" || !v2.IsGif || !strings.Contains(v2.Title, "(2/2)") {
		t.Fatalf("SelectXVideo(2) = (%+v, %v)", v2, ok)
	}
}
