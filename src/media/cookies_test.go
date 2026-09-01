package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeCookiesToNetscape(t *testing.T) {
	// 1. Netscape input
	netscape := "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tFALSE\t1772451000\tSID\tabc123\n"
	res1 := NormalizeCookiesToNetscape(netscape, "https://www.youtube.com/watch?v=123")
	if !strings.Contains(res1, "SID") || !strings.Contains(res1, "abc123") {
		t.Errorf("Expected SID and abc123 in netscape output, got: %s", res1)
	}

	// 2. JSON Array input (from Cookie-Editor)
	jsonArray := `[
		{"name": "sessionid", "value": "xyz789", "domain": ".instagram.com", "path": "/", "secure": true},
		{"name": "ds_user_id", "value": "123456", "domain": ".threads.net", "path": "/"}
	]`
	res2 := NormalizeCookiesToNetscape(jsonArray, "https://www.threads.net/@user/post/123")
	if !strings.Contains(res2, "sessionid") || !strings.Contains(res2, "xyz789") || !strings.Contains(res2, ".instagram.com") {
		t.Errorf("Expected parsed json array cookies, got: %s", res2)
	}

	// 3. Raw header input
	rawHeader := "Cookie: sessionid=my_secret_token; ds_user_id=888999; csrftoken=token123"
	res3 := NormalizeCookiesToNetscape(rawHeader, "https://www.threads.net/@user/post/abc")
	if !strings.Contains(res3, "sessionid") || !strings.Contains(res3, "my_secret_token") || !strings.Contains(res3, ".threads.net") {
		t.Errorf("Expected parsed header cookies with .threads.net domain, got: %s", res3)
	}

	// 4. JSON Key-Value object
	jsonObj := `{"sessionid": "token_abc", "auth": "yes"}`
	res4 := NormalizeCookiesToNetscape(jsonObj, "https://www.tiktok.com/@creator/video/123")
	if !strings.Contains(res4, "sessionid") || !strings.Contains(res4, "token_abc") || !strings.Contains(res4, ".tiktok.com") {
		t.Errorf("Expected parsed key-value json cookies with .tiktok.com domain, got: %s", res4)
	}
}

func TestCreateEphemeralCookieFile(t *testing.T) {
	cookies := "sessionid=test_token_123; user_id=456"
	path, cleanup, err := CreateEphemeralCookieFile(cookies, "https://www.threads.net/@user/post/123")
	if err != nil {
		t.Fatalf("Unexpected error creating ephemeral cookie file: %v", err)
	}
	if path == "" {
		t.Fatal("Expected non-empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read created cookie file: %v", err)
	}
	if !strings.Contains(string(data), "test_token_123") {
		t.Fatalf("Expected token in file, got %s", string(data))
	}

	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Expected file to be deleted after cleanup, but it still exists")
	}
}

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
