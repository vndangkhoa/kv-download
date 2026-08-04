package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshCookiesFromBrowser(t *testing.T) {
	if os.Getenv("MR_COOKIES_BROWSER") == "" {
		t.Skip("MR_COOKIES_BROWSER not set")
	}
	root, _ := filepath.Abs("../..")
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	os.Setenv("MR_COOKIES_MAX_AGE_HOURS", "0")
	if !RefreshCookiesFromBrowser() {
		t.Fatal("refresh failed")
	}
	if _, err := os.Stat(getCookiesPath()); err != nil {
		t.Fatalf("cookies.txt not created: %v", err)
	}
}
