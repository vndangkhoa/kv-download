package media

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
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
