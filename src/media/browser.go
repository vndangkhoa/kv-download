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
	"sort"
	"strings"
	"sync"
	"time"

	"kv-download/src/anpan"

	"github.com/rs/zerolog/log"
)

type SniffedMediaItem struct {
	ID         string `json:"id"`
	Type       string `json:"type"`      // "video", "audio", "image", "stream"
	Format     string `json:"format"`    // "M3U8 HLS", "MP4 1080p", "MP3", "JPG Image", etc.
	Title      string `json:"title"`     // Title or filename
	URL        string `json:"url"`       // Stream or direct media URL
	Thumbnail  string `json:"thumbnail"` // Thumbnail if available
	Size       string `json:"size"`      // Formatted size if known
	Source     string `json:"source"`    // "page_dom", "network_sniffer", "ytdlp_engine", "anpan_extractor"
	Duration   string `json:"duration,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Channel    string `json:"channel,omitempty"`
	IsStream   bool   `json:"isStream,omitempty"`
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

func normalizeSniffUrl(raw string) string {
	cleaned := sanitizeMediaUrl(raw)
	if cleaned == "" {
		return ""
	}
	u, err := url.Parse(cleaned)
	if err != nil {
		return cleaned
	}
	u.Fragment = ""
	q := u.Query()
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || lk == "fbclid" || lk == "gclid" || lk == "mc_cid" || lk == "mc_eid" || lk == "ref" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

var smallDimRegex = regexp.MustCompile(`(?i)(?:^|[/_\-=])(\d{2,4})x(\d{2,4})(?:[/_\-.&]|$)`)

func isNoisyImageURL(raw string) bool {
	lower := strings.ToLower(raw)
	for _, kw := range []string{
		"avatar", "logo", "icon", "badge", "emoji", "spinner", "loader",
		"placeholder", "banner", "favicon", "sprite", "1x1", "pixel",
		"tracking", "/ads/", "doubleclick", "googlesyndication", ".svg",
		"/data/avatars/", "/smilies/", "/stickers/", "gravatar.com",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	if m := smallDimRegex.FindStringSubmatch(lower); len(m) == 3 {
		var w, h int
		_, _ = fmt.Sscanf(m[1], "%d", &w)
		_, _ = fmt.Sscanf(m[2], "%d", &h)
		if (w > 0 && w < 240) || (h > 0 && h < 180) {
			return true
		}
	}
	return false
}

func deriveCleanMediaTitle(rawURL, candidate, pageTitle, prefix string, index int) string {
	c := strings.TrimSpace(html.UnescapeString(candidate))
	if c != "" && !strings.EqualFold(c, pageTitle) && !strings.HasPrefix(c, "http") {
		return c
	}
	base := filepath.Base(strings.Split(rawURL, "?")[0])
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	// Check if stem looks meaningful (not just random hex/numeric/master/index)
	isGenericStem := stem == "" || stem == "master" || stem == "index" || stem == "playlist" || stem == "video" || len(stem) <= 3
	if !isGenericStem {
		hexLike, _ := regexp.MatchString(`^[0-9a-fA-F_-]{16,}$`, stem)
		numLike, _ := regexp.MatchString(`^\d+$`, stem)
		if !hexLike && !numLike {
			clean := strings.ReplaceAll(strings.ReplaceAll(stem, "-", " "), "_", " ")
			if pageTitle != "" && !strings.HasPrefix(pageTitle, "http") && len(clean) < 28 {
				return fmt.Sprintf("%s — %s", pageTitle, clean)
			}
			return clean
		}
	}
	if pageTitle != "" && !strings.HasPrefix(pageTitle, "http") {
		if index > 1 {
			return fmt.Sprintf("%s (%s #%d)", pageTitle, prefix, index)
		}
		return pageTitle
	}
	if index > 1 {
		return fmt.Sprintf("%s #%d", prefix, index)
	}
	return base
}

var (
	m3u8Regex       = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:m3u8|mpd)(?:\?[^\s"'<>\\]*)?`)
	videoRegex      = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:mp4|webm|mkv|mov|avi|flv)(?:\?[^\s"'<>\\]*)?`)
	audioRegex      = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:mp3|m4a|aac|wav|flac|ogg|oga)(?:\?[^\s"'<>\\]*)?`)
	imgTagRegex     = regexp.MustCompile(`(?is)<img\s+[^>]*?(?:src|data-src|data-original)=["']([^"']+)["'][^>]*>`)
	altAttrRegex    = regexp.MustCompile(`(?is)(?:alt|title)=["']([^"']+)["']`)
	ogVideoRegex    = regexp.MustCompile(`(?i)<meta\s+[^>]*?(?:property|name)=["'](?:og:video(?::url|:secure_url)?|twitter:player:stream)["'][^>]*?content=["']([^"']+)["']`)
	jsonLdScriptRe  = regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>([\s\S]*?)</script>`)
	titleRegex      = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
	ytEmbedRegex    = regexp.MustCompile(`(?:youtube\.com/(?:embed/|v/|watch\?v=|shorts/)|youtu\.be/)([a-zA-Z0-9_-]{11})`)
	vimeoEmbedRegex = regexp.MustCompile(`(?:vimeo\.com/(?:video/)?|player\.vimeo\.com/video/)(\d+)`)
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
	normalizedTarget := normalizeSniffUrl(targetUrl)

	w.Header().Set("Content-Type", "application/json")

	result := BrowserSniffResult{
		URL:   targetUrl,
		Items: make([]SniffedMediaItem, 0),
	}

	seenURLs := make(map[string]bool)
	var mu sync.Mutex

	addItem := func(item SniffedMediaItem) bool {
		mu.Lock()
		defer mu.Unlock()
		normURL := normalizeSniffUrl(item.URL)
		if normURL == "" || seenURLs[normURL] {
			return false
		}
		seenURLs[normURL] = true
		item.URL = sanitizeMediaUrl(item.URL)
		if item.ID == "" {
			item.ID = fmt.Sprintf("sniff_%d", len(result.Items)+1)
		}
		if item.Title == "" {
			item.Title = filepath.Base(strings.Split(item.URL, "?")[0])
		}
		if item.Thumbnail != "" {
			item.Thumbnail = sanitizeMediaUrl(item.Thumbnail)
		}
		result.Items = append(result.Items, item)
		return true
	}

	// 1. Check with Anpan Multi-source Engines (Archive, Booru, Cloud, Pixiv, Imgur)
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
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

	// 2. Fetch page HTML and inspect for embedded media, JSON-LD VideoObject, og:video and M3U8
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
					result.PageTitle = strings.TrimSpace(html.UnescapeString(m[1]))
				}
			}

			// Extract OpenGraph Image and Title
			ogImage := ""
			ogImgRe := regexp.MustCompile(`(?i)<meta\s+[^>]*?(?:property|name)=["'](?:og:image|twitter:image)["'][^>]*?content=["']([^"']+)["']`)
			if m := ogImgRe.FindStringSubmatch(htmlStr); len(m) > 1 {
				ogImage = sanitizeMediaUrl(m[1])
			}
			ogTitleRe := regexp.MustCompile(`(?i)<meta\s+[^>]*?(?:property|name)=["'](?:og:title|twitter:title)["'][^>]*?content=["']([^"']+)["']`)
			if m := ogTitleRe.FindStringSubmatch(htmlStr); len(m) > 1 {
				tVal := strings.TrimSpace(html.UnescapeString(m[1]))
				if result.PageTitle == "" || (len(tVal) > len(result.PageTitle) && len(tVal) < 140) {
					result.PageTitle = tVal
				}
			}

			videoIdx := 0

			// OpenGraph Video tags
			for _, m := range ogVideoRegex.FindAllStringSubmatch(htmlStr, -1) {
				if len(m) > 1 {
					videoIdx++
					vUrl := sanitizeMediaUrl(m[1])
					addItem(SniffedMediaItem{
						Type:      "video",
						Format:    "OpenGraph Video",
						Title:     deriveCleanMediaTitle(vUrl, "", result.PageTitle, "Video", videoIdx),
						URL:       vUrl,
						Thumbnail: ogImage,
						IsStream:  strings.Contains(strings.ToLower(vUrl), ".m3u8"),
						Source:    "og_video",
					})
				}
			}

			// JSON-LD VideoObject extraction
			for _, ldMatch := range jsonLdScriptRe.FindAllStringSubmatch(htmlStr, -1) {
				if len(ldMatch) < 2 {
					continue
				}
				var vo struct {
					Type         string `json:"@type"`
					Name         string `json:"name"`
					ContentURL   string `json:"contentUrl"`
					EmbedURL     string `json:"embedUrl"`
					ThumbnailURL string `json:"thumbnailUrl"`
					Duration     string `json:"duration"`
				}
				if json.Unmarshal([]byte(ldMatch[1]), &vo) == nil && strings.EqualFold(vo.Type, "VideoObject") {
					targetMedia := vo.ContentURL
					if targetMedia == "" {
						targetMedia = vo.EmbedURL
					}
					if targetMedia != "" {
						videoIdx++
						thumb := vo.ThumbnailURL
						if thumb == "" {
							thumb = ogImage
						}
						addItem(SniffedMediaItem{
							Type:      "video",
							Format:    "VideoObject",
							Title:     deriveCleanMediaTitle(targetMedia, vo.Name, result.PageTitle, "Video", videoIdx),
							URL:       targetMedia,
							Thumbnail: thumb,
							Duration:  vo.Duration,
							IsStream:  strings.Contains(strings.ToLower(targetMedia), ".m3u8"),
							Source:    "json_ld",
						})
					}
				}
			}

			// M3U8 / MPD Streams
			for _, m := range m3u8Regex.FindAllString(htmlStr, -1) {
				clean := strings.ReplaceAll(m, `\/`, `/`)
				videoIdx++
				fmtLabel := "M3U8 HLS Stream"
				if strings.Contains(strings.ToLower(clean), ".mpd") {
					fmtLabel = "MPEG-DASH Stream"
				}
				addItem(SniffedMediaItem{
					Type:      "video",
					Format:    fmtLabel,
					Title:     deriveCleanMediaTitle(clean, "", result.PageTitle, "Stream", videoIdx),
					URL:       clean,
					Thumbnail: ogImage,
					IsStream:  true,
					Source:    "stream_sniffer",
				})
			}

			// Direct Videos
			for _, m := range videoRegex.FindAllString(htmlStr, -1) {
				clean := strings.ReplaceAll(m, `\/`, `/`)
				videoIdx++
				addItem(SniffedMediaItem{
					Type:      "video",
					Format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(strings.Split(clean, "?")[0]), ".")),
					Title:     deriveCleanMediaTitle(clean, "", result.PageTitle, "Video", videoIdx),
					URL:       clean,
					Thumbnail: ogImage,
					Source:    "video_sniffer",
				})
			}

			// Direct Audio
			audioIdx := 0
			for _, m := range audioRegex.FindAllString(htmlStr, -1) {
				clean := strings.ReplaceAll(m, `\/`, `/`)
				audioIdx++
				addItem(SniffedMediaItem{
					Type:      "audio",
					Format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(strings.Split(clean, "?")[0]), ".")),
					Title:     deriveCleanMediaTitle(clean, "", result.PageTitle, "Audio", audioIdx),
					URL:       clean,
					Thumbnail: ogImage,
					Source:    "audio_sniffer",
				})
			}

			// Embedded YouTube Video Detection
			for _, ym := range ytEmbedRegex.FindAllStringSubmatch(htmlStr, -1) {
				if len(ym) > 1 {
					vid := ym[1]
					ytUrl := "https://www.youtube.com/watch?v=" + vid
					videoIdx++
					addItem(SniffedMediaItem{
						Type:      "video",
						Format:    "YouTube Embed",
						Title:     deriveCleanMediaTitle(ytUrl, "", result.PageTitle, "YouTube Clip", videoIdx),
						URL:       ytUrl,
						Thumbnail: "https://i.ytimg.com/vi/" + vid + "/hqdefault.jpg",
						Source:    "youtube_embed",
					})
				}
			}

			// Embedded Vimeo Detection
			for _, vm := range vimeoEmbedRegex.FindAllStringSubmatch(htmlStr, -1) {
				if len(vm) > 1 {
					videoIdx++
					vUrl := "https://vimeo.com/" + vm[1]
					addItem(SniffedMediaItem{
						Type:      "video",
						Format:    "Vimeo Embed",
						Title:     deriveCleanMediaTitle(vUrl, "", result.PageTitle, "Vimeo Clip", videoIdx),
						URL:       vUrl,
						Thumbnail: ogImage,
						Source:    "vimeo_embed",
					})
				}
			}

			// Extracted page links helper
			for _, l := range ExtractPageLinks(htmlStr, targetUrl) {
				if normalizeSniffUrl(l) == normalizedTarget {
					continue
				}
				videoIdx++
				addItem(SniffedMediaItem{
					Type:      "video",
					Format:    "Extracted Stream",
					Title:     deriveCleanMediaTitle(l, "", result.PageTitle, "Stream", videoIdx),
					URL:       l,
					Thumbnail: ogImage,
					IsStream:  strings.Contains(strings.ToLower(l), ".m3u8"),
					Source:    "page_extractor",
				})
			}

			// Filtered high-res <img> tags only (avoid scraping noisy JSON/CSS/avatar/icon URLs)
			parsedPage, _ := url.Parse(targetUrl)
			imgAdded := 0
			for _, match := range imgTagRegex.FindAllStringSubmatch(htmlStr, -1) {
				if imgAdded >= 12 {
					break
				}
				if len(match) < 2 {
					continue
				}
				rawImg := strings.TrimSpace(html.UnescapeString(match[1]))
				if rawImg == "" || strings.HasPrefix(rawImg, "data:") {
					continue
				}
				if parsedPage != nil {
					if relU, errU := url.Parse(rawImg); errU == nil {
						rawImg = parsedPage.ResolveReference(relU).String()
					}
				}
				if isNoisyImageURL(rawImg) {
					continue
				}
				imgTitle := ""
				if am := altAttrRegex.FindStringSubmatch(match[0]); len(am) > 1 {
					imgTitle = strings.TrimSpace(html.UnescapeString(am[1]))
				}
				if imgTitle == "" {
					base := filepath.Base(strings.Split(rawImg, "?")[0])
					imgTitle = strings.TrimSuffix(base, filepath.Ext(base))
				}
				if addItem(SniffedMediaItem{
					Type:      "image",
					Format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(strings.Split(rawImg, "?")[0]), ".")),
					Title:     imgTitle,
					URL:       rawImg,
					Thumbnail: rawImg,
					Source:    "image_sniffer",
				}) {
					imgAdded++
				}
			}
		}
	}

	// 3. Scan with yt-dlp metadata engine — ignore generic fallback entry where entry.Url == targetUrl with no formats
	if scanInfo, _, errScan := ScanUrl(targetUrl, ""); errScan == nil && scanInfo != nil {
		if (result.PageTitle == "" || result.PageTitle == targetUrl) && scanInfo.Title != "" && scanInfo.Title != targetUrl {
			result.PageTitle = scanInfo.Title
		}
		for _, entry := range scanInfo.Entries {
			if entry.Url == "" {
				continue
			}
			isSameDummy := normalizeSniffUrl(entry.Url) == normalizedTarget &&
				len(entry.Formats) == 0 && entry.Duration <= 0 && !scanInfo.IsPlaylist &&
				(entry.Title == "" || entry.Title == targetUrl)
			if isSameDummy {
				continue
			}
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
				Duration:  formatDurationFloat(entry.Duration),
				Channel:   entry.Channel,
				Source:    "ytdlp_engine",
			})
		}
	}

	// Sort items: videos/streams first, then audio, then images
	typeRank := func(t string) int {
		switch t {
		case "video", "stream":
			return 0
		case "audio":
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		ri, rj := typeRank(result.Items[i].Type), typeRank(result.Items[j].Type)
		if ri != rj {
			return ri < rj
		}
		if result.Items[i].IsStream != result.Items[j].IsStream {
			return result.Items[i].IsStream
		}
		return false
	})

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

	if strings.Contains(targetUrl, "youtube.com") || strings.Contains(targetUrl, "youtu.be") {
		if strings.HasPrefix(targetUrl, "https://www.youtube.com") {
			targetUrl = strings.Replace(targetUrl, "https://www.youtube.com", "https://m.youtube.com", 1)
		} else if strings.HasPrefix(targetUrl, "https://youtube.com") {
			targetUrl = strings.Replace(targetUrl, "https://youtube.com", "https://m.youtube.com", 1)
		}
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
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	if strings.Contains(targetUrl, "m.youtube.com") {
		ua = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1"
	}
	req.Header.Set("User-Agent", ua)
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
  var currentPageUrl = %q;
  var detected = new Set();
  function normUrl(u) {
    try {
      var parsed = new URL(u, document.baseURI);
      parsed.hash = '';
      ['utm_source','utm_medium','utm_campaign','utm_term','utm_content','fbclid','gclid'].forEach(function(k){ parsed.searchParams.delete(k); });
      return parsed.toString();
    } catch(e) { return u; }
  }
  function report(url, type, format, title, thumb, isStream) {
    if (!url || typeof url !== 'string') return;
    if (url.startsWith('blob:') || url.startsWith('data:')) return;
    var key = normUrl(url);
    if (detected.has(key)) return;
    detected.add(key);
    try {
      window.parent.postMessage({
        type: 'kv_browser_media_found',
        item: {
          url: key,
          type: type || 'video',
          format: format || 'Media Stream',
          title: title || document.title || key,
          thumbnail: thumb || '',
          isStream: !!isStream
        }
      }, '*');
    } catch(e) {}
  }

  // Intercept link clicks so browsing stays inside KV Download proxy
  document.addEventListener('click', function(e) {
    var a = e.target && e.target.closest ? e.target.closest('a[href]') : null;
    if (!a) return;
    var rawHref = a.getAttribute('href') || '';
    if (!rawHref || rawHref.startsWith('#') || rawHref.startsWith('javascript:') || rawHref.startsWith('mailto:') || rawHref.startsWith('tel:') || a.hasAttribute('download')) return;
    if (a.getAttribute('target') === '_blank' && e.ctrlKey) return;
    try {
      var resolved = new URL(rawHref, currentPageUrl).toString();
      if (resolved.startsWith('http://') || resolved.startsWith('https://')) {
        e.preventDefault();
        window.parent.postMessage({ type: 'kv_browser_navigate', url: resolved }, '*');
        window.location.href = '/api/browser/proxy?url=' + encodeURIComponent(resolved);
      }
    } catch(err) {}
  }, true);

  // Hook XHR
  var origOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url) {
    if (typeof url === 'string') {
      if (/\.m3u8/i.test(url)) report(url, 'video', 'M3U8 HLS', document.title, '', true);
      else if (/\.mpd/i.test(url)) report(url, 'video', 'DASH Stream', document.title, '', true);
      else if (/\.(mp4|webm|mkv)(\?|$)/i.test(url)) report(url, 'video', 'MP4 Video', document.title, '', false);
      else if (/\.(mp3|m4a|aac|flac)(\?|$)/i.test(url)) report(url, 'audio', 'Audio Stream', document.title, '', false);
    }
    return origOpen.apply(this, arguments);
  };

  // Hook Fetch
  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function(input, init) {
      var url = typeof input === 'string' ? input : (input && input.url ? input.url : '');
      if (url) {
        if (/\.m3u8/i.test(url)) report(url, 'video', 'M3U8 HLS', document.title, '', true);
        else if (/\.mpd/i.test(url)) report(url, 'video', 'DASH Stream', document.title, '', true);
        else if (/\.(mp4|webm|mkv)(\?|$)/i.test(url)) report(url, 'video', 'MP4 Video', document.title, '', false);
        else if (/\.(mp3|m4a|aac|flac)(\?|$)/i.test(url)) report(url, 'audio', 'Audio Stream', document.title, '', false);
      }
      return origFetch.apply(this, arguments);
    };
  }

  // Periodic DOM Scan
  function scanDOM() {
    var ogImage = '';
    var metaOg = document.querySelector('meta[property="og:image"], meta[name="twitter:image"]');
    if (metaOg) ogImage = metaOg.getAttribute('content') || '';

    // 1. Direct HTML5 media elements
    document.querySelectorAll('video').forEach(function(el, idx) {
      var src = el.src || el.currentSrc || el.getAttribute('data-src') || el.getAttribute('data-url') || el.getAttribute('data-mp4') || el.getAttribute('data-hls');
      if (!src) {
        var sourceEl = el.querySelector('source');
        if (sourceEl) src = sourceEl.src || sourceEl.getAttribute('data-src') || sourceEl.getAttribute('src');
      }
      var poster = el.poster || el.getAttribute('poster') || el.getAttribute('data-poster') || ogImage;
      var title = el.getAttribute('title') || el.getAttribute('aria-label') || '';
      if (!title) {
        var container = el.closest('figure, article, .thread-content, .video-container, div');
        if (container) {
          var heading = container.querySelector('h1, h2, h3, h4, figcaption, .title');
          if (heading) title = heading.textContent.trim();
        }
      }
      if (!title) title = (document.title || 'Video') + (idx > 0 ? ' #' + (idx + 1) : '');
      if (src) {
        var isHls = /\.m3u8/i.test(src);
        report(src, 'video', isHls ? 'M3U8 HLS' : 'MP4 Video', title, poster, isHls);
      }
    });

    document.querySelectorAll('audio').forEach(function(el) {
      var src = el.src || el.currentSrc || el.getAttribute('data-src') || el.getAttribute('data-url');
      if (!src) {
        var sourceEl = el.querySelector('source');
        if (sourceEl) src = sourceEl.src || sourceEl.getAttribute('data-src') || sourceEl.getAttribute('src');
      }
      if (src) {
        report(src, 'audio', 'Audio Stream', document.title, ogImage, false);
      }
    });

    // 2. Embedded Video Iframes (YouTube, Facebook, Vimeo, TikTok, etc.)
    document.querySelectorAll('iframe').forEach(function(el) {
      var s = el.src || el.getAttribute('data-src') || el.getAttribute('src');
      if (!s) return;
      if (s.indexOf('youtube.com/embed/') !== -1 || s.indexOf('youtu.be') !== -1 || s.indexOf('youtube-nocookie.com/embed/') !== -1) {
        var m = s.match(/(?:embed\/|v=|\/vi?\/)([a-zA-Z0-9_-]{11})/);
        if (m) {
          var vid = m[1];
          report('https://www.youtube.com/watch?v=' + vid, 'video', 'YouTube Embed', document.title || 'YouTube Video', 'https://i.ytimg.com/vi/' + vid + '/hqdefault.jpg', false);
        }
      } else if (s.indexOf('facebook.com/plugins/video.php') !== -1) {
        var match = s.match(/href=([^&]+)/);
        if (match) {
          report(decodeURIComponent(match[1]), 'video', 'Facebook Video', 'Facebook Video', ogImage, false);
        }
      } else if (s.indexOf('player.vimeo.com') !== -1 || s.indexOf('vimeo.com') !== -1) {
        report(s, 'video', 'Vimeo Video', document.title || 'Vimeo Video', ogImage, false);
      } else if (s.indexOf('tiktok.com') !== -1) {
        report(s, 'video', 'TikTok Video', document.title || 'TikTok Video', ogImage, false);
      }
    });

    // 3. Large content images only (ignore avatars, icons, banners & feed thumbnails < 320px)
    document.querySelectorAll('article img, figure img, .content img, .thread-content img').forEach(function(img) {
      var isrc = img.src || img.getAttribute('data-src') || img.getAttribute('data-original');
      if (!isrc || (!isrc.startsWith('http://') && !isrc.startsWith('https://'))) return;
      if (/(?:icon|logo|avatar|badge|emoji|btn|button|banner|sprite|svg)/i.test(isrc)) return;
      if (img.naturalWidth > 0 && img.naturalWidth < 320) return;
      var fname = isrc.split('?')[0].split('/').pop() || 'Image';
      var ititle = (img.alt && img.alt.trim()) || (img.title && img.title.trim()) || fname;
      report(isrc, 'image', 'Image', ititle, isrc, false);
    });
  }

  window.addEventListener('DOMContentLoaded', scanDOM);
  window.addEventListener('load', function() {
    scanDOM();
    setInterval(scanDOM, 2500);
    try {
      window.parent.postMessage({ type: 'kv_browser_page_loaded', url: currentPageUrl, title: document.title }, '*');
    } catch(e) {}
  });
})();
</script>
`, baseUrl, targetUrl)

	htmlStr := string(bodyBytes)
	// When host required DoH fallback (ISP DNS blocked), proxy same-host images/assets via /api/browser/proxy-media
	if isHostDNSBlocked(parsedTarget.Hostname()) {
		subRe := regexp.MustCompile(`(?i)\b(src|poster)=["'](https?://[^"']+|/[^"']+)["']`)
		htmlStr = subRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
			sub := subRe.FindStringSubmatch(m)
			if len(sub) != 3 {
				return m
			}
			attr := sub[1]
			rawVal := sub[2]
			if strings.HasPrefix(rawVal, "/api/browser/") || strings.HasPrefix(rawVal, "data:") {
				return m
			}
			abs := rawVal
			if strings.HasPrefix(rawVal, "/") {
				abs = parsedTarget.Scheme + "://" + parsedTarget.Host + rawVal
			}
			return fmt.Sprintf(`%s="/api/browser/proxy-media?asset=1&url=%s"`, attr, url.QueryEscape(abs))
		})
	}

	headIdx := strings.Index(strings.ToLower(htmlStr), "<head>")
	if headIdx != -1 {
		htmlStr = htmlStr[:headIdx+6] + snifferScript + htmlStr[headIdx+6:]
	} else {
		htmlStr = snifferScript + htmlStr
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write([]byte(htmlStr))
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

	// Direct media/asset URL => proxy with Range forwarding and resilient DoH DNS transport
	if r.URL.Query().Get("asset") == "1" || isDirectMediaUrl(targetUrl) {
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
		// Use resilient transport with no request timeout for large streams
		proxyClient := NewResilientHTTPClient(0)
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
		"--format-sort", "res,vcodec:avc:vp9:av1,acodec:m4a:aac:opus,ext:mp4",
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
