package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInfoCacheSaveAndGet(t *testing.T) {
	testURL := "https://www.youtube.com/watch?v=test_fast_start_123"
	testJSON := []byte(`{"id":"test_fast_start_123","title":"Fast Start Test Video","formats":[]}`)

	// 1. Save valid JSON
	savedPath, err := SaveCachedInfoJSON(testURL, testJSON)
	if err != nil {
		t.Fatalf("expected SaveCachedInfoJSON to succeed, got %v", err)
	}
	if savedPath == "" {
		t.Fatalf("expected non-empty savedPath")
	}

	// 2. Retrieve cached path
	retrievedPath, ok := GetCachedInfoJSON(testURL)
	if !ok {
		t.Fatalf("expected GetCachedInfoJSON to return true")
	}
	if retrievedPath != savedPath {
		t.Errorf("expected path %s, got %s", savedPath, retrievedPath)
	}

	// 3. Verify content
	content, err := os.ReadFile(retrievedPath)
	if err != nil {
		t.Fatalf("failed to read cached file: %v", err)
	}
	if string(content) != string(testJSON) {
		t.Errorf("expected cached content %s, got %s", string(testJSON), string(content))
	}

	// 4. Test Invalidation
	InvalidateCachedInfoJSON(testURL)
	_, okAfterDelete := GetCachedInfoJSON(testURL)
	if okAfterDelete {
		t.Errorf("expected GetCachedInfoJSON to return false after invalidation")
	}
}

func TestInfoCacheInvalidJSON(t *testing.T) {
	badURL := "https://www.youtube.com/watch?v=bad_json"
	badJSON := []byte(`not valid json {[[`)

	_, err := SaveCachedInfoJSON(badURL, badJSON)
	if err == nil {
		t.Errorf("expected SaveCachedInfoJSON to fail on malformed JSON, got nil")
	}
}

func TestScoreVideo(t *testing.T) {
	// MP4 AVC1 should beat WebM VP9 and WebM AV1 at the same bitrate
	avcMP4 := RawFormat{
		FormatID: "137",
		Ext:      "mp4",
		VCodec:   "avc1.640028",
		TBR:      3000,
		Height:   1080,
	}

	vp9WebM := RawFormat{
		FormatID: "248",
		Ext:      "webm",
		VCodec:   "vp9",
		TBR:      3000,
		Height:   1080,
	}

	av1MP4 := RawFormat{
		FormatID: "399",
		Ext:      "mp4",
		VCodec:   "av01.0.08M.08",
		TBR:      3000,
		Height:   1080,
	}

	scoreAVC := ScoreVideo(avcMP4)
	scoreVP9 := ScoreVideo(vp9WebM)
	scoreAV1 := ScoreVideo(av1MP4)

	// avcMP4: 3000 + 10000 (mp4) + 5000 (avc) = 18000
	// vp9WebM: 3000 = 3000
	// av1MP4: 3000 + 10000 (mp4) = 13000
	if scoreAVC <= scoreVP9 {
		t.Errorf("expected AVC MP4 score (%f) to be higher than VP9 WebM score (%f)", scoreAVC, scoreVP9)
	}
	if scoreAVC <= scoreAV1 {
		t.Errorf("expected AVC MP4 score (%f) to be higher than AV1 MP4 score (%f)", scoreAVC, scoreAV1)
	}
}

func TestBuildFormatChoices(t *testing.T) {
	formats := []RawFormat{
		{FormatID: "140", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 128, Filesize: 4 * 1024 * 1024},
		{FormatID: "251", Ext: "webm", ACodec: "opus", VCodec: "none", ABR: 160, Filesize: 5 * 1024 * 1024},
		{FormatID: "137", Ext: "mp4", VCodec: "avc1.640028", ACodec: "none", Height: 1080, TBR: 3000, Filesize: 40 * 1024 * 1024},
		{FormatID: "248", Ext: "webm", VCodec: "vp9", ACodec: "none", Height: 1080, TBR: 3100, Filesize: 38 * 1024 * 1024},
		{FormatID: "136", Ext: "mp4", VCodec: "avc1.4d401f", ACodec: "none", Height: 720, TBR: 1500, Filesize: 20 * 1024 * 1024},
	}

	choices := BuildFormatChoices(formats)
	if len(choices) == 0 {
		t.Fatalf("expected non-empty format choices")
	}

	// 1080p, 720p, and Audio Only
	var has1080, has720, hasAudio bool
	for _, c := range choices {
		if c.Height == 1080 {
			has1080 = true
			if !strings.Contains(c.Label, "1080p") {
				t.Errorf("expected label to contain 1080p, got %s", c.Label)
			}
			// Size should be video size (40MB) + best audio size (5MB from opus 160kbps) = 45MB
			if c.EstimatedSize <= 0 {
				t.Errorf("expected non-zero estimated size for 1080p")
			}
		}
		if c.Height == 720 {
			has720 = true
		}
		if c.Kind == "audio" {
			hasAudio = true
			if !strings.Contains(c.Label, "Audio Only") {
				t.Errorf("expected audio label, got %s", c.Label)
			}
		}
	}

	if !has1080 || !has720 || !hasAudio {
		t.Errorf("missing expected choices: has1080=%t, has720=%t, hasAudio=%t", has1080, has720, hasAudio)
	}
}

func TestBuildYtDlpFormatSelector(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"1080", "bv*[height=1080]+ba/b[height=1080]/bv*[height<=1080]+ba/b"},
		{"1080p", "bv*[height=1080]+ba/b[height=1080]/bv*[height<=1080]+ba/b"},
		{"720", "bv*[height=720]+ba/b[height=720]/bv*[height<=720]+ba/b"},
		{"4k", "bv*[height=2160]+ba/b[height=2160]/bv*[height<=2160]+ba/b"},
		{"highest", "bestvideo+bestaudio/best"},
	}

	for _, tc := range cases {
		got := BuildYtDlpFormatSelector(tc.input)
		if got != tc.want {
			t.Errorf("BuildYtDlpFormatSelector(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPruneExpiredInfoCache(t *testing.T) {
	dir := GetInfoCacheDir()
	expiredFile := filepath.Join(dir, "info-expired-test.json")
	if err := os.WriteFile(expiredFile, []byte(`{"test":true}`), 0644); err != nil {
		t.Fatalf("failed to create expired test file: %v", err)
	}

	// Change mtime to 10 hours ago
	oldTime := time.Now().Add(-10 * time.Hour)
	if err := os.Chtimes(expiredFile, oldTime, oldTime); err != nil {
		t.Fatalf("failed to change mtime: %v", err)
	}

	PruneExpiredInfoCache(4 * time.Hour)

	if _, err := os.Stat(expiredFile); !os.IsNotExist(err) {
		t.Errorf("expected expired file to be deleted by pruner")
		_ = os.Remove(expiredFile)
	}
}
