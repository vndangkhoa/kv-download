package media

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

const DefaultAppVersion = "1.0.46"

// AppVersionInfo describes the latest Android client release metadata served to mobile clients.
type AppVersionInfo struct {
	Version        string `json:"version"`
	ApkURL         string `json:"apkUrl"`
	LatestApkURL   string `json:"latestApkUrl"`
	ReleasePageURL string `json:"releasePageUrl"`
	Notes          string `json:"notes"`
}

// GetAppVersion returns the running release version from APP_VERSION (trimmed of a leading v)
// or DefaultAppVersion when unset.
func GetAppVersion() string {
	v := strings.TrimSpace(os.Getenv("APP_VERSION"))
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if v == "" || strings.EqualFold(v, "latest") {
		return DefaultAppVersion
	}
	return v
}

// AppVersionHandler responds with GET /api/app/version for Android in-app update checks.
func AppVersionHandler(w http.ResponseWriter, r *http.Request) {
	ver := GetAppVersion()
	info := AppVersionInfo{
		Version:        ver,
		ApkURL:         "https://syno.vndns.net/kvdownload-v" + ver + ".apk",
		LatestApkURL:   "https://syno.vndns.net/kvdownload-release.apk",
		ReleasePageURL: "https://syno.vndns.net/package/kvdownload",
		Notes:          "Low-CPU Chromium Core browser (tini + 7fps paced stream, 0% idle), edge-to-edge fit-view & maximized browser chrome.",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}
