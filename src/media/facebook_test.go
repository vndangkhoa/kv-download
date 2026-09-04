package media

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestIsFacebookVideoURL(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"https://www.facebook.com/Linhchanh.2k", false},
		{"https://www.facebook.com/Linhchanh.2k/videos", false},
		{"https://www.facebook.com/Linhchanh.2k/videos/", false},
		{"https://www.facebook.com/Linhchanh.2k/videos/10151234567890/", true},
		{"https://www.facebook.com/watch?v=1234567890", true},
		{"https://www.facebook.com/Linhchanh.2k/posts/1234567890", true},
		{"https://www.facebook.com/Linhchanh.2k/reel/1234567890", true},
		{"https://www.facebook.com/Linhchanh.2k/video.php?v=1234567890", true},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", false},
		{"https://m.facebook.com/Linhchanh.2k", false},
		{"https://www.facebook.com/Linhchanh.2k/about", false},
	}
	for _, tc := range cases {
		got := IsFacebookVideoURL(tc.input)
		if got != tc.want {
			t.Errorf("IsFacebookVideoURL(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsFacebookProfileURL(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"https://www.facebook.com/Linhchanh.2k", true},
		{"https://www.facebook.com/Linhchanh.2k/", true},
		{"https://www.facebook.com/Linhchanh.2k/videos", true},
		{"https://www.facebook.com/Linhchanh.2k/videos/", true},
		{"https://www.facebook.com/pages/MyPage/1234567890", true},
		{"https://www.facebook.com/groups/mygrouppage", true},
		{"https://www.facebook.com/profile.php?id=61592464987497", true},
		{"https://www.facebook.com/Linhchanh.2k/videos/10151234567890/", false},
		{"https://www.facebook.com/watch?v=1234567890", false},
		{"https://www.youtube.com", false},
		{"", false},
		{"not-a-url", false},
	}
	for _, tc := range cases {
		got := IsFacebookProfileURL(tc.input)
		if got != tc.want {
			t.Errorf("IsFacebookProfileURL(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeFacebookVideosURL(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"https://www.facebook.com/Linhchanh.2k", "https://m.facebook.com/Linhchanh.2k/videos/", false},
		{"https://www.facebook.com/Linhchanh.2k/", "https://m.facebook.com/Linhchanh.2k/videos/", false},
		{"https://www.facebook.com/Linhchanh.2k/videos", "https://m.facebook.com/Linhchanh.2k/videos/", false},
		{"https://www.facebook.com/Linhchanh.2k/videos/", "https://m.facebook.com/Linhchanh.2k/videos/", false},
		{"https://www.facebook.com/pages/MyPage/1234567890", "https://m.facebook.com/pages/MyPage/1234567890/reels/", false},
		{"https://www.facebook.com/groups/mygrouppage", "https://m.facebook.com/groups/mygrouppage/reels/", false},
		{"https://www.facebook.com/profile.php?id=61592464987497", "https://www.facebook.com/profile.php?id=61592464987497&sk=reels_tab", false},
		{"https://www.fb.com/Linhchanh.2k", "", true},
		{"https://www.youtube.com/watch?v=123", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeFacebookVideosURL(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("normalizeFacebookVideosURL(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("normalizeFacebookVideosURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFbCanonicalVideoUrl(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/Linhchanh.2k/videos/10151234567890/", "https://www.facebook.com/Linhchanh.2k/videos/10151234567890/"},
		{"//m.facebook.com/Linhchanh.2k/videos/10151234567890/", "https://www.facebook.com/Linhchanh.2k/videos/10151234567890/"},
		{"https://m.facebook.com/Linhchanh.2k/videos/10151234567890/", "https://www.facebook.com/Linhchanh.2k/videos/10151234567890/"},
		{"/video.php?v=1234567890", "https://www.facebook.com/video.php?v=1234567890"},
	}
	for _, tc := range cases {
		got := fbCanonicalVideoUrl(tc.input)
		if got != tc.want {
			t.Errorf("fbCanonicalVideoUrl(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFbCanonicalLinkUrl(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/Linhchanh.2k/videos/?cursor=abc123", "https://m.facebook.com/Linhchanh.2k/videos/?cursor=abc123"},
		{"//m.facebook.com/Linhchanh.2k/videos/?cursor=abc123", "https://m.facebook.com/Linhchanh.2k/videos/?cursor=abc123"},
		{"https://m.fb.com/Linhchanh.2k/videos/?cursor=abc123", "https://m.facebook.com/Linhchanh.2k/videos/?cursor=abc123"},
	}
	for _, tc := range cases {
		got := fbCanonicalLinkUrl(tc.input)
		if got != tc.want {
			t.Errorf("fbCanonicalLinkUrl(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseFbCookieHeader(t *testing.T) {
	netscape := `# Netscape HTTP Cookie File
.facebook.com	TRUE	/	TRUE	1772451000	c_user	123456
.fbcdn.net	FALSE	/	FALSE	1772451000	cs	abc123
.youtube.com	TRUE	/	FALSE	1772451000	SID	ignoreme
`
	got := parseNetscapeCookiesForFb(netscape)
	if !strings.Contains(got, "c_user=123456") {
		t.Errorf("expected c_user cookie, got: %s", got)
	}
	if !strings.Contains(got, "cs=abc123") {
		t.Errorf("expected fbcdn cookie, got: %s", got)
	}
	if strings.Contains(got, "SID") || strings.Contains(got, "ignoreme") {
		t.Errorf("youtube cookie leaked into fb header: %s", got)
	}
}

func TestParseFbVideoLinks(t *testing.T) {
	html := `<html><body>
<a href="/Linhchanh.2k/videos/10151234567890/"><img src="https://scontent.fbcdn.net/v1/thumb1.jpg" /></a>
<a href="/Linhchanh.2k/videos/10151234567891/"><img src="https://scontent.fbcdn.net/v1/thumb2.jpg" /></a>
<a href="/Linhchanh.2k/videos/?cursor=abc123">See More</a>
</body></html>`

	entries, nextPages := parseFbVideoLinks("Linhchanh.2k", html)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Url != "https://www.facebook.com/Linhchanh.2k/videos/10151234567890" {
		t.Errorf("entry 0 url = %q", entries[0].Url)
	}
	if entries[1].Url != "https://www.facebook.com/Linhchanh.2k/videos/10151234567891" {
		t.Errorf("entry 1 url = %q", entries[1].Url)
	}
	if entries[0].Thumbnail != "https://scontent.fbcdn.net/v1/thumb1.jpg" {
		t.Errorf("entry 0 thumb = %q", entries[0].Thumbnail)
	}
	if entries[0].Category != "video" {
		t.Errorf("entry 0 category = %q, want %q", entries[0].Category, "video")
	}
	if len(nextPages) == 0 || nextPages[0] != "https://m.facebook.com/Linhchanh.2k/videos/?cursor=abc123" {
		t.Errorf("nextPages = %v", nextPages)
	}
}

func TestCategorizeFbHref(t *testing.T) {
	cases := []struct {
		href  string
		wantC string
		wantK string
	}{
		{"/Linhchanh.2k/videos/10151234567890/", "https://www.facebook.com/Linhchanh.2k/videos/10151234567890", "video"},
		{"/Linhchanh.2k/reel/10151234567890/", "https://www.facebook.com/Linhchanh.2k/reel/10151234567890", "reel"},
		{"/Linhchanh.2k/reels/10151234567890/", "https://www.facebook.com/Linhchanh.2k/reel/10151234567890", "reel"},
		{"/Linhchanh.2k/posts/10151234567890/", "https://www.facebook.com/Linhchanh.2k/posts/10151234567890", "post"},
		{"https://www.facebook.com/watch?v=10151234567890", "https://www.facebook.com/watch?v=10151234567890", "video"},
		{"https://www.facebook.com/video.php?v=10151234567890", "https://www.facebook.com/watch?v=10151234567890", "video"},
		{"/Linhchanh.2k/about", "", ""},
	}
	for _, tc := range cases {
		canonical, vid, kind := categorizeFbHref(tc.href)
		if canonical != tc.wantC {
			t.Errorf("categorizeFbHref(%q) canonical = %q, want %q", tc.href, canonical, tc.wantC)
		}
		if kind != tc.wantK {
			t.Errorf("categorizeFbHref(%q) kind = %q, want %q (vid=%s)", tc.href, kind, tc.wantK, vid)
		}
	}
}

func TestIsFbPaginationLink(t *testing.T) {
	cases := []struct {
		href string
		want bool
	}{
		{"https://m.facebook.com/Linhchanh.2k/videos/?cursor=abc", true},
		{"https://m.facebook.com/Linhchanh.2k/videos/?section=1", true},
		{"https://m.facebook.com/Linhchanh.2k/videos/?after=xyz", true},
		{"https://m.facebook.com/Linhchanh.2k/videos/?ref=page_internal", true},
		{"https://m.facebook.com/Linhchanh.2k/videos/?multi_permalinks=1", true},
		{"https://m.facebook.com/Linhchanh.2k/videos/?see_more=1", true},
		{"https://www.facebook.com/Linhchanh.2k/videos/1234567890/", false},
		{"https://www.facebook.com/login/", false},
		{"https://www.facebook.com/watch?v=1234567890", false},
	}
	for _, tc := range cases {
		got := isFbPaginationLink("Linhchanh.2k", strings.ToLower(tc.href))
		if got != tc.want {
			t.Errorf("isFbPaginationLink(%q) = %v, want %v", tc.href, got, tc.want)
		}
	}
}

func TestLiveFacebookScrape(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	info, errMsg, err := ScrapeFacebookVideos("https://www.facebook.com/Linhchanh.2k", "", 1, 24)
	if err != nil {
		t.Fatalf("ScrapeFacebookVideos error: %v (errMsg: %s)", err, errMsg)
	}
	if info == nil || len(info.Entries) == 0 {
		t.Fatalf("Expected entries from Linhchanh.2k, got none (errMsg: %s)", errMsg)
	}
	t.Logf("Successfully scanned %d videos for %s", len(info.Entries), info.Title)
	for i, e := range info.Entries {
		t.Logf("  [%d] %s -> %s", i+1, e.Title, e.Url)
	}
}

func TestLiveFacebookReelsScrapeChutchit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	info, errMsg, err := ScrapeFacebookVideos("https://www.facebook.com/chutchit.v0/reels/", "", 1, 100)
	if err != nil {
		t.Fatalf("ScrapeFacebookVideos error: %v (errMsg: %s)", err, errMsg)
	}
	if info == nil || len(info.Entries) <= 2 {
		t.Fatalf("Expected all reels from chutchit.v0 (> 2), got %d (errMsg: %s)", len(info.Entries), errMsg)
	}
	t.Logf("Successfully scanned %d videos for %s (TotalCount: %d)", len(info.Entries), info.Title, info.TotalCount)
	for i, e := range info.Entries[:5] {
		t.Logf("  [%d] %s -> %s (thumb: %s)", i+1, e.Title, e.Url, e.Thumbnail)
	}
}

func TestResolveFbShareLink(t *testing.T) {
	shareURL := "https://www.facebook.com/share/18AgVPBsRT/"
	resolved := resolveFbShareURL(shareURL, "")
	if resolved == "" || IsFacebookShareURL(resolved) {
		t.Fatalf("resolveFbShareURL failed to resolve %s, got %q", shareURL, resolved)
	}
	if !strings.Contains(resolved, "djfakfang") {
		t.Errorf("expected resolved URL to contain 'djfakfang', got %q", resolved)
	}
	t.Logf("Successfully resolved %s -> %s", shareURL, resolved)

	profileHandle, normalizedURL := extractFbProfileFromShareURL(shareURL, "")
	if profileHandle != "djfakfang" {
		t.Errorf("extractFbProfileFromShareURL handle = %q, want %q", profileHandle, "djfakfang")
	}
	if !strings.Contains(normalizedURL, "djfakfang") {
		t.Errorf("extractFbProfileFromShareURL normalizedURL = %q, want djfakfang URL", normalizedURL)
	}
	t.Logf("Profile: %s, Normalized URL: %s", profileHandle, normalizedURL)
}

func TestScanFacebookShareLink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	shareURL := "https://www.facebook.com/share/18AgVPBsRT/"
	info, errMsg, err := ScanUrlWithPagination(shareURL, "", 1, 50)
	if err != nil {
		t.Fatalf("ScanUrlWithPagination error: %v (errMsg: %s)", err, errMsg)
	}
	if info == nil || len(info.Entries) <= 1 {
		t.Fatalf("Expected multiple videos from share link %s, got %d (errMsg: %s)", shareURL, len(info.Entries), errMsg)
	}
	t.Logf("Successfully fetched %d videos for %s from share link %s", len(info.Entries), info.Title, shareURL)
	for i, e := range info.Entries {
		t.Logf("  [%d] %s -> %s", i+1, e.Title, e.Url)
	}
}

func TestStreamFacebookShareLink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	shareURL := "https://www.facebook.com/share/18AgVPBsRT/"
	var items []ScanEntry
	var metaTitle string
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err := StreamScanUrl(ctx, shareURL, "", func(entry ScanEntry, count int) {
		items = append(items, entry)
		t.Logf("Streamed item #%d: %s -> %s", count, entry.Title, entry.Url)
	}, func(title, uploader, channel, thumbnail string, total int) {
		metaTitle = title
		t.Logf("Stream meta: %s (uploader: %s)", title, uploader)
	})

	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("StreamScanUrl error: %v", err)
	}
	if len(items) <= 1 {
		t.Fatalf("Expected > 1 streamed items from share link %s, got %d", shareURL, len(items))
	}
	t.Logf("Successfully streamed %d items for %s", len(items), metaTitle)
}

func TestStreamFacebookBaoNgocShareLink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	shareURL := "https://www.facebook.com/share/1MAjfNpz2Z/"
	var items []ScanEntry
	var metaTitle string
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	err := StreamScanUrl(ctx, shareURL, "", func(entry ScanEntry, count int) {
		items = append(items, entry)
		t.Logf("Streamed item #%d: %s -> %s", count, entry.Title, entry.Url)
	}, func(title, uploader, channel, thumbnail string, total int) {
		metaTitle = title
		t.Logf("Stream meta: %s (uploader: %s)", title, uploader)
	})

	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("StreamScanUrl error: %v", err)
	}
	if len(items) < 20 {
		t.Fatalf("Expected >= 20 streamed items from share link %s, got %d", shareURL, len(items))
	}
	t.Logf("Successfully streamed %d items for %s", len(items), metaTitle)
}


