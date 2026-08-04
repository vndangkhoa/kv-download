package media

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	cookiesEnvBrowser = "MR_COOKIES_BROWSER"
	cookiesEnvURL     = "MR_COOKIES_URL"
	cookiesEnvMaxAge  = "MR_COOKIES_MAX_AGE_HOURS"
)

var supportedBrowsers = []string{"chrome", "chromium", "firefox", "edge", "brave", "opera", "vivaldi", "safari"}

// RefreshCookiesFromBrowser regenerates cookies.txt from the configured browser
// if the file is missing or stale. Returns true if a cookies.txt is available afterwards.
func RefreshCookiesFromBrowser() bool {
	cookiesPath := getCookiesPath()

	if cookiesFileFresh(cookiesPath) {
		return true
	}

	browser := strings.TrimSpace(os.Getenv(cookiesEnvBrowser))
	if browser == "" {
		log.Info().Msgf("cookies.txt is missing or stale and %s is not set — skipping auto-refresh", cookiesEnvBrowser)
		return false
	}

	if !isSupportedBrowser(browser) {
		log.Warn().Msgf("unsupported browser %q for %s — skipping auto-refresh", browser, cookiesEnvBrowser)
		return false
	}

	log.Info().Msgf("Refreshing cookies.txt from browser %q...", browser)
	if err := extractCookiesFromBrowser(browser, cookiesPath); err != nil {
		log.Warn().Err(err).Msg("cookie extraction failed")
		return false
	}
	log.Info().Msgf("cookies.txt refreshed from browser %q", browser)
	return true
}

func cookiesFileFresh(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	maxAge := 24 * 7 // hours, default 7 days
	if v := strings.TrimSpace(os.Getenv(cookiesEnvMaxAge)); v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil && n > 0 {
			maxAge = n
		}
	}

	if time.Since(info.ModTime()) < time.Duration(maxAge)*time.Hour {
		return true
	}
	log.Info().Msgf("cookies.txt is older than %d hours — refreshing", maxAge)
	return false
}

func extractCookiesFromBrowser(browser, cookiesPath string) error {
	url := strings.TrimSpace(os.Getenv(cookiesEnvURL))
	if url == "" {
		url = "https://www.tiktok.com/"
	}

	args := []string{
		"--cookies-from-browser", browser,
		"--cookies", cookiesPath,
		"--skip-download",
		url,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug().Msgf("yt-dlp cookie extraction output: %s", output)
	}

	if _, statErr := os.Stat(cookiesPath); statErr != nil {
		return statErr
	}
	return nil
}

func isSupportedBrowser(browser string) bool {
	for _, b := range supportedBrowsers {
		if b == browser {
			return true
		}
	}
	return false
}
