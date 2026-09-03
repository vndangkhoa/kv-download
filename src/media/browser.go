package media

import (
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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"kv-download/src/anpan"

	"github.com/rs/zerolog/log"
)

type SniffedMediaItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`      // "video", "audio", "image", "stream"
	Format    string `json:"format"`    // "M3U8 HLS", "MP4 1080p", "MP3", "JPG Image", etc.
	Title     string `json:"title"`     // Title or filename
	URL       string `json:"url"`       // Stream or direct media URL
	Thumbnail string `json:"thumbnail"` // Thumbnail if available
	Size      string `json:"size"`      // Formatted size if known
	Source    string `json:"source"`    // "page_dom", "network_sniffer", "ytdlp_engine", "anpan_extractor"
}

type BrowserSniffResult struct {
	URL        string             `json:"url"`
	PageTitle  string             `json:"pageTitle"`
	Items      []SniffedMediaItem `json:"items"`
	TotalCount int                `json:"totalCount"`
}

func sanitizeMediaUrl(raw string) string {
	u := html.UnescapeString(strings.TrimSpace(raw))
	u = strings.ReplaceAll(u, `\/`, `/`)
	u = strings.Trim(u, `"'`)
	if idx := strings.LastIndex(u, "http://"); idx > 0 {
		u = u[idx:]
	} else if idx := strings.LastIndex(u, "https://"); idx > 0 {
		u = u[idx:]
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return ""
	}
	return u
}

var (
	m3u8Regex  = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:m3u8|mpd)(?:\?[^\s"'<>\\]*)?`)
	videoRegex = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:mp4|webm|mkv|mov|avi|flv)(?:\?[^\s"'<>\\]*)?`)
	audioRegex = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:mp3|m4a|aac|wav|flac|ogg|oga)(?:\?[^\s"'<>\\]*)?`)
	imageRegex = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:jpg|jpeg|png|webp|gif|avif)(?:\?[^\s"'<>\\]*)?`)
	titleRegex = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
)

// BrowserSniffHandler deeply analyzes any URL and extracts all playable videos, m3u8 streams, audio, and photos.
func BrowserSniffHandler(w http.ResponseWriter, r *http.Request) {
	targetUrl := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetUrl == "" {
		http.Error(w, `{"error":"Missing url parameter"}`, http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(targetUrl, "http://") && !strings.HasPrefix(targetUrl, "https://") {
		targetUrl = "https://" + targetUrl
	}

	w.Header().Set("Content-Type", "application/json")

	result := BrowserSniffResult{
		URL:   targetUrl,
		Items: make([]SniffedMediaItem, 0),
	}

	seenURLs := make(map[string]bool)
	var mu sync.Mutex

	addItem := func(item SniffedMediaItem) {
		mu.Lock()
		defer mu.Unlock()
		cleanURL := sanitizeMediaUrl(item.URL)
		if cleanURL == "" || seenURLs[cleanURL] {
			return
		}
		seenURLs[cleanURL] = true
		item.URL = cleanURL
		if item.ID == "" {
			item.ID = fmt.Sprintf("sniff_%d", len(result.Items)+1)
		}
		if item.Title == "" {
			item.Title = filepath.Base(strings.Split(cleanURL, "?")[0])
		}
		if item.Thumbnail != "" {
			item.Thumbnail = sanitizeMediaUrl(item.Thumbnail)
		}
		result.Items = append(result.Items, item)
	}

	// 1. Check with Anpan Multi-source Engines (Archive, Booru, Cloud, Pixiv, Imgur)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if target, err := anpan.InspectTarget(ctx, targetUrl); err == nil && target != nil {
		if target.ArchivePost != nil {
			if target.ArchivePost.Title != "" {
				result.PageTitle = target.ArchivePost.Title
			}
			for _, f := range target.ArchivePost.Files {
				mType := "image"
				fLower := strings.ToLower(f.Name)
				if strings.HasSuffix(fLower, ".mp4") || strings.HasSuffix(fLower, ".webm") {
					mType = "video"
				} else if strings.HasSuffix(fLower, ".mp3") || strings.HasSuffix(fLower, ".m4a") || strings.HasSuffix(fLower, ".flac") {
					mType = "audio"
				}
				addItem(SniffedMediaItem{
					Type:      mType,
					Format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(f.Name), ".")),
					Title:     f.Name,
					URL:       f.URL,
					Thumbnail: f.URL,
					Source:    "anpan_extractor",
				})
			}
		} else if target.Type == anpan.TargetDirect && target.URL != "" {
			addItem(SniffedMediaItem{
				Type:   "video",
				Format: "Direct File",
				Title:  target.Filename,
				URL:    target.URL,
				Source: "anpan_extractor",
			})
		}
	}

	// 2. Fetch page HTML and inspect for embedded media and M3U8
	req, errReq := http.NewRequestWithContext(ctx, "GET", targetUrl, nil)
	if errReq == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,vi;q=0.8")

		if resp, errDo := httpClient.Do(req); errDo == nil && resp.StatusCode == 200 {
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
			resp.Body.Close()
			htmlStr := string(bodyBytes)

			// Extract Page Title
			if result.PageTitle == "" {
				if m := titleRegex.FindStringSubmatch(htmlStr); len(m) > 1 {
					result.PageTitle = strings.TrimSpace(m[1])
				}
			}

			// M3U8 Streams
			for _, m := range m3u8Regex.FindAllString(htmlStr, -1) {
				clean := strings.ReplaceAll(m, `\/`, `/`)
				addItem(SniffedMediaItem{
					Type:   "video",
					Format: "M3U8 HLS Stream",
					Title:  "HLS Video Stream (.m3u8)",
					URL:    clean,
					Source: "stream_sniffer",
				})
			}

			// Direct Videos
			for _, m := range videoRegex.FindAllString(htmlStr, -1) {
				clean := strings.ReplaceAll(m, `\/`, `/`)
				addItem(SniffedMediaItem{
					Type:   "video",
					Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(strings.Split(clean, "?")[0]), ".")),
					Title:  filepath.Base(strings.Split(clean, "?")[0]),
					URL:    clean,
					Source: "video_sniffer",
				})
			}

			// Direct Audio
			for _, m := range audioRegex.FindAllString(htmlStr, -1) {
				clean := strings.ReplaceAll(m, `\/`, `/`)
				addItem(SniffedMediaItem{
					Type:   "audio",
					Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(strings.Split(clean, "?")[0]), ".")),
					Title:  filepath.Base(strings.Split(clean, "?")[0]),
					URL:    clean,
					Source: "audio_sniffer",
				})
			}

			// High-res Images
			imgMatches := imageRegex.FindAllString(htmlStr, -1)
			for idx, m := range imgMatches {
				if idx >= 15 {
					break
				}
				clean := strings.ReplaceAll(m, `\/`, `/`)
				addItem(SniffedMediaItem{
					Type:      "image",
					Format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(strings.Split(clean, "?")[0]), ".")),
					Title:     filepath.Base(strings.Split(clean, "?")[0]),
					URL:       clean,
					Thumbnail: clean,
					Source:    "image_sniffer",
				})
			}

			// Extracted page links helper
			for _, l := range ExtractPageLinks(htmlStr, targetUrl) {
				addItem(SniffedMediaItem{
					Type:   "video",
					Format: "Stream Link",
					Title:  "Extracted Video Stream",
					URL:    l,
					Source: "page_extractor",
				})
			}
		}
	}

	// 3. Scan with yt-dlp metadata engine (in background timeout)
	if scanInfo, _, errScan := ScanUrl(targetUrl, ""); errScan == nil && scanInfo != nil {
		if result.PageTitle == "" && scanInfo.Title != "" {
			result.PageTitle = scanInfo.Title
		}
		for _, entry := range scanInfo.Entries {
			thumb := entry.Thumbnail
			if thumb == "" && len(entry.Thumbnails) > 0 {
				thumb = entry.Thumbnails[len(entry.Thumbnails)-1].URL
			}
			addItem(SniffedMediaItem{
				Type:      "video",
				Format:    "Best Quality Video",
				Title:     entry.Title,
				URL:       entry.Url,
				Thumbnail: thumb,
				Source:    "ytdlp_engine",
			})
		}
	}

	if result.PageTitle == "" {
		result.PageTitle = targetUrl
	}
	result.TotalCount = len(result.Items)

	_ = json.NewEncoder(w).Encode(result)
}

// BrowserProxyHandler strips iframe blocking headers and injects a real-time DOM/network sniffer script.
func BrowserProxyHandler(w http.ResponseWriter, r *http.Request) {
	targetUrl := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetUrl == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(targetUrl, "http://") && !strings.HasPrefix(targetUrl, "https://") {
		targetUrl = "https://" + targetUrl
	}

	parsedTarget, errUrl := url.Parse(targetUrl)
	if errUrl != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", targetUrl, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Forward client Accept and user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", r.Header.Get("Accept"))
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,vi;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msgf("Proxy error fetching %s", targetUrl)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><style>body{background:#0a0b0d;color:#e3e2e5;font-family:sans-serif;display:flex;flex-direction:column;align-items:center;justify-content:center;height:90vh;text-align:center;padding:20px;}</style></head>
<body>
  <h2>Unable to load page inside sandbox</h2>
  <p style="color:#94a3b8;max-width:500px;">%s</p>
  <p style="margin-top:16px;">
    <a href="%s" target="_blank" style="color:#d0bcff;text-decoration:none;font-weight:bold;background:rgba(160,120,255,0.2);padding:8px 16px;border-radius:8px;">Open in System Browser</a>
  </p>
</body>
</html>`, err.Error(), targetUrl)
		return
	}
	defer resp.Body.Close()

	// Strip frame restrictions
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "x-frame-options" ||
			lk == "frame-options" ||
			lk == "content-security-policy" ||
			lk == "content-security-policy-report-only" ||
			lk == "cross-origin-embedder-policy" ||
			lk == "cross-origin-opener-policy" ||
			lk == "cross-origin-resource-policy" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	w.Header().Set("X-Frame-Options", "ALLOWALL")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(strings.ToLower(contentType), "text/html")

	if !isHTML {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}

	// Read HTML and inject <base href="..."> + sniffer script
	bodyBytes, errRead := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if errRead != nil {
		w.WriteHeader(resp.StatusCode)
		return
	}

	baseUrl := fmt.Sprintf("%s://%s%s", parsedTarget.Scheme, parsedTarget.Host, parsedTarget.Path)
	if !strings.HasSuffix(baseUrl, "/") && !strings.Contains(filepath.Base(parsedTarget.Path), ".") {
		baseUrl += "/"
	}

	snifferScript := fmt.Sprintf(`
<base href="%s">
<script>
(function() {
  var detected = new Set();
  function report(url, type, format, title) {
    if (!url || typeof url !== 'string' || detected.has(url)) return;
    detected.add(url);
    try {
      window.parent.postMessage({
        type: 'kv_browser_media_found',
        item: {
          url: url,
          type: type || 'video',
          format: format || 'Media Stream',
          title: title || document.title || url,
          thumbnail: ''
        }
      }, '*');
    } catch(e) {}
  }

  // Hook XHR
  var origOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url) {
    if (typeof url === 'string') {
      if (/\.m3u8/i.test(url)) report(url, 'video', 'M3U8 HLS', document.title);
      else if (/\.mpd/i.test(url)) report(url, 'video', 'DASH Stream', document.title);
      else if (/\.(mp4|webm|mkv)/i.test(url)) report(url, 'video', 'MP4 Video', document.title);
      else if (/\.(mp3|m4a|aac|flac)/i.test(url)) report(url, 'audio', 'Audio Stream', document.title);
    }
    return origOpen.apply(this, arguments);
  };

  // Hook Fetch
  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function(input, init) {
      var url = typeof input === 'string' ? input : (input && input.url ? input.url : '');
      if (url) {
        if (/\.m3u8/i.test(url)) report(url, 'video', 'M3U8 HLS', document.title);
        else if (/\.mpd/i.test(url)) report(url, 'video', 'DASH Stream', document.title);
        else if (/\.(mp4|webm|mkv)/i.test(url)) report(url, 'video', 'MP4 Video', document.title);
        else if (/\.(mp3|m4a|aac|flac)/i.test(url)) report(url, 'audio', 'Audio Stream', document.title);
      }
      return origFetch.apply(this, arguments);
    };
  }

  // Periodic DOM Scan
  function scanDOM() {
    document.querySelectorAll('video, audio, source').forEach(function(el) {
      var src = el.src || el.currentSrc || el.getAttribute('data-src');
      if (src && !src.startsWith('blob:')) {
        var isAudio = el.tagName === 'AUDIO' || /\.(mp3|m4a|aac|flac)/i.test(src);
        report(src, isAudio ? 'audio' : 'video', isAudio ? 'Audio Stream' : 'Video Stream', document.title);
      }
    });
  }

  window.addEventListener('DOMContentLoaded', scanDOM);
  window.addEventListener('load', function() {
    scanDOM();
    setInterval(scanDOM, 2500);
    try {
      window.parent.postMessage({ type: 'kv_browser_page_loaded', url: window.location.href, title: document.title }, '*');
    } catch(e) {}
  });
})();
</script>
`, baseUrl)

	html := string(bodyBytes)
	headIdx := strings.Index(strings.ToLower(html), "<head>")
	if headIdx != -1 {
		html = html[:headIdx+6] + snifferScript + html[headIdx+6:]
	} else {
		html = snifferScript + html
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write([]byte(html))
}

func isDirectMediaUrl(raw string) bool {
	lower := strings.ToLower(strings.Split(raw, "?")[0])
	for _, ext := range []string{".mp4", ".webm", ".mkv", ".mov", ".avi", ".flv", ".m3u8", ".mpd", ".mp3", ".m4a", ".aac", ".wav", ".flac", ".ogg", ".oga", ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	// Also treat known CDN / direct video URLs without extension as direct
	// YouTube googlevideo, TikTok CDN (tiktokcdn, v19-webapp, tos-alisg), and generic mime params
	l := strings.ToLower(raw)
	if strings.Contains(l, "googlevideo.com/videoplayback") {
		return true
	}
	if strings.Contains(l, "tiktokcdn") || strings.Contains(l, "v16-webapp") || strings.Contains(l, "v19-webapp") || strings.Contains(l, "/video/tos/") {
		return true
	}
	if strings.Contains(l, "mime_type=video") || strings.Contains(l, "mime=video") {
		return true
	}
	return false
}

// BrowserResolveHandler resolves a page URL (e.g. TikTok/YouTube) to a direct CDN media URL via yt-dlp
func BrowserResolveHandler(w http.ResponseWriter, r *http.Request) {
	targetUrl := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetUrl == "" {
		http.Error(w, `{"error":"Missing url parameter"}`, http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(targetUrl, "http://") && !strings.HasPrefix(targetUrl, "https://") {
		targetUrl = "https://" + targetUrl
	}
	cookies := strings.TrimSpace(r.URL.Query().Get("cookies"))
	if cookies == "" {
		cookies = strings.TrimSpace(r.Header.Get("X-Cookies"))
	}

	// If already direct media file, return as-is
	if isDirectMediaUrl(targetUrl) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"url":  targetUrl,
			"type": "direct",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	args := []string{
		"-g",
		"--no-playlist",
		"--no-warnings",
		"--no-check-certificates",
		"--extractor-args", "instagram:image_persist=1;threads:app_version=30.0.0",
	}

	if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
		args = append(args, "--impersonate", impersonate)
	}

	if cookies != "" {
		tmpCookie, cleanup, err := CreateEphemeralCookieFile(cookies, targetUrl)
		if err == nil && tmpCookie != "" {
			defer cleanup()
			args = append(args, "--cookies", tmpCookie)
		}
	} else {
		cp := getCookiesPath()
		if workingCookies := getWorkingCookiesPath(cp); workingCookies != "" {
			args = append(args, "--cookies", workingCookies)
		}
	}

	for arg, value := range getEnvVars() {
		args = append(args, arg)
		if value != "" {
			args = append(args, value)
		}
	}

	args = append(args, targetUrl)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		log.Warn().Msgf("BrowserResolve failed for %s: %s", targetUrl, errMsg)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		return
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var directUrl string
	// Prefer last non-empty line that looks like http URL; if multiple lines (video+audio), we need to fallback to stream
	if len(lines) == 1 {
		directUrl = strings.TrimSpace(lines[0])
	} else if len(lines) > 1 {
		// Multiple URLs (e.g. DASH separate video/audio) — cannot play directly; signal to use stream endpoint
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "DASH streams require server-side muxing; use /api/browser/stream", "hint": "use_stream"})
		return
	}

	if directUrl == "" || (!strings.HasPrefix(directUrl, "http://") && !strings.HasPrefix(directUrl, "https://")) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to resolve direct media URL"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"url":  directUrl,
		"type": "resolved",
	})
}

// BrowserStreamHandler streams media for in-app preview.
// For direct media URLs it proxies the remote file with Range/CORS support.
// For page URLs it runs yt-dlp to pipe merged mp4 directly to the client.
func BrowserStreamHandler(w http.ResponseWriter, r *http.Request) {
	targetUrl := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetUrl == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(targetUrl, "http://") && !strings.HasPrefix(targetUrl, "https://") {
		targetUrl = "https://" + targetUrl
	}
	cookies := strings.TrimSpace(r.URL.Query().Get("cookies"))
	if cookies == "" {
		cookies = strings.TrimSpace(r.Header.Get("X-Cookies"))
	}

	// Handle CORS preflight
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type, X-Cookies")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges, Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodHead && !isDirectMediaUrl(targetUrl) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Direct media URL => proxy with Range forwarding (use client without timeout to allow large streams)
	if isDirectMediaUrl(targetUrl) {
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		method := r.Method
		if method != http.MethodHead {
			method = http.MethodGet
		}
		req, err := http.NewRequestWithContext(ctx, method, targetUrl, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Referer", targetUrl)
		if rh := r.Header.Get("Range"); rh != "" {
			req.Header.Set("Range", rh)
		}
		// Use transport with no timeout for large files
		proxyClient := &http.Client{
			Timeout: 0,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
		resp, err := proxyClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			lk := strings.ToLower(k)
			if lk == "content-length" || lk == "content-type" || lk == "accept-ranges" || lk == "content-range" || lk == "cache-control" || lk == "expires" {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", getMimeType(targetUrl))
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// HLS m3u8 rewriting: proxy segment URLs to bypass CORS and handle relative paths
		ct := w.Header().Get("Content-Type")
		lowerTarget := strings.ToLower(targetUrl)
		isM3u8 := strings.Contains(strings.ToLower(ct), "mpegurl") || strings.Contains(ct, "application/x-mpegURL") || strings.HasSuffix(lowerTarget, ".m3u8")
		if isM3u8 && resp.StatusCode == http.StatusOK && r.Method != http.MethodHead {
			bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
			if err == nil {
				rewritten := rewriteM3U8Content(string(bodyBytes), targetUrl)
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))
				w.Header().Del("Content-Range")
				w.Header().Del("Accept-Ranges")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(rewritten))
				return
			}
		}
		w.WriteHeader(resp.StatusCode)
		if r.Method != http.MethodHead {
			_, _ = io.Copy(w, resp.Body)
		}
		return
	}

	// Page URL (TikTok, YouTube, etc.) => stream via yt-dlp pipe with retry for transient TikTok failures
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	baseArgs := []string{
		"-o", "-",
		"--no-playlist",
		"--no-warnings",
		"--no-check-certificates",
		"--extractor-args", "instagram:image_persist=1;threads:app_version=30.0.0",
		"--remux-video", "mp4",
		"--merge-output-format", "mp4",
	}
	if !strings.Contains(targetUrl, "tiktok.com") {
		baseArgs = append(baseArgs, "--format", "b/bv*+ba/best")
	}
	if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
		baseArgs = append(baseArgs, "--impersonate", impersonate)
	}
	var tmpCookie string
	var cleanup func()
	if cookies != "" {
		var err error
		tmpCookie, cleanup, err = CreateEphemeralCookieFile(cookies, targetUrl)
		if err == nil && tmpCookie != "" {
			defer cleanup()
			baseArgs = append(baseArgs, "--cookies", tmpCookie)
		}
	} else {
		cp := getCookiesPath()
		if workingCookies := getWorkingCookiesPath(cp); workingCookies != "" {
			baseArgs = append(baseArgs, "--cookies", workingCookies)
		}
	}
	for arg, value := range getEnvVars() {
		baseArgs = append(baseArgs, arg)
		if value != "" {
			baseArgs = append(baseArgs, value)
		}
	}

	var lastErrMsg string
	for attempt := 1; attempt <= 2; attempt++ {
		args := append([]string(nil), baseArgs...)
		args = append(args, targetUrl)

		cmd := exec.CommandContext(ctx, "yt-dlp", args...)
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := cmd.Start(); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		var stderrBuf bytes.Buffer
		var stderrDone = make(chan struct{})
		go func() {
			_, _ = io.Copy(&stderrBuf, stderrPipe)
			close(stderrDone)
		}()

		flusher, _ := w.(http.Flusher)
		var totalWritten int64
		var headerSent bool
		sendHeader := func() {
			if !headerSent {
				w.Header().Set("Content-Type", "video/mp4")
				w.Header().Set("Accept-Ranges", "bytes")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Access-Control-Allow-Origin", "*")
				headerSent = true
			}
		}
		buf := make([]byte, 32*1024)
		for {
			n, readErr := stdoutPipe.Read(buf)
			if n > 0 {
				sendHeader()
				if _, werr := w.Write(buf[:n]); werr != nil {
					_ = cmd.Process.Kill()
					break
				}
				totalWritten += int64(n)
				if flusher != nil {
					flusher.Flush()
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					log.Warn().Msgf("BrowserStream read error for %s: %v", targetUrl, readErr)
				}
				break
			}
			select {
			case <-ctx.Done():
				_ = cmd.Process.Kill()
				return
			default:
			}
		}
		_ = cmd.Wait()
		<-stderrDone
		if ctx.Err() == context.Canceled {
			return
		}
		if totalWritten > 0 {
			if stderrBuf.Len() > 0 {
				log.Debug().Msgf("BrowserStream yt-dlp stderr for %s: %s", targetUrl, stderrBuf.String())
			}
			return
		}
		lastErrMsg = strings.TrimSpace(stderrBuf.String())
		if lastErrMsg == "" {
			lastErrMsg = "Failed to fetch video stream (yt-dlp returned no data)"
		}
		log.Warn().Msgf("BrowserStream attempt %d/2 failed for %s: %s", attempt, targetUrl, lastErrMsg)
		if attempt < 2 {
			select {
			case <-time.After(1500 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			// retry - need to not have sent headers yet
			if headerSent {
				// Already sent headers but no data? shouldn't happen; just return
				return
			}
			continue
		}
		// final failure
		if len(lastErrMsg) > 800 {
			lastErrMsg = lastErrMsg[:800]
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": lastErrMsg})
		return
	}
	if lastErrMsg != "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": lastErrMsg})
	}
}

func rewriteM3U8Content(content string, baseUrl string) string {
	baseParsed, err := url.Parse(baseUrl)
	if err != nil {
		return content
	}
	baseDir := baseParsed.Scheme + "://" + baseParsed.Host
	if idx := strings.LastIndex(baseParsed.Path, "/"); idx != -1 {
		baseDir += baseParsed.Path[:idx+1]
	} else {
		baseDir += "/"
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		absUrl := trimmed
		if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
			if strings.HasPrefix(trimmed, "/") {
				absUrl = baseParsed.Scheme + "://" + baseParsed.Host + trimmed
			} else {
				absUrl = baseDir + trimmed
			}
		}
		// Proxy via our endpoint to handle CORS & range
		proxied := "/api/browser/proxy-media?url=" + url.QueryEscape(absUrl)
		lines[i] = proxied
	}
	return strings.Join(lines, "\n")
}

// BrowserProxyMediaHandler is an alias for BrowserStreamHandler for direct media proxying (used by frontend for CORS bypass)
func BrowserProxyMediaHandler(w http.ResponseWriter, r *http.Request) {
	BrowserStreamHandler(w, r)
}
