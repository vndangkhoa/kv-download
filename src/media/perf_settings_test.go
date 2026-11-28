package media

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func isolatePerfDownloadDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	if !stringsHasSuffixSep(tmpDir) {
		tmpDir += string(filepath.Separator)
	}

	downloadDirMu.Lock()
	prevCustom := customDownloadDir
	prevDir := downloadDir
	customDownloadDir = tmpDir
	downloadDir = tmpDir
	downloadDirMu.Unlock()

	t.Setenv("MR_DOWNLOAD_DIR", tmpDir)
	resetPerformanceSettingsCacheForTest()

	t.Cleanup(func() {
		resetPerformanceSettingsCacheForTest()
		downloadDirMu.Lock()
		customDownloadDir = prevCustom
		downloadDir = prevDir
		downloadDirMu.Unlock()
	})
	return tmpDir
}

func stringsHasSuffixSep(p string) bool {
	return len(p) > 0 && p[len(p)-1] == filepath.Separator
}

func TestPerformanceSettingsDefaultsClampAndPersist(t *testing.T) {
	tmpDir := isolatePerfDownloadDir(t)

	// 1. Verify defaults
	def := GetPerformanceSettings()
	if def.MaxWorkers != 4 {
		t.Fatalf("expected default MaxWorkers=4, got %d", def.MaxWorkers)
	}
	if def.ConnectionsPerTask != 24 {
		t.Fatalf("expected default ConnectionsPerTask=24, got %d", def.ConnectionsPerTask)
	}
	if def.RateLimit != "" {
		t.Fatalf("expected default RateLimit=\"\", got %q", def.RateLimit)
	}
	if !def.NativeHLS {
		t.Fatalf("expected default NativeHLS=true")
	}

	// 2. Verify clamping + persistence
	hiWorkers := 99
	loConns := 2
	rl := "unlimited"
	nhls := false
	updated, err := UpdatePerformanceSettings(PerformanceSettingsPatch{
		MaxWorkers:         &hiWorkers,
		ConnectionsPerTask: &loConns,
		RateLimit:          &rl,
		NativeHLS:          &nhls,
	})
	if err != nil {
		t.Fatalf("UpdatePerformanceSettings failed: %v", err)
	}
	if updated.MaxWorkers != 16 {
		t.Fatalf("expected clamped MaxWorkers=16, got %d", updated.MaxWorkers)
	}
	if updated.ConnectionsPerTask != 4 {
		t.Fatalf("expected clamped ConnectionsPerTask=4, got %d", updated.ConnectionsPerTask)
	}
	if updated.RateLimit != "" {
		t.Fatalf("expected normalized RateLimit=\"\", got %q", updated.RateLimit)
	}
	if updated.NativeHLS {
		t.Fatalf("expected NativeHLS=false after update")
	}

	// Clear memory cache and verify reload from disk
	resetPerformanceSettingsCacheForTest()
	if _, statErr := os.Stat(filepath.Join(tmpDir, "performance_settings.json")); statErr != nil {
		t.Fatalf("expected persisted performance_settings.json: %v", statErr)
	}
	reloaded := GetPerformanceSettings()
	if reloaded != updated {
		t.Fatalf("reloaded settings %+v != updated %+v", reloaded, updated)
	}

	// 3. HTTP GET / POST handlers
	postBody := []byte(`{"maxWorkers":6,"connectionsPerTask":99,"rateLimit":"25M","nativeHls":true}`)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/settings/performance", bytes.NewReader(postBody))
	recPost := httptest.NewRecorder()
	SetPerformanceSettingsHandler(recPost, reqPost)
	if recPost.Code != http.StatusOK {
		t.Fatalf("POST /api/settings/performance status=%d body=%s", recPost.Code, recPost.Body.String())
	}

	var postResp PerformanceSettingsResponse
	if err := json.Unmarshal(recPost.Body.Bytes(), &postResp); err != nil {
		t.Fatalf("decode POST response: %v", err)
	}
	if !postResp.Success || postResp.MaxWorkers != 6 || postResp.ConnectionsPerTask != 48 || postResp.RateLimit != "25M" || !postResp.NativeHLS {
		t.Fatalf("unexpected POST response: %+v", postResp)
	}

	reqGet := httptest.NewRequest(http.MethodGet, "/api/settings/performance", nil)
	recGet := httptest.NewRecorder()
	GetPerformanceSettingsHandler(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("GET /api/settings/performance status=%d", recGet.Code)
	}
	var getResp PerformanceSettingsResponse
	if err := json.Unmarshal(recGet.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if getResp.MaxWorkers != 6 || getResp.ConnectionsPerTask != 48 || getResp.RateLimit != "25M" || !getResp.NativeHLS {
		t.Fatalf("unexpected GET response: %+v", getResp)
	}
}

func TestParseRateLimitBytesPerSec(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"10M", 10 * 1024 * 1024},
		{"500K", 500 * 1024},
		{"100MB", 100 * 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{"unlimited", 0},
		{"0", 0},
		{"", 0},
	}

	for _, tc := range cases {
		if got := parseRateLimitBytesPerSec(tc.in); got != tc.want {
			t.Errorf("parseRateLimitBytesPerSec(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
