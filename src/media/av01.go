package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	av01Referer   = "https://av01.media/"
	av01Origin    = "https://av01.media"
	av01GeoURL    = "https://files.iw01.xyz/edge/geo.js?json"
	av01APIBase   = "https://customers.iw01.xyz"
	av01UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

var (
	av01VideoURLRe    = regexp.MustCompile(`(?i)^https?://(?:www\.)?av01\.(?:media|tv)/(?:[a-z]{2}(?:-[a-z]{2})?/)?video/(\d+)(?:/([^/?#]+))?`)
	av01ManifestURLRe = regexp.MustCompile(`(?i)^https?://(?:[a-z0-9-]+\.)?iw01\.xyz/api/v1/videos/(\d+)/manifest/`)
	av01ResolutionRe  = regexp.MustCompile(`(?i)RESOLUTION=(\d+)x(\d+)`)
	av01BandwidthRe   = regexp.MustCompile(`(?i)BANDWIDTH=(\d+)`)
)

// AV01Variant represents one quality stream in an AV01 master HLS manifest.
type AV01Variant struct {
	Label     string `json:"label"`
	URL       string `json:"url"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Bandwidth int64  `json:"bandwidth"`
}

// AV01ResolvedVideo holds resolved unblocked HLS streams and metadata for an av01.media video.
type AV01ResolvedVideo struct {
	VideoID   string        `json:"videoId"`
	Slug      string        `json:"slug,omitempty"`
	Code      string        `json:"code,omitempty"`
	Title     string        `json:"title"`
	Thumbnail string        `json:"thumbnail,omitempty"`
	MasterURL string        `json:"masterUrl"`
	Variants  []AV01Variant `json:"variants"`
	Referer   string        `json:"referer"`
}

// ParseAV01VideoURL extracts the numeric video ID and optional URL slug from an av01.media / av01.tv / iw01.xyz video URL.
func ParseAV01VideoURL(rawURL string) (videoID string, slug string, ok bool) {
	u := strings.TrimSpace(rawURL)
	if m := av01VideoURLRe.FindStringSubmatch(u); len(m) >= 2 {
		id := strings.TrimSpace(m[1])
		s := ""
		if len(m) >= 3 {
			s = strings.TrimSpace(m[2])
		}
		if id != "" {
			return id, s, true
		}
	}
	if m := av01ManifestURLRe.FindStringSubmatch(u); len(m) >= 2 {
		id := strings.TrimSpace(m[1])
		if id != "" {
			return id, "", true
		}
	}
	return "", "", false
}

// ResolveAV01Video resolves unblocked HLS m3u8 streams for an av01.media URL via files.iw01.xyz & customers.iw01.xyz.
func ResolveAV01Video(ctx context.Context, rawURL string) (*AV01ResolvedVideo, error) {
	videoID, slug, ok := ParseAV01VideoURL(rawURL)
	if !ok {
		return nil, errors.New("not a recognized av01.media video URL")
	}

	client := NewResilientHTTPClient(15 * time.Second)

	// Step 1: Fetch IP-bound geo token from files.iw01.xyz/edge/geo.js?json
	geoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, av01GeoURL, nil)
	if err != nil {
		return nil, err
	}
	setAV01Headers(geoReq)
	geoResp, err := client.Do(geoReq)
	if err != nil {
		return nil, fmt.Errorf("av01 geo token request failed: %w", err)
	}
	defer geoResp.Body.Close()
	if geoResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("av01 geo token HTTP %d", geoResp.StatusCode)
	}
	var geo struct {
		TokenV2 string          `json:"token_v2"`
		Expires json.RawMessage `json:"expires"`
		IP      string          `json:"ip"`
		Comp    bool            `json:"comp"`
	}
	if err := json.NewDecoder(io.LimitReader(geoResp.Body, 256*1024)).Decode(&geo); err != nil {
		return nil, fmt.Errorf("failed parsing av01 geo token JSON: %w", err)
	}
	expiresStr := strings.Trim(strings.TrimSpace(string(geo.Expires)), `"`)
	if geo.TokenV2 == "" || expiresStr == "" || expiresStr == "null" {
		return nil, errors.New("incomplete av01 geo token payload")
	}

	// Step 2: Exchange geo token for video-scoped access_token JWT on customers.iw01.xyz
	q := url.Values{}
	q.Set("token_v2", geo.TokenV2)
	q.Set("expires", expiresStr)
	if geo.IP != "" {
		q.Set("ip", geo.IP)
	}
	if geo.Comp {
		q.Set("comp", "true")
	}
	cdnAccessURL := fmt.Sprintf("%s/api/v1/videos/%s/cdn-access?%s", av01APIBase, videoID, q.Encode())
	accessReq, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnAccessURL, nil)
	if err != nil {
		return nil, err
	}
	setAV01Headers(accessReq)
	accessResp, err := client.Do(accessReq)
	if err != nil {
		return nil, fmt.Errorf("av01 cdn-access request failed: %w", err)
	}
	defer accessResp.Body.Close()
	if accessResp.StatusCode != http.StatusOK {
		bodySnippet, _ := io.ReadAll(io.LimitReader(accessResp.Body, 512))
		return nil, fmt.Errorf("av01 cdn-access HTTP %d: %s", accessResp.StatusCode, strings.TrimSpace(string(bodySnippet)))
	}
	var cdnAccess struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(accessResp.Body, 256*1024)).Decode(&cdnAccess); err != nil {
		return nil, fmt.Errorf("failed parsing av01 cdn-access JSON: %w", err)
	}
	if strings.TrimSpace(cdnAccess.AccessToken) == "" {
		return nil, errors.New("empty access_token from av01 cdn-access")
	}

	code := extractAV01CodeFromJWT(cdnAccess.AccessToken)
	title := deriveAV01Title(videoID, slug, code)
	thumb := ""

	// Step 3: Best-effort localized metadata lookup on av01.media (skip quickly if ISP-blocked)
	metaCtx, metaCancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer metaCancel()
	if metaReq, mErr := http.NewRequestWithContext(metaCtx, http.MethodGet, fmt.Sprintf("https://av01.media/api/v1/videos/%s", videoID), nil); mErr == nil {
		setAV01Headers(metaReq)
		if metaResp, rErr := client.Do(metaReq); rErr == nil {
			defer metaResp.Body.Close()
			if metaResp.StatusCode == http.StatusOK {
				var meta struct {
					Title     string `json:"title"`
					Name      string `json:"name"`
					Code      string `json:"code"`
					Thumbnail string `json:"thumbnail"`
					Cover     string `json:"cover"`
					Poster    string `json:"poster"`
				}
				if json.NewDecoder(io.LimitReader(metaResp.Body, 512*1024)).Decode(&meta) == nil {
					if t := strings.TrimSpace(meta.Title); t != "" {
						title = t
					} else if n := strings.TrimSpace(meta.Name); n != "" {
						title = n
					}
					if c := strings.TrimSpace(meta.Code); c != "" {
						code = c
					}
					for _, img := range []string{meta.Cover, meta.Poster, meta.Thumbnail} {
						if strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
							thumb = img
							break
						}
					}
				}
			}
		}
	}

	// Step 4: Fetch master.m3u8 manifest
	manifestBaseDir := fmt.Sprintf("%s/api/v1/videos/%s/manifest/", av01APIBase, videoID)
	masterURL := manifestBaseDir + "master.m3u8?access_token=" + url.QueryEscape(cdnAccess.AccessToken)
	masterReq, err := http.NewRequestWithContext(ctx, http.MethodGet, masterURL, nil)
	if err != nil {
		return nil, err
	}
	setAV01Headers(masterReq)
	masterResp, err := client.Do(masterReq)
	if err != nil {
		return nil, fmt.Errorf("av01 master.m3u8 request failed: %w", err)
	}
	defer masterResp.Body.Close()
	if masterResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("av01 master.m3u8 HTTP %d", masterResp.StatusCode)
	}
	manifestBytes, err := io.ReadAll(io.LimitReader(masterResp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	manifestStr := string(manifestBytes)
	if !strings.Contains(manifestStr, "#EXTM3U") {
		return nil, errors.New("av01 master manifest did not return valid #EXTM3U")
	}

	variants := parseAV01MasterM3U8(manifestStr, manifestBaseDir, cdnAccess.AccessToken, masterURL)
	if len(variants) == 0 {
		return nil, errors.New("no playable variants found in av01 master manifest")
	}

	return &AV01ResolvedVideo{
		VideoID:   videoID,
		Slug:      slug,
		Code:      code,
		Title:     title,
		Thumbnail: thumb,
		MasterURL: masterURL,
		Variants:  variants,
		Referer:   av01Referer,
	}, nil
}

func setAV01Headers(req *http.Request) {
	req.Header.Set("Origin", av01Origin)
	req.Header.Set("Referer", av01Referer)
	req.Header.Set("User-Agent", av01UserAgent)
	req.Header.Set("Accept", "*/*")
}

func deriveAV01Title(videoID, slug, code string) string {
	cleanSlug := strings.TrimSpace(slug)
	if cleanSlug != "" {
		cleanSlug = strings.ReplaceAll(cleanSlug, "-", " ")
		cleanSlug = strings.ReplaceAll(cleanSlug, "_", " ")
		if code != "" && !strings.Contains(strings.ToLower(cleanSlug), strings.ToLower(strings.ReplaceAll(code, "-", " "))) {
			return code + " — " + cleanSlug
		}
		if code != "" {
			return strings.ToUpper(code)
		}
		return strings.ToUpper(cleanSlug)
	}
	if code != "" {
		return code
	}
	return "AV01 #" + videoID
}

// extractAV01CodeFromJWT decodes the middle JWT payload claim `"sub"` (e.g. "/fmp4/20261121f/FC2-PPV-5011616/map/")
// and returns the DVD/release code segment if present.
func extractAV01CodeFromJWT(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return ""
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if json.Unmarshal(rawPayload, &claims) != nil || claims.Sub == "" {
		return ""
	}
	segs := strings.FieldsFunc(claims.Sub, func(r rune) bool { return r == '/' })
	// Typical format: fmp4 / 20261121f / FC2-PPV-5011616 / map
	if len(segs) >= 3 && segs[2] != "" && !strings.EqualFold(segs[2], "map") {
		return segs[2]
	}
	for _, s := range segs {
		if strings.Contains(s, "-") && len(s) >= 4 {
			return s
		}
	}
	return ""
}

func parseAV01MasterM3U8(content, manifestBaseDir, accessToken, masterURL string) []AV01Variant {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	variants := make([]AV01Variant, 0, 4)

	pendingWidth := 0
	pendingHeight := 0
	var pendingBandwidth int64
	hasStreamInf := false

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			hasStreamInf = true
			pendingWidth = 0
			pendingHeight = 0
			pendingBandwidth = 0
			if rm := av01ResolutionRe.FindStringSubmatch(line); len(rm) == 3 {
				pendingWidth, _ = strconv.Atoi(rm[1])
				pendingHeight, _ = strconv.Atoi(rm[2])
			}
			if bm := av01BandwidthRe.FindStringSubmatch(line); len(bm) == 2 {
				pendingBandwidth, _ = strconv.ParseInt(bm[1], 10, 64)
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if hasStreamInf {
			variantURL := resolveAV01VariantURL(line, manifestBaseDir, accessToken)
			label := "Auto"
			if pendingHeight > 0 {
				label = fmt.Sprintf("%dp", pendingHeight)
			} else if pendingBandwidth > 0 {
				label = fmt.Sprintf("%dk", pendingBandwidth/1000)
			}
			variants = append(variants, AV01Variant{
				Label:     label,
				URL:       variantURL,
				Width:     pendingWidth,
				Height:    pendingHeight,
				Bandwidth: pendingBandwidth,
			})
			hasStreamInf = false
		}
	}

	if len(variants) == 0 && strings.Contains(content, "#EXTINF:") && masterURL != "" {
		variants = append(variants, AV01Variant{
			Label:     "Auto",
			URL:       masterURL,
			Height:    0,
			Bandwidth: 0,
		})
	}

	sort.SliceStable(variants, func(i, j int) bool {
		if variants[i].Height != variants[j].Height {
			return variants[i].Height > variants[j].Height
		}
		return variants[i].Bandwidth > variants[j].Bandwidth
	})
	return variants
}

func resolveAV01VariantURL(ref, manifestBaseDir, accessToken string) string {
	absURL := ref
	if !strings.HasPrefix(ref, "http://") && !strings.HasPrefix(ref, "https://") {
		if baseParsed, err := url.Parse(manifestBaseDir); err == nil {
			if relParsed, rErr := baseParsed.Parse(ref); rErr == nil {
				absURL = relParsed.String()
			}
		}
	}
	if accessToken != "" && !strings.Contains(absURL, "access_token=") {
		sep := "?"
		if strings.Contains(absURL, "?") {
			sep = "&"
		}
		absURL += sep + "access_token=" + url.QueryEscape(accessToken)
	}
	return absURL
}

// SelectAV01Variant selects the closest matching stream variant for a requested quality ("highest", "1080p", "720p", "480p", "360p").
func SelectAV01Variant(variants []AV01Variant, formatPref string) AV01Variant {
	if len(variants) == 0 {
		return AV01Variant{}
	}
	sorted := make([]AV01Variant, len(variants))
	copy(sorted, variants)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Height != sorted[j].Height {
			return sorted[i].Height > sorted[j].Height
		}
		return sorted[i].Bandwidth > sorted[j].Bandwidth
	})

	f := strings.ToLower(strings.TrimSpace(formatPref))
	maxHeight := 0
	switch f {
	case "1080", "1080p", "fhd", "high", "optimal":
		maxHeight = 1080
	case "720", "720p", "hd", "normal":
		maxHeight = 720
	case "480", "480p", "sd", "low":
		maxHeight = 480
	case "360", "360p", "saver":
		maxHeight = 360
	}
	if maxHeight > 0 {
		for _, v := range sorted {
			if v.Height > 0 && v.Height <= maxHeight {
				return v
			}
		}
		return sorted[len(sorted)-1]
	}
	return sorted[0]
}

func isHLSOrDashURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	noQuery := strings.ToLower(strings.Split(strings.Split(trimmed, "?")[0], "#")[0])
	if strings.HasSuffix(noQuery, ".m3u8") || strings.HasSuffix(noQuery, ".mpd") {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, ".m3u8?") || strings.Contains(lower, ".mpd?")
}

func originFromURL(raw string) string {
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return ""
}

// downloadHLSWithFFmpeg copies/remuxes a direct HLS .m3u8 stream to .mp4 (or .mp3) using ffmpeg as fallback when yt-dlp fails.
func downloadHLSWithFFmpeg(ctx context.Context, streamURL, referer, outFile string, isAudio bool) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	headers := "User-Agent: " + av01UserAgent + "\r\n"
	if referer != "" {
		headers += "Referer: " + referer + "\r\n"
		if org := originFromURL(referer); org != "" {
			headers += "Origin: " + org + "\r\n"
		}
	}

	args := []string{
		"-y",
		"-loglevel", "warning",
		"-headers", headers,
		"-i", streamURL,
	}
	if isAudio {
		ext := strings.ToLower(filepath.Ext(outFile))
		switch ext {
		case ".m4a", ".aac":
			args = append(args, "-vn", "-c:a", "aac", "-b:a", "192k", outFile)
		default:
			args = append(args, "-vn", "-c:a", "libmp3lame", "-q:a", "2", outFile)
		}
	} else {
		args = append(args, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", outFile)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return fmt.Errorf("ffmpeg HLS failed: %s", errText)
	}
	return nil
}
