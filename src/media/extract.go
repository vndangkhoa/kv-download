package media

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ExtractPageLinks scans HTML text for the primary active video stream on the page
func ExtractPageLinks(htmlContent string, baseURL string) []string {
	var results []string
	seen := make(map[string]bool)

	addResult := func(rawUrl string) {
		cleaned := strings.TrimRight(strings.ReplaceAll(rawUrl, `\/`, `/`), `)]};,'">`)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" && (strings.HasPrefix(cleaned, "http://") || strings.HasPrefix(cleaned, "https://")) && !seen[cleaned] {
			seen[cleaned] = true
			results = append(results, cleaned)
		}
	}

	m3u8Pat := regexp.MustCompile(`https?://[^\s"'<>]+\.m3u8(?:\?[^\s"'<>]*)?`)
	sourcePat := regexp.MustCompile(`(?:file|src|source)\s*:\s*['"](https?://[^'"]+\.(?:m3u8|mp4|mpd|webm)[^'"]*)['"]`)

	parsedBase, _ := url.Parse(baseURL)
	hostPrefix := ""
	if parsedBase != nil && parsedBase.Host != "" {
		hostPrefix = parsedBase.Scheme + "://" + parsedBase.Host
	}

	// 1. Primary WordPress Video Player Check
	primaryDataIdPat := regexp.MustCompile(`class=['"][^'"]*(?:mb-screen|videoWrapper)[^'"]*['"][^>]*data-id=['"](\d+)['"]|data-id=['"](\d+)['"][^>]*class=['"][^'"]*(?:mb-screen|videoWrapper)`)
	var primaryPostID string
	if m := primaryDataIdPat.FindStringSubmatch(htmlContent); len(m) > 1 {
		if m[1] != "" {
			primaryPostID = m[1]
		} else if len(m) > 2 && m[2] != "" {
			primaryPostID = m[2]
		}
	}
	if primaryPostID == "" {
		singleDataId := regexp.MustCompile(`data-id=['"](\d+)['"]`)
		if m := singleDataId.FindStringSubmatch(htmlContent); len(m) > 1 {
			primaryPostID = m[1]
		}
	}

	if primaryPostID != "" && hostPrefix != "" {
		for _, srv := range []string{"1", "2"} {
			ajaxURL := hostPrefix + "/wp-json/sextop1/player/?id=" + primaryPostID + "&server=" + srv
			req, _ := http.NewRequest("GET", ajaxURL, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			if resp, err := httpClient.Do(req); err == nil && resp.StatusCode == 200 {
				var ajaxRes struct {
					Success bool   `json:"success"`
					Data    string `json:"data"`
				}
				if json.NewDecoder(resp.Body).Decode(&ajaxRes) == nil && ajaxRes.Data != "" {
					for _, m := range m3u8Pat.FindAllString(ajaxRes.Data, -1) {
						addResult(m)
					}
					for _, sm := range sourcePat.FindAllStringSubmatch(ajaxRes.Data, -1) {
						if len(sm) > 1 {
							addResult(sm[1])
						}
					}
				}
				resp.Body.Close()
			}
			if len(results) > 0 {
				return results
			}
		}
	}

	// 2. JSON-LD Schema VideoObject Extractor
	jsonLdRe := regexp.MustCompile(`<script[^>]*type=['"]application/ld\+json['"][^>]*>([\s\S]*?)</script>`)
	for _, ldMatch := range jsonLdRe.FindAllStringSubmatch(htmlContent, -1) {
		if len(ldMatch) > 1 {
			rawLd := ldMatch[1]
			var vo struct {
				Type       string `json:"@type"`
				ContentURL string `json:"contentUrl"`
			}
			if json.Unmarshal([]byte(rawLd), &vo) == nil && vo.ContentURL != "" && vo.Type == "VideoObject" {
				addResult(vo.ContentURL)
				return results
			}
		}
	}

	// 3. WordPress Base64 Player Endpoint (?get_player=1)
	if baseURL != "" {
		ajaxPlayerURL := baseURL + "?get_player=1"
		if req, err := http.NewRequest("GET", ajaxPlayerURL, nil); err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			if resp, err := httpClient.Do(req); err == nil && resp.StatusCode == 200 {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				trimmed := strings.TrimSpace(string(bodyBytes))
				if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil {
					var playerData struct {
						Sources []struct {
							File string `json:"file"`
						} `json:"sources"`
					}
					if json.Unmarshal(decoded, &playerData) == nil {
						for _, src := range playerData.Sources {
							if src.File != "" {
								addResult(src.File)
							}
						}
					}
				}
			}
		}
		if len(results) > 0 {
			return results
		}
	}

	// 4. Direct Regex for embedded m3u8 in main player script
	for _, m := range m3u8Pat.FindAllString(htmlContent, -1) {
		addResult(m)
	}

	for _, match := range sourcePat.FindAllStringSubmatch(htmlContent, -1) {
		if len(match) > 1 {
			addResult(match[1])
		}
	}

	// 5. Video platform regex if still no direct stream
	if len(results) == 0 {
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`https?://[^\s"'<>]+\.(?:mp4|mpd|webm|m4a|mp3)(?:\?[^\s"'<>]*)?`),
			regexp.MustCompile(`https?://(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/|tiktok\.com/@[^/]+/video/|instagram\.com/(?:p|reel)/|twitter\.com/[^/]+/status/|x\.com/[^/]+/status/|reddit\.com/r/[^\s"'<>]+|vimeo\.com/\d+|soundcloud\.com/[^\s"'<>]+)[^\s"'<>]*`),
		}
		for _, pat := range patterns {
			for _, m := range pat.FindAllString(htmlContent, -1) {
				addResult(m)
			}
		}
	}

	return results
}
