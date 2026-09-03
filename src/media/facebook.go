package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)



// This file adds support for enumerating videos from a Facebook profile / page
// URL (e.g. https://www.facebook.com/Linhchanh.2k) — URLs yt-dlp cannot crawl
// directly because Facebook's video listing is JS-rendered and anti-bot protected.
//
// The scraper targets the mobile site (m.facebook.com) which serves a simpler,
// server-rendered HTML feed, discovers individual video links, and returns them
// as ScanEntry items. Each discovered link is an individual Facebook video URL
// that yt-dlp can download (using the configured cookies / impersonation).

const fbScrapeMaxPages = 80
const fbScrapeMaxEntries = 500
const fbRequestTimeout = 25 * time.Second
const fbLookahead = 16384

var (
	fbHostnameRe = regexp.MustCompile(`(?i)^https?://([a-z0-9-]+\.)?(facebook\.com|fb\.com)/`)

	// Matches an individual Facebook *video* URL path segment.
	fbVideoIdRe = regexp.MustCompile(`(?i)(/videos/|/posts/|/reel/|/reels/|watch\?v=|video\.php\?v=)(\d{6,})`)

	// Captures the kind prefix (videos/posts/reel/reels) along with the id so we
	// can categorize the entry (reel vs post vs standalone video).
	fbVideoKindRe = regexp.MustCompile(`(?i)/(videos|posts|reel|reels)/(\d{6,})`)
	fbWatchIdRe   = regexp.MustCompile(`(?i)(?:watch\?v=|video\.php\?v=)(\d{6,})`)

	// Matches <a href="..."> to enumerate links on the page.
	fbHrefRe = regexp.MustCompile(`href="([^"]*)"`)
	// Matches <img ... src="..."> for thumbnails.
	fbImgRe = regexp.MustCompile(`(?is)<\s*img[^>]+src="([^"]+)"`)

	// Detects a login / auth-wall form on the page.
	fbLoginFormRe = regexp.MustCompile(`(?i)(?:name|id)="(?:email|pass|password)"`)
)

// IsFacebookVideoURL reports whether raw is an individual Facebook video URL
// (handled directly by yt-dlp).
func IsFacebookVideoURL(raw string) bool {
	if !fbHostnameRe.MatchString(raw) {
		return false
	}
	return fbVideoIdRe.MatchString(raw)
}

// IsFacebookProfileURL reports whether raw is a Facebook profile / page /
// group URL whose video feed we should enumerate ourselves.
func IsFacebookProfileURL(raw string) bool {
	if !fbHostnameRe.MatchString(raw) {
		return false
	}
	if IsFacebookVideoURL(raw) {
		return false
	}
	// Exclude obvious non-profile paths.
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Path == "" || u.Path == "/" {
		return false
	}
	return true
}

// extractFbProfile returns the human-readable profile/page/group handle used for
// titles and channel metadata.
func extractFbProfile(inputURL string) string {
	u, err := url.Parse(strings.TrimSpace(inputURL))
	if err != nil {
		return "Facebook"
	}
	if id := u.Query().Get("id"); id != "" {
		return id
	}
	p := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
	if p == "" {
		return "Facebook"
	}
	if strings.HasPrefix(p, "pages/") {
		parts := strings.Split(p, "/")
		if len(parts) >= 2 && parts[1] != "" {
			return parts[1]
		}
	}
	if strings.HasPrefix(p, "people/") {
		parts := strings.Split(p, "/")
		if len(parts) >= 2 && parts[1] != "" {
			return parts[1]
		}
	}
	parts := strings.Split(p, "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "Facebook"
}

// normalizeFacebookVideosURL converts a profile/page/group URL into the mobile
// videos-tab feed URL used for scraping.
func normalizeFacebookVideosURL(inputURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(inputURL))
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, "facebook.com") && host != "fb.com" {
		return "", fmt.Errorf("not a facebook.com URL")
	}

	if id := u.Query().Get("id"); id != "" {
		return "https://www.facebook.com/profile.php?id=" + id + "&sk=reels_tab", nil
	}

	path := strings.Trim(strings.TrimSpace(u.Path), "/")
	if path == "" {
		return "", fmt.Errorf("missing profile path")
	}

	if strings.HasPrefix(path, "pages/") {
		return "https://m.facebook.com/" + path + "/videos/", nil
	}
	if strings.HasPrefix(path, "groups/") {
		return "https://m.facebook.com/" + path + "/videos/", nil
	}
	if strings.HasPrefix(path, "people/") {
		return "https://www.facebook.com/" + path + "/", nil
	}
	for _, s := range []string{"/reels_tab", "/reels", "/videos_by", "/videos_tagged", "/videos", "/posts", "/photos_by", "/photos", "/about"} {
		if strings.HasSuffix(strings.ToLower(path), s) {
			path = path[:len(path)-len(s)]
			break
		}
	}
	return "https://m.facebook.com/" + path + "/videos/", nil
}

// fbCookieHeader builds a Netscape-cookie "Cookie:" header value restricted to
// Facebook-owned domains, sourcing from the request cookies string or the
// project's cookies.txt.
func fbCookieHeader(cookies, targetURL string) string {
	raw := strings.TrimSpace(cookies)
	if raw == "" {
		cp := getWorkingCookiesPath(getCookiesPath())
		if cp != "" && isReadableFile(cp) {
			if data, err := os.ReadFile(cp); err == nil {
				raw = string(data)
			}
		}
	}
	if raw == "" {
		return ""
	}
	netscape := NormalizeCookiesToNetscape(raw, targetURL)
	return parseNetscapeCookiesForFb(netscape)
}

func parseNetscapeCookiesForFb(netscape string) string {
	var sb strings.Builder
	for _, line := range strings.Split(netscape, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := fields[0]
		if !isFbCookieDomain(domain) {
			continue
		}
		name := fields[5]
		value := fields[6]
		if name == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(name)
		sb.WriteString("=")
		sb.WriteString(value)
	}
	return sb.String()
}

func isFbCookieDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	switch {
	case strings.HasSuffix(d, "facebook.com"):
		return true
	case strings.HasSuffix(d, "fbcdn.net"):
		return true
	case strings.HasSuffix(d, "fbsbx.com"):
		return true
	case strings.HasSuffix(d, "fb.com"):
		return true
	}
	return false
}

// fbCanonicalVideoUrl converts a (possibly relative / protocol-relative) mobile
// Facebook video href into a canonical desktop URL that yt-dlp can download.
func fbCanonicalVideoUrl(href string) string {
	h := strings.TrimSpace(html.UnescapeString(href))
	h = strings.ReplaceAll(h, `\/`, `/`)
	if strings.HasPrefix(h, "//") {
		h = "https:" + h
	}
	if strings.HasPrefix(h, "http://") {
		h = "https" + h[4:]
	}
	if strings.HasPrefix(h, "https://") {
		// Normalize the mobile host to the desktop host yt-dlp understands.
		h = strings.Replace(h, "://m.facebook.com/", "://www.facebook.com/", 1)
		h = strings.Replace(h, "://m.fb.com/", "://www.facebook.com/", 1)
		h = strings.Replace(h, "://mobile.facebook.com/", "://www.facebook.com/", 1)
		return h
	}
	if strings.HasPrefix(h, "/") {
		return "https://www.facebook.com" + h
	}
	return ""
}

// fbCanonicalLinkUrl canonicalizes a (possibly relative) pagination link to an
// absolute m.facebook.com URL.
func fbCanonicalLinkUrl(href string) string {
	h := strings.TrimSpace(html.UnescapeString(href))
	h = strings.ReplaceAll(h, `\/`, `/`)
	if strings.HasPrefix(h, "//") {
		return "https:" + h
	}
	if strings.HasPrefix(h, "http://") {
		h = "https" + h[4:]
	}
	if strings.HasPrefix(h, "https://") {
		if strings.Contains(h, "m.facebook.com") || strings.Contains(h, "m.fb.com") {
			return strings.Replace(h, "://m.fb.com/", "://m.facebook.com/", 1)
		}
		return h
	}
	if strings.HasPrefix(h, "/") {
		return "https://m.facebook.com" + h
	}
	return ""
}

func fbFetchPageCurlCffi(ctx context.Context, pageURL, cookieHeader string) (string, error) {
	pyCode := `import sys
try:
    from curl_cffi import requests
except ImportError:
    sys.exit(2)

url = sys.argv[1]
cookies = sys.argv[2] if len(sys.argv) > 2 else ""

headers = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
    "Accept-Language": "en-US,en;q=0.9,vi;q=0.8",
    "Sec-Fetch-Dest": "document",
    "Sec-Fetch-Mode": "navigate",
    "Sec-Fetch-Site": "none",
    "Sec-Fetch-User": "?1",
    "Upgrade-Insecure-Requests": "1",
}
if cookies:
    headers["Cookie"] = cookies

session = requests.Session(impersonate="chrome124")
try:
    resp = session.get(url, headers=headers, timeout=25)
    if resp.status_code == 200:
        sys.stdout.write(resp.text)
        sys.exit(0)
    elif resp.status_code in (401, 403):
        sys.exit(3)
    else:
        sys.exit(1)
except Exception:
    sys.exit(1)
`
	cmd := exec.CommandContext(ctx, "python3", "-c", pyCode, pageURL, cookieHeader)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil && stdout.Len() > 0 {
		return stdout.String(), nil
	}
	return "", fmt.Errorf("curl_cffi fetch failed")
}

func fbFetchPage(ctx context.Context, pageURL, cookieHeader string) (string, error) {
	if body, err := fbFetchPageCurlCffi(ctx, pageURL, cookieHeader); err == nil && body != "" {
		return body, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	ua := "Mozilla/5.0 (Linux; Android 10; SM-G973F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.158 Mobile Safari/537.36"
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,vi;q=0.8")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Referer", "https://www.facebook.com/")
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("facebook HTTP %d (login wall)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("facebook HTTP %d", resp.StatusCode)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(b), nil
}

// fbHasLoginForm reports whether the page is an auth/login wall.
func fbHasLoginForm(html string) bool {
	if fbLoginFormRe.MatchString(html) {
		return true
	}
	lower := strings.ToLower(html)
	if strings.Contains(lower, "log in to continue") || strings.Contains(lower, "enter email or phone") {
		return true
	}
	return false
}

// categorizeFbHref inspects a raw href and returns a canonical Facebook video
// URL plus a category label ("reel", "post" or "video") when the href points
// at an individual video. Empty strings mean "not an individual video href".
// The returned canonical URL preserves the profile prefix when the href
// contains one (e.g. "/Linhchanh.2k/videos/123/"), so yt-dlp gets a fully
// qualified URL it can resolve.
func categorizeFbHref(rawHref string) (canonical, vid, category string) {
	h := strings.TrimSpace(html.UnescapeString(rawHref))
	h = strings.ReplaceAll(h, `\/`, `/`)
	if h == "" {
		return "", "", ""
	}

	if m := fbVideoKindRe.FindStringSubmatch(h); len(m) >= 3 {
		kind := strings.ToLower(m[1])
		vid = m[2]
		prefix := fbProfilePrefix(h)
		switch kind {
		case "reel", "reels":
			return "https://www.facebook.com" + prefix + "/reel/" + vid, vid, "reel"
		case "posts":
			return "https://www.facebook.com" + prefix + "/posts/" + vid, vid, "post"
		default:
			return "https://www.facebook.com" + prefix + "/videos/" + vid, vid, "video"
		}
	}

	if m := fbWatchIdRe.FindStringSubmatch(h); len(m) >= 2 {
		vid = m[1]
		return "https://www.facebook.com/watch?v=" + vid, vid, "video"
	}

	return "", "", ""
}

// fbProfilePrefix extracts the profile path segment (e.g. "/Linhchanh.2k",
// "/pages/MyPage/123", "/groups/mygroup") that precedes a /videos|/posts|/reel
// segment in a relative href. Returns "" for absolute or path-only hrefs.
// For page/group URLs the "prefix" is the full path leading up to the kind.
func fbProfilePrefix(href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "//") {
		return ""
	}
	if !strings.HasPrefix(href, "/") {
		return ""
	}
	// /Linhchanh.2k/videos/123  ->  /Linhchanh.2k
	// // Look for the kind segment and take everything before it.
	idx := strings.Index(href, "/videos/")
	if idx > 0 {
		return href[:idx]
	}
	idx = strings.Index(href, "/posts/")
	if idx > 0 {
		return href[:idx]
	}
	idx = strings.Index(href, "/reel/")
	if idx > 0 {
		return href[:idx]
	}
	idx = strings.Index(href, "/reels/")
	if idx > 0 {
		return href[:idx]
	}
	return ""
}

// fbHasVideoHref reports whether rawHref is any kind of Facebook video link.
func fbHasVideoHref(rawHref string) bool {
	return fbVideoIdRe.MatchString(strings.ToLower(rawHref))
}

// fbTitleForCategory returns a friendly default title for a freshly discovered
// video id, using its kind (reel / post / video).
func fbTitleForCategory(category, vid string) string {
	switch category {
	case "reel":
		return "Facebook reel #" + vid
	case "post":
		return "Facebook video (post) #" + vid
	default:
		return "Facebook video #" + vid
	}
}

// dedupeStrings removes empty / duplicate strings while preserving order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// isFbPaginationLink returns true when a normalized href on a Facebook profile
// page is a candidate next-page link (a videos-tab anchor with a cursor, an
// "Older posts" / "See more" anchor, or any anchor that re-enters the videos
// feed with query parameters).
func isFbPaginationLink(profileHandle, lowHref string) bool {
	if !strings.Contains(lowHref, "facebook.com") && !strings.HasPrefix(lowHref, "/") {
		return false
	}
	if strings.Contains(lowHref, "/login") {
		return false
	}
	if strings.Contains(lowHref, "watch?") {
		return false
	}

	lowerHandle := strings.ToLower(profileHandle)

	// Exclude direct individual-video anchors like "/videos/1234567890/" or
	// "/reel/1234567890/" — those point at a single video, not a paginated
	// feed.
	if fbVideoKindRe.MatchString(lowHref) {
		return false
	}

	// Profile-relative links like "/Linhchanh.2k/videos/?section=..."
	if lowerHandle != "" && strings.Contains(lowHref, "/"+lowerHandle+"/videos") {
		return true
	}
	if strings.Contains(lowHref, "/videos/") && (strings.Contains(lowHref, "cursor") ||
		strings.Contains(lowHref, "section") ||
		strings.Contains(lowHref, "after") ||
		strings.Contains(lowHref, "ref=") ||
		strings.Contains(lowHref, "page=") ||
		strings.Contains(lowHref, "timeline") ||
		strings.Contains(lowHref, "sk=") ||
		strings.Contains(lowHref, "multi_permalinks")) {
		return true
	}
	// Generic "See more" / older-posts anchors that point at the videos feed.
	markers := []string{"see_more", "see more", "show_more", "older", "morevideos", "more_videos"}
	for _, m := range markers {
		if strings.Contains(lowHref, m) {
			if strings.Contains(lowHref, "/videos") || strings.Contains(lowHref, "/reels") || strings.Contains(lowHref, "/posts") {
				return true
			}
		}
	}
	return false
}

// parseFbVideoLinks scans a mobile Facebook videos-tab page for individual video
// links and a list of "next page" candidate links.
func parseFbVideoLinks(profileHandle string, html string) (entries []ScanEntry, nextPages []string) {
	unescaped := strings.ReplaceAll(html, `\/`, `/`)

	locs := fbHrefRe.FindAllStringSubmatchIndex(html, -1)
	for i, loc := range locs {
		rawHref := html[loc[2]:loc[3]]

		canonical, vid, category := categorizeFbHref(rawHref)
		if canonical != "" {
			if duplicateFbScanEntry(entries, canonical) || duplicateFbVid(entries, vid) {
				continue
			}
			thumb := fbThumbnailAt(html, loc[1], i, locs)
			title := fbTitleForCategory(category, vid)
			if thumb == "" {
				metaTitle, metaThumb := fbExtractReelMeta(unescaped, vid)
				if metaThumb != "" {
					thumb = metaThumb
				}
				if metaTitle != "" && !strings.HasPrefix(metaTitle, "Facebook video #") && !strings.HasPrefix(metaTitle, "Facebook reel #") {
					title = metaTitle
				}
			}
			entries = append(entries, ScanEntry{
				Id:        canonical,
				Title:     title,
				Url:       canonical,
				Thumbnail: thumb,
				Category:  category,
			})
			continue
		}

		low := strings.ToLower(rawHref)
		if isFbPaginationLink(profileHandle, low) {
			if np := fbCanonicalLinkUrl(rawHref); np != "" {
				nextPages = append(nextPages, np)
			}
		}
	}

	// Also scan for modern React / JSON embedded video IDs (e.g. reels / video_id)
	jsonVidRe := regexp.MustCompile(`(?i)(/reel/|/reels/|/videos/|/posts/|"video_id":\s*"|"video":\s*\{\s*"id":\s*")(\d{6,})`)
	for _, sub := range jsonVidRe.FindAllStringSubmatch(unescaped, -1) {
		kind := ""
		switch {
		case strings.HasPrefix(strings.ToLower(sub[1]), "/reel/"), strings.HasPrefix(strings.ToLower(sub[1]), "/reels/"):
			kind = "reel"
		case strings.HasPrefix(strings.ToLower(sub[1]), "/posts/"):
			kind = "post"
		case strings.HasPrefix(strings.ToLower(sub[1]), "/videos/"):
			kind = "video"
		}
		vid := sub[2]
		var canonical string
		switch kind {
		case "reel":
			canonical = "https://www.facebook.com/reel/" + vid
		case "post":
			canonical = "https://www.facebook.com/posts/" + vid
		default:
			canonical = "https://www.facebook.com/videos/" + vid
		}
		title, thumb := fbExtractReelMeta(unescaped, vid)
		if title == "" || strings.HasPrefix(title, "Facebook reel #") || strings.HasPrefix(title, "Facebook video ") {
			// Fallback to category-prefixed default.
			title = fbTitleForCategory(kind, vid)
		}

		if existingIdx := findEntryByVid(entries, vid); existingIdx >= 0 {
			if entries[existingIdx].Thumbnail == "" && thumb != "" {
				entries[existingIdx].Thumbnail = thumb
			}
			if (strings.HasPrefix(entries[existingIdx].Title, "Facebook video #") || strings.HasPrefix(entries[existingIdx].Title, "Facebook reel #")) && title != "" && !strings.HasPrefix(title, "Facebook video #") && !strings.HasPrefix(title, "Facebook reel #") {
				entries[existingIdx].Title = title
			}
			if kind == "reel" && entries[existingIdx].Category != "reel" {
				entries[existingIdx].Category = "reel"
				entries[existingIdx].Url = canonical
				entries[existingIdx].Id = canonical
			}
			continue
		}

		entries = append(entries, ScanEntry{
			Id:        canonical,
			Title:     title,
			Url:       canonical,
			Thumbnail: thumb,
			Category:  kind,
		})
	}
	// Look for numeric Page / User IDs to discover additional feeds
	idRe := regexp.MustCompile(`"(?:pageID|page_id|delegate_page":\{"id)":"(\d{6,})"`)
	for _, m := range idRe.FindAllStringSubmatch(unescaped, -1) {
		pid := m[1]
		nextPages = append(nextPages,
			"https://www.facebook.com/"+pid+"/videos/",
			"https://www.facebook.com/"+pid+"/videos_by/",
			"https://www.facebook.com/"+pid+"/reels_tab/",
		)
	}

	return entries, dedupeStrings(nextPages)
}

func findEntryByVid(entries []ScanEntry, vid string) int {
	for i, e := range entries {
		if strings.Contains(e.Url, vid) || strings.Contains(e.Id, vid) {
			return i
		}
	}
	return -1
}

var (
	fbThumbRe = regexp.MustCompile(`(?i)(?:first_frame_thumbnail|thumbnailImage|preferred_thumbnail|image|"uri")[:\s"\{]+(https://scontent[^"\\]+)`)
	fbTextRe  = regexp.MustCompile(`"text":"([^"]+)"`)
)

func fbExtractReelMeta(fullText, vid string) (title, thumbnail string) {
	pos := 0
	for {
		idx := strings.Index(fullText[pos:], vid)
		if idx == -1 {
			break
		}
		realIdx := pos + idx
		start := realIdx - 10000
		if start < 0 {
			start = 0
		}
		end := realIdx + 10000
		if end > len(fullText) {
			end = len(fullText)
		}
		chunk := fullText[start:end]

		if thumbnail == "" {
			matches := fbThumbRe.FindAllStringSubmatch(chunk, -1)
			for _, m := range matches {
				if len(m) > 1 {
					u := m[1]
					if strings.Contains(u, "t15") || strings.Contains(u, "dst-jpg") || strings.Contains(u, "p960x960") || strings.Contains(u, "s960x960") {
						thumbnail = u
						break
					} else if thumbnail == "" && !strings.Contains(u, "t39.30808-1") {
						thumbnail = u
					}
				}
			}
		}
		if title == "" {
			if m := fbTextRe.FindStringSubmatch(chunk); len(m) > 1 {
				cand := strings.TrimSpace(unescapeUnicodeString(m[1]))
				if len(cand) > 5 && !strings.HasPrefix(cand, "http") && cand != "Profile" && cand != "Reels" && cand != "Videos" && cand != "Follow" && cand != "Message" && cand != "No reels to show" {
					title = cand
				}
			}
		}
		if thumbnail != "" && title != "" {
			break
		}
		pos = realIdx + len(vid)
	}
	if title == "" {
		title = "Facebook video #" + vid
	}
	return title, thumbnail
}

func unescapeUnicodeString(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+5 < len(s) && s[i+1] == 'u' {
			var r rune
			if _, err := fmt.Sscanf(s[i:i+6], "\\u%04x", &r); err == nil {
				sb.WriteRune(r)
				i += 6
				continue
			}
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}

func duplicateFbVid(entries []ScanEntry, vid string) bool {
	for _, e := range entries {
		if strings.Contains(e.Url, vid) || strings.Contains(e.Id, vid) {
			return true
		}
	}
	return false
}

func duplicateFbScanEntry(entries []ScanEntry, canonical string) bool {
	for _, e := range entries {
		if e.Url == canonical {
			return true
		}
	}
	return false
}

// fbThumbnailAt looks for the first <img src="..."> after the given href's
// closing quote, bounded by a lookahead and the next href to stay within the
// same tile.
func fbThumbnailAt(pageHTML string, start int, idx int, locs [][]int) string {
	end := start + fbLookahead
	if end > len(pageHTML) {
		end = len(pageHTML)
	}
	if idx+1 < len(locs) {
		nextStart := locs[idx+1][0]
		if nextStart > start && nextStart < end {
			end = nextStart
		}
	}
	sub := pageHTML[start:end]
	if m := fbImgRe.FindStringSubmatch(sub); len(m) > 1 {
		thumb := strings.TrimSpace(html.UnescapeString(m[1]))
		if strings.HasPrefix(thumb, "https://") || strings.HasPrefix(thumb, "http://") {
			return thumb
		}
		if strings.HasPrefix(thumb, "/") {
			return "https://m.facebook.com" + thumb
		}
	}
	return ""
}

// scrapeFbVideoPage fetches one page of the mobile videos feed.
func scrapeFbVideoPage(ctx context.Context, pageURL, cookieHeader, profileHandle string) (entries []ScanEntry, nextPages []string, loginWall bool, err error) {
	body, err := fbFetchPage(ctx, pageURL, cookieHeader)
	if err != nil {
		return nil, nil, false, err
	}
	if fbHasLoginForm(body) {
		return nil, nil, true, nil
	}
	entries, nextPages = parseFbVideoLinks(profileHandle, body)
	return entries, nextPages, false, nil
}

// ScrapeFacebookVideos enumerates video links from a Facebook profile / page /
// group URL. Used by the non-streaming /api/scan endpoint.
func ScrapeFacebookVideos(inputURL, cookies string, start, limit int) (*ScanInfo, string, error) {
	if start <= 0 {
		start = 1
	}
	if limit <= 0 {
		limit = 24
	}

	mobileURL, err := normalizeFacebookVideosURL(inputURL)
	if err != nil {
		return nil, "", fmt.Errorf("could not parse Facebook profile URL: %w", err)
	}

	cookieHeader := fbCookieHeader(cookies, inputURL)
	profile := extractFbProfile(inputURL)

	maxFetch := fbScrapeMaxEntries
	if start+limit-1 > maxFetch {
		maxFetch = start + limit - 1
	}

	// Collect from both GraphQL and HTML scrapers, then merge with deduplication
	// so we never miss videos (GraphQL may find only the current page).
	graphQLSeen := make(map[string]bool)
	graphQLEntries := make([]ScanEntry, 0, fbScrapeMaxEntries)
	_ = fbStreamGraphQLAll(context.Background(), inputURL, cookieHeader, maxFetch, func(e ScanEntry) {
		if graphQLSeen[e.Url] || graphQLSeen[e.Id] {
			return
		}
		graphQLSeen[e.Url] = true
		graphQLSeen[e.Id] = true
		graphQLEntries = append(graphQLEntries, e)
	})

	// Always run HTML scraper as a secondary source to catch videos the GraphQL
	// API may not return (e.g., when Facebook's GraphQL only returns the current
	// visible page rather than the full paginated collection).
	htmlEntries, _, _, sErr := scrapeFbAll(context.Background(), mobileURL, cookieHeader, profile, maxFetch)
	if sErr != nil && len(graphQLEntries) == 0 && len(htmlEntries) == 0 {
		return nil, sErr.Error(), sErr
	}

	// Merge: keep GraphQL entries first, then add HTML entries (skip duplicates).
	seen := make(map[string]bool)
	for _, e := range graphQLEntries {
		seen[e.Url] = true
		seen[e.Id] = true
	}
	for _, e := range htmlEntries {
		if !seen[e.Url] && !seen[e.Id] {
			seen[e.Url] = true
			seen[e.Id] = true
			graphQLEntries = append(graphQLEntries, e)
		}
	}

	var entries []ScanEntry
	entries = graphQLEntries

	// Apply offset window.
	total := len(entries)
	if start > total {
		entries = nil
	} else {
		if end := start + limit; end <= total {
			entries = entries[start-1 : end]
		} else {
			entries = entries[start-1:]
		}
	}

	if len(entries) == 0 {
		return nil, "", nil
	}

	hasMore := total > start+limit-1
	return &ScanInfo{
		Title:      profile + " — Facebook Videos",
		Count:      len(entries),
		TotalCount: total,
		IsPlaylist: len(entries) > 1,
		Entries:    entries,
		Channel:    profile,
		Start:      start,
		Limit:      limit,
		HasMore:    hasMore,
		NextStart:  start + len(entries),
	}, "", nil
}

func extractFbVid(raw string) string {
	if m := fbVideoIdRe.FindStringSubmatch(raw); len(m) >= 3 {
		return m[2]
	}
	return raw
}

// fbStreamGraphQLAll uses browser-impersonated Python curl_cffi to paginate through
// Facebook GraphQL collections (Reels, Videos, Posts) and streams each entry as JSON.
func fbStreamGraphQLAll(ctx context.Context, inputURL, cookieHeader string, maxItems int, onEntry func(ScanEntry)) error {
	pyCode := `import sys, json, re, urllib.parse
from curl_cffi import requests

profile_url = sys.argv[1].rstrip("/")
cookies = sys.argv[2] if len(sys.argv) > 2 else ""
max_items = int(sys.argv[3]) if len(sys.argv) > 3 and sys.argv[3].isdigit() else 200

u = urllib.parse.urlparse(profile_url)
qs = urllib.parse.parse_qs(u.query)
pid = qs.get("id", [""])[0]

if pid:
    reels_url = f"https://www.facebook.com/profile.php?id={pid}&sk=reels_tab"
    videos_url = f"https://www.facebook.com/profile.php?id={pid}&sk=videos"
    main_url = f"https://www.facebook.com/profile.php?id={pid}"
else:
    path = u.path.strip("/")
    suffixes = ["/reels_tab", "/reels", "/videos_by", "/videos_tagged", "/videos", "/posts", "/photos_by", "/photos", "/about"]
    for s in suffixes:
        if path.lower().endswith(s):
            path = path[:-len(s)]
            break
    base = f"https://www.facebook.com/{path}"
    reels_url = f"{base}/reels_tab"
    videos_url = f"{base}/videos"
    main_url = base

headers = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
    "Accept-Language": "en-US,en;q=0.9,vi;q=0.8",
}
if cookies:
    headers["Cookie"] = cookies

session = requests.Session(impersonate="chrome124")

def emit(item):
    try:
        sys.stdout.write(json.dumps(item, ensure_ascii=False) + "\n")
        sys.stdout.flush()
    except Exception:
        try:
            sys.stdout.write(json.dumps(item, ensure_ascii=True) + "\n")
            sys.stdout.flush()
        except Exception:
            pass

vids_seen = set()

def safe_decode_json_str(s):
    try:
        return json.loads('"' + s.replace('"', '\\"') + '"')
    except Exception:
        try:
            return s.encode('utf-8', 'ignore').decode('unicode_escape', 'ignore')
        except Exception:
            return s

def extract_meta(chunk, vid):
    thumb = ""
    try:
        tm = re.findall(r'(?i)(?:first_frame_thumbnail|thumbnailImage|preferred_thumbnail|image|"uri")[:\s"\{]+(https://[^\"]+?(?:fbcdn\.net|fbsbx\.com)[^\"]+)', chunk)
        v_thumbs = [u for u in tm if "t15" in u or "dst-jpg" in u or "p960x960" in u or "s960x960" in u]
        if not v_thumbs:
            v_thumbs = [u for u in tm if "t39" not in u and "blank.gif" not in u]
        if v_thumbs:
            thumb = v_thumbs[0].replace(r"\/", "/")
    except Exception:
        pass

    title = ""
    try:
        txtm = re.search(r'"text":"((?:[^"\\\\]|\\\\.)*)"', chunk)
        if txtm:
            cand = safe_decode_json_str(txtm.group(1)).strip()
            if len(cand) > 3 and cand not in ["Profile", "Reels", "Videos", "Follow", "Message", "No reels to show"]:
                title = cand
    except Exception:
        pass
    return title, thumb

# 1. reels_tab
try:
    r = session.get(reels_url, headers=headers, timeout=25)
    text = r.text
    unescaped = text.replace(r"\/", "/")
    initial_vids = re.findall(r'(?:/reel/|/videos/|\"video_id\":\s*\"|\"video\":\s*\{\s*\"id\":\s*\")(\d{6,})', unescaped)
    for v in initial_vids:
        if v in vids_seen: continue
        vids_seen.add(v)
        try:
            idx = unescaped.find(v)
            chunk = unescaped[max(0, idx - 10000):min(len(unescaped), idx + 10000)]
            t, th = extract_meta(chunk, v)
            emit({"id": f"https://www.facebook.com/reel/{v}", "title": t or f"Facebook reel #{v}", "url": f"https://www.facebook.com/reel/{v}", "thumbnail": th, "category": "Reels"})
        except Exception:
            emit({"id": f"https://www.facebook.com/reel/{v}", "title": f"Facebook reel #{v}", "url": f"https://www.facebook.com/reel/{v}", "thumbnail": "", "category": "Reels"})
        if len(vids_seen) >= max_items: break

    lsd_m = re.search(r'\"LSD\",\[\],\{\"token\":\"([^\"]+)\"', text)
    lsd = lsd_m.group(1) if lsd_m else ""
    cursor_m = re.search(r'\"end_cursor\":\"([^\"]+)\"[^}}]*\"has_next_page\":true\}\},\"id\":\"([^\"]+)\"', text)
    if lsd and cursor_m:
        cursor = cursor_m.group(1)
        coll_id = cursor_m.group(2)
        pv_names = [
          "__relay_internal__pv__FBReels_deprecate_short_form_video_context_gkrelayprovider",
          "__relay_internal__pv__FBReelsMediaFooter_comet_enable_reels_ads_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaContentContainer_comet_reels_video_footer_defer_loading_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaContentContainer_comet_video_document_picture_in_picture_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaContentContainer_enable_chapters_pill_gkrelayprovider",
          "__relay_internal__pv__ShouldEnableBakedInTextUnifiedVideorelayprovider",
          "__relay_internal__pv__FBUnifiedVideoCometVideoMedia_comet_photosensitive_content_warning_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaHeaderControls_enable_chapters_pill_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaFooter_comet_enable_reels_ads_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaFooter_organic_ad_cta_on_comet_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaFooter_enable_meta_ai_pill_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaFooter_enable_ai_embodiment_chat_pill_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMediaFooter_enable_video_augment_pills_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoPlayerScrubber_fb_comet_vpv_heatmap_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoDescriptionWithEntities_comet_translations_revamp_sync_caption_with_audio_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoFeedbackBar_comet_reels_save_button_gkrelayprovider",
          "__relay_internal__pv__usePushPipEngagementCounts_comet_video_document_picture_in_picture_gkrelayprovider",
          "__relay_internal__pv__FBReels_enable_view_dubbed_audio_type_gkrelayprovider",
          "__relay_internal__pv__FBUnifiedVideoMenu_fb_reels_ranking_debug_tool_gkrelayprovider",
          "__relay_internal__pv__CometAudioLanguageUtils_comet_translations_revamp_preferred_languages_gkrelayprovider"
        ]
        while cursor and len(vids_seen) < max_items:
            vars_dict = {"count": 10, "cursor": cursor, "id": coll_id, "renderLocation": None, "scale": None, "useDefaultActor": False}
            for pv in pv_names: vars_dict[pv] = False
            payload = {
                "av": "0", "__user": "0", "__a": "1", "lsd": lsd,
                "fb_api_caller_class": "RelayModern",
                "fb_api_req_friendly_name": "ProfileCometAppCollectionReelsRendererPaginationQuery",
                "variables": json.dumps(vars_dict),
                "doc_id": "28401661769429506"
            }
            resp = session.post("https://www.facebook.com/api/graphql/", data=payload, headers=headers, timeout=20)
            resp_text = resp.text.replace(r"\/", "/")
            page_vids = re.findall(r'\"video\":\{\"id\":\"(\d+)\"', resp_text) + re.findall(r'\"video_id\":\"(\d+)\"', resp_text) + re.findall(r'/reel/(\d+)', resp_text)
            new_in_page = 0
            for v in page_vids:
                if v in vids_seen: continue
                vids_seen.add(v)
                new_in_page += 1
                try:
                    idx = resp_text.find(v)
                    chunk = resp_text[max(0, idx - 10000):min(len(resp_text), idx + 10000)]
                    t, th = extract_meta(chunk, v)
                    emit({"id": f"https://www.facebook.com/reel/{v}", "title": t or f"Facebook reel #{v}", "url": f"https://www.facebook.com/reel/{v}", "thumbnail": th, "category": "Reels"})
                except Exception:
                    emit({"id": f"https://www.facebook.com/reel/{v}", "title": f"Facebook reel #{v}", "url": f"https://www.facebook.com/reel/{v}", "thumbnail": "", "category": "Reels"})
                if len(vids_seen) >= max_items: break
            pi = re.search(r'\"page_info\":\{[^}]*\"end_cursor\":\"([^\"]+)\"[^}]*\"has_next_page\":(true|false)', resp.text)
            if pi and pi.group(2) == "true" and new_in_page > 0 and len(vids_seen) < max_items:
                cursor = pi.group(1)
            else:
                break
except Exception:
    pass

# 2. /videos tab
if len(vids_seen) < max_items:
    try:
        vr = session.get(videos_url, headers=headers, timeout=20)
        v_text = vr.text.replace(r"\/", "/")
        regular_vids = re.findall(r'(?:/videos/|\"video_id\":\s*\"|\"video\":\s*\{\s*\"id\":\s*\")(\d{6,})', v_text)
        for v in regular_vids:
            if v in vids_seen: continue
            vids_seen.add(v)
            try:
                idx = v_text.find(v)
                chunk = v_text[max(0, idx - 10000):min(len(v_text), idx + 10000)]
                t, th = extract_meta(chunk, v)
                emit({"id": f"https://www.facebook.com/watch/?v={v}", "title": t or f"Facebook video #{v}", "url": f"https://www.facebook.com/watch/?v={v}", "thumbnail": th, "category": "Videos"})
            except Exception:
                emit({"id": f"https://www.facebook.com/watch/?v={v}", "title": f"Facebook video #{v}", "url": f"https://www.facebook.com/watch/?v={v}", "thumbnail": "", "category": "Videos"})
            if len(vids_seen) >= max_items: break
    except Exception:
        pass

# 3. Main profile timeline posts
if len(vids_seen) < max_items:
    try:
        pr = session.get(main_url, headers=headers, timeout=20)
        p_text = pr.text.replace(r"\/", "/")
        post_vids = re.findall(r'(?:/posts/|\"video_id\":\s*\"|\"video\":\s*\{\s*\"id\":\s*\")(\d{6,})', p_text)
        for v in post_vids:
            if v in vids_seen: continue
            vids_seen.add(v)
            try:
                idx = p_text.find(v)
                chunk = p_text[max(0, idx - 10000):min(len(p_text), idx + 10000)]
                t, th = extract_meta(chunk, v)
                emit({"id": f"https://www.facebook.com/watch/?v={v}", "title": t or f"Facebook post #{v}", "url": f"https://www.facebook.com/watch/?v={v}", "thumbnail": th, "category": "Posts"})
            except Exception:
                emit({"id": f"https://www.facebook.com/watch/?v={v}", "title": f"Facebook post #{v}", "url": f"https://www.facebook.com/watch/?v={v}", "thumbnail": "", "category": "Posts"})
            if len(vids_seen) >= max_items: break
    except Exception:
        pass
`
	cmd := exec.CommandContext(ctx, "python3", "-c", pyCode, inputURL, cookieHeader, strconv.Itoa(maxItems))
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdoutPipe)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var item ScanEntry
		if err := json.Unmarshal([]byte(line), &item); err == nil && item.Url != "" {
			onEntry(item)
		}
	}

	_ = cmd.Wait()
	return nil
}

// StreamFacebookVideos enumerates video links live, emitting each entry via the
// provided callbacks. Used by the /api/scan/stream SSE endpoint.
func StreamFacebookVideos(ctx context.Context, inputURL, cookies string, onEntry func(ScanEntry, int), onMeta func(title, uploader, channel, thumbnail string, total int)) error {
	mobileURL, err := normalizeFacebookVideosURL(inputURL)
	if err != nil {
		return fmt.Errorf("could not parse Facebook profile URL: %w", err)
	}

	cookieHeader := fbCookieHeader(cookies, inputURL)
	profile := extractFbProfile(inputURL)
	title := profile + " — Facebook Videos"

	onMeta(title, profile, profile, "", 0)

	limit := fbScrapeMaxEntries
	visited := make(map[string]bool)
	seen := make(map[string]bool)
	count := 0

	// 1. Run GraphQL & multi-category paginated scanner (reels_tab, videos, timeline posts)
	// 2. Then run HTML mobile queue to catch any additional videos (same dedup logic).
	_ = fbStreamGraphQLAll(ctx, inputURL, cookieHeader, limit, func(e ScanEntry) {
		if seen[e.Url] || seen[e.Id] {
			return
		}
		seen[e.Url] = true
		seen[e.Id] = true
		count++
		onEntry(e, count)
	})

	// HTML mobile queue: finds additional videos that GraphQL may not return
	queue := fbProfileTabs(profile, mobileURL)
	emittedAtLeastOne := false
	for len(queue) > 0 && count < limit && ctx.Err() == nil {
		currentPage := queue[0]
		queue = queue[1:]
		if currentPage == "" || visited[currentPage] {
			continue
		}
		visited[currentPage] = true

		pageCtx, cancel := context.WithTimeout(ctx, fbRequestTimeout)
		found, nextPages, loginWall, ferr := scrapeFbVideoPage(pageCtx, currentPage, cookieHeader, profile)
		cancel()
		if ferr != nil {
			if emittedAtLeastOne || len(queue) > 0 {
				continue
			}
			return ferr
		}
		if loginWall {
			if emittedAtLeastOne || len(queue) > 0 {
				continue
			}
			return fmt.Errorf("Facebook requires authentication: the profile returned a login wall. Provide Facebook cookies (cookies.txt) to list its videos")
		}

		emitted := false
		for _, e := range found {
			vid := extractFbVid(e.Url)
			if seen[vid] || seen[e.Url] {
				continue
			}
			seen[vid] = true
			seen[e.Url] = true
			count++
			emitted = true
			emittedAtLeastOne = true
			onEntry(e, count)
			if count >= limit {
				break
			}
		}

		if !emitted && len(found) == 0 {
			continue
		}
		for _, np := range nextPages {
			if !visited[np] {
				queue = append(queue, np)
			}
		}
		log.Debug().Msgf("facebook scraper queue=%d, emitted=%d, total=%d", len(queue), len(found), count)
	}

	return nil
}

// fbProfileTabs returns the initial list of feed URLs the streaming scraper
// should walk for a profile. It includes mobile and desktop tabs (reels, videos, posts)
// so we pick up reels + posts alongside regular videos.
func fbProfileTabs(profile, mobileURL string) []string {
	tabs := []string{mobileURL}

	if strings.Contains(mobileURL, "profile.php?id=") {
		u, err := url.Parse(mobileURL)
		id := ""
		if err == nil {
			id = u.Query().Get("id")
		}
		if id == "" {
			id = profile
		}
		tabs = append(tabs,
			"https://www.facebook.com/profile.php?id="+id+"&sk=reels_tab",
			"https://www.facebook.com/profile.php?id="+id+"&sk=videos",
			"https://www.facebook.com/profile.php?id="+id+"&sk=videos_by",
			"https://www.facebook.com/profile.php?id="+id+"&sk=photos_by",
			"https://www.facebook.com/profile.php?id="+id,
			"https://m.facebook.com/profile.php?id="+id,
			"https://www.facebook.com/"+id+"/reels_tab/",
			"https://www.facebook.com/"+id+"/videos/",
			"https://www.facebook.com/"+id+"/videos_by/",
			"https://m.facebook.com/"+id+"/videos/",
		)
	} else {
		profileEnc := url.PathEscape(profile)
		if profileEnc == "" {
			profileEnc = profile
		}

		desktopVideos := strings.Replace(mobileURL, "m.facebook.com", "www.facebook.com", 1)
		tabs = append(tabs,
			"https://www.facebook.com/"+profile+"/reels_tab/",
			"https://www.facebook.com/"+profile+"/reels/",
			"https://www.facebook.com/"+profile+"/videos/",
			"https://www.facebook.com/"+profile+"/videos_by/",
			"https://www.facebook.com/"+profile+"/videos_tagged/",
			"https://www.facebook.com/"+profile+"/posts/",
			"https://m.facebook.com/"+profile+"/videos_by/",
			"https://m.facebook.com/"+profile+"/reels/",
			"https://m.facebook.com/"+profile+"?v=timeline",
			"https://www.facebook.com/"+profileEnc+"/videos/",
			desktopVideos,
		)
	}

	out := make([]string, 0, len(tabs))
	seen := make(map[string]bool)
	for _, t := range tabs {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// scrapeFbAll accumulates entries up to maxCount across paginated pages.
func scrapeFbAll(ctx context.Context, firstURL, cookieHeader, profile string, maxCount int) ([]ScanEntry, string, string, error) {
	if maxCount <= 0 {
		maxCount = fbScrapeMaxEntries
	}
	if maxCount > fbScrapeMaxEntries {
		maxCount = fbScrapeMaxEntries
	}

	entries := make([]ScanEntry, 0, maxCount)
	seen := make(map[string]bool)
	visited := make(map[string]bool)
	queue := fbProfileTabs(profile, firstURL)
	pages := 0

	for len(entries) < maxCount && ctx.Err() == nil {
		if pages >= fbScrapeMaxPages {
			break
		}
		if len(queue) == 0 {
			break
		}
		current := queue[0]
		queue = queue[1:]
		if current == "" || visited[current] {
			continue
		}
		visited[current] = true
		pages++

		pageCtx, cancel := context.WithTimeout(ctx, fbRequestTimeout)
		found, nextPages, loginWall, err := scrapeFbVideoPage(pageCtx, current, cookieHeader, profile)
		cancel()
		if err != nil {
			if len(entries) > 0 || len(queue) > 0 {
				continue
			}
			if strings.Contains(err.Error(), "login wall") {
				return nil, "", "Facebook returned a login wall (HTTP 401/403). A logged-in cookies.txt is required to list videos from this profile.", err
			}
			return nil, "", fmt.Sprintf("Facebook fetch failed: %s", err), err
		}
		if loginWall {
			if len(entries) > 0 || len(queue) > 0 {
				continue
			}
			return nil, "", "Facebook returned a login wall. A logged-in cookies.txt is required to list videos from this profile.", fmt.Errorf("login wall")
		}

		for _, e := range found {
			if len(entries) >= maxCount {
				break
			}
			vid := extractFbVid(e.Url)
			if seen[vid] || seen[e.Url] {
				continue
			}
			seen[vid] = true
			seen[e.Url] = true
			entries = append(entries, e)
		}

		if len(found) == 0 {
			continue
		}
		for _, np := range nextPages {
			if !visited[np] {
				queue = append(queue, np)
			}
		}
	}

	if len(entries) == 0 {
		return nil, "", "No videos found on this Facebook profile. The page may be empty, private, or blocking automated access.", nil
	}
	return entries, "", "", nil
}
