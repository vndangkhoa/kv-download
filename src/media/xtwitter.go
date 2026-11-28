package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	xTwitterReferer   = "https://x.com/"
	xTwitterUserAgent = "KV-Download/2.0"
)

var (
	fxTwitterAPIBase = "https://api.fxtwitter.com"
	vxTwitterAPIBase = "https://api.vxtwitter.com"

	xStatusURLRe   = regexp.MustCompile(`(?i)^https?://(?:(?:www|mobile|m)\.)?(?:x\.com|twitter\.com|fxtwitter\.com|vxtwitter\.com|fixupx\.com|fixvx\.com)/(?:i/web|([^/?#]+))/status(?:es)?/(\d+)(?:/video/(\d+))?(?:[/?#]|$)`)
	xVariantDimRe  = regexp.MustCompile(`/(\d{2,4})x(\d{2,4})/`)
	xTcoStripRe    = regexp.MustCompile(`https?://t\.co/\S+`)
	xPicStripRe    = regexp.MustCompile(`(?i)pic\.(?:twitter|x)\.com/\S+`)
	xMultiSpaceRe  = regexp.MustCompile(`\s+`)
)

// XVideoVariant represents one playable/downloadable video variant for an X / Twitter status video.
type XVideoVariant struct {
	Label       string `json:"label"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Bitrate     int64  `json:"bitrate"`
	IsHLS       bool   `json:"isHls"`
}

// XVideoItem represents a single video or animated GIF attached to an X / Twitter post.
type XVideoItem struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	URL       string          `json:"url"`
	Thumbnail string          `json:"thumbnail,omitempty"`
	Duration  float64         `json:"duration,omitempty"`
	Width     int             `json:"width,omitempty"`
	Height    int             `json:"height,omitempty"`
	IsGif     bool            `json:"isGif,omitempty"`
	Variants  []XVideoVariant `json:"variants"`
}

// XResolvedTweet holds resolved video streams and metadata for an X / Twitter status URL.
type XResolvedTweet struct {
	TweetID      string       `json:"tweetId"`
	CanonicalURL string       `json:"canonicalUrl"`
	Title        string       `json:"title"`
	Text         string       `json:"text"`
	AuthorName   string       `json:"authorName,omitempty"`
	ScreenName   string       `json:"screenName,omitempty"`
	Channel      string       `json:"channel"`
	Thumbnail    string       `json:"thumbnail,omitempty"`
	Videos       []XVideoItem `json:"videos"`
	Referer      string       `json:"referer"`
}

// ParseXStatusURL extracts the handle, tweet ID, and optional 1-based video index from an X / Twitter status URL.
func ParseXStatusURL(rawURL string) (user string, tweetID string, videoIndex int, ok bool) {
	u := strings.TrimSpace(rawURL)
	m := xStatusURLRe.FindStringSubmatch(u)
	if len(m) < 3 {
		return "", "", 0, false
	}
	user = strings.TrimSpace(m[1])
	tweetID = strings.TrimSpace(m[2])
	if tweetID == "" {
		return "", "", 0, false
	}
	if user == "" || strings.EqualFold(user, "i") || strings.EqualFold(user, "web") {
		user = "i"
	}
	if len(m) >= 4 && strings.TrimSpace(m[3]) != "" {
		if idx, err := strconv.Atoi(strings.TrimSpace(m[3])); err == nil && idx > 0 {
			videoIndex = idx
		}
	}
	return user, tweetID, videoIndex, true
}

// IsXStatusURL reports whether rawURL points to an X / Twitter status post.
func IsXStatusURL(rawURL string) bool {
	_, _, _, ok := ParseXStatusURL(rawURL)
	return ok
}

// ResolveXVideo resolves direct MP4/HLS video streams and metadata for an X / Twitter status URL
// using FxTwitter as primary and VxTwitter as fallback.
func ResolveXVideo(ctx context.Context, rawURL string) (*XResolvedTweet, error) {
	user, tweetID, _, ok := ParseXStatusURL(rawURL)
	if !ok {
		return nil, errors.New("not a recognized X / Twitter status URL")
	}

	client := NewResilientHTTPClient(15 * time.Second)

	res, fxErr := fetchFxTwitter(ctx, client, user, tweetID)
	if fxErr == nil && res != nil && len(res.Videos) > 0 {
		return res, nil
	}

	vxRes, vxErr := fetchVxTwitter(ctx, client, user, tweetID)
	if vxErr == nil && vxRes != nil && len(vxRes.Videos) > 0 {
		return vxRes, nil
	}

	if fxErr != nil && vxErr != nil {
		return nil, fmt.Errorf("x resolve failed (fxtwitter: %v; vxtwitter: %v)", fxErr, vxErr)
	}
	if fxErr != nil {
		return nil, fmt.Errorf("x resolve failed: %w", fxErr)
	}
	if vxErr != nil {
		return nil, fmt.Errorf("x resolve failed: %w", vxErr)
	}
	return nil, errors.New("no video found in X / Twitter post")
}

type fxTwitterFormat struct {
	URL       string `json:"url"`
	Bitrate   int64  `json:"bitrate"`
	Container string `json:"container"`
	Codec     string `json:"codec"`
}

type fxTwitterVariant struct {
	URL         string `json:"url"`
	Bitrate     int64  `json:"bitrate"`
	ContentType string `json:"content_type"`
}

type fxTwitterMediaItem struct {
	ID           string             `json:"id"`
	URL          string             `json:"url"`
	ThumbnailURL string             `json:"thumbnail_url"`
	Duration     float64            `json:"duration"`
	Width        int                `json:"width"`
	Height       int                `json:"height"`
	Format       string             `json:"format"`
	Type         string             `json:"type"`
	Formats      []fxTwitterFormat  `json:"formats"`
	Variants     []fxTwitterVariant `json:"variants"`
}

type fxTwitterMediaContainer struct {
	All    []fxTwitterMediaItem `json:"all"`
	Videos []fxTwitterMediaItem `json:"videos"`
}

type fxTwitterTweet struct {
	URL    string `json:"url"`
	ID     string `json:"id"`
	Text   string `json:"text"`
	Author struct {
		Name       string `json:"name"`
		ScreenName string `json:"screen_name"`
	} `json:"author"`
	Media *fxTwitterMediaContainer `json:"media"`
	Quote *fxTwitterTweet          `json:"quote"`
}

type fxTwitterResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Tweet   *fxTwitterTweet `json:"tweet"`
}

func fetchFxTwitter(ctx context.Context, client *http.Client, user, tweetID string) (*XResolvedTweet, error) {
	endpoint := fmt.Sprintf("%s/%s/status/%s", strings.TrimRight(fxTwitterAPIBase, "/"), url.PathEscape(user), url.PathEscape(tweetID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", xTwitterUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fxtwitter HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var payload fxTwitterResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed parsing fxtwitter JSON: %w", err)
	}
	if payload.Tweet == nil {
		return nil, fmt.Errorf("fxtwitter returned empty tweet (%s)", payload.Message)
	}

	t := payload.Tweet
	rawVideos := collectFxVideos(t.Media)
	if len(rawVideos) == 0 && t.Quote != nil {
		rawVideos = collectFxVideos(t.Quote.Media)
	}
	if len(rawVideos) == 0 {
		return nil, errors.New("fxtwitter: tweet contains no video")
	}

	screenName := strings.TrimSpace(t.Author.ScreenName)
	authorName := strings.TrimSpace(t.Author.Name)
	channel := buildXChannel(screenName, authorName)
	baseTitle := buildXTweetTitle(t.Text, screenName, tweetID)
	canonical := strings.TrimSpace(t.URL)
	if canonical == "" {
		canonical = fmt.Sprintf("https://x.com/%s/status/%s", user, tweetID)
	}

	videos := make([]XVideoItem, 0, len(rawVideos))
	firstThumb := ""
	for i, rv := range rawVideos {
		variants := buildFxVariants(rv)
		if len(variants) == 0 {
			continue
		}
		best := SelectXVariant(variants, "highest")
		itemTitle := baseTitle
		if len(rawVideos) > 1 {
			itemTitle = fmt.Sprintf("%s (%d/%d)", baseTitle, i+1, len(rawVideos))
		}
		thumb := strings.TrimSpace(rv.ThumbnailURL)
		if firstThumb == "" && thumb != "" {
			firstThumb = thumb
		}
		vidID := strings.TrimSpace(rv.ID)
		if vidID == "" {
			vidID = fmt.Sprintf("%s_%d", tweetID, i+1)
		}
		videos = append(videos, XVideoItem{
			ID:        vidID,
			Title:     itemTitle,
			URL:       best.URL,
			Thumbnail: thumb,
			Duration:  rv.Duration,
			Width:     rv.Width,
			Height:    rv.Height,
			IsGif:     strings.EqualFold(rv.Type, "gif"),
			Variants:  variants,
		})
	}
	if len(videos) == 0 {
		return nil, errors.New("fxtwitter: no playable variants found")
	}

	return &XResolvedTweet{
		TweetID:      tweetID,
		CanonicalURL: canonical,
		Title:        baseTitle,
		Text:         strings.TrimSpace(t.Text),
		AuthorName:   authorName,
		ScreenName:   screenName,
		Channel:      channel,
		Thumbnail:    firstThumb,
		Videos:       videos,
		Referer:      xTwitterReferer,
	}, nil
}

func collectFxVideos(m *fxTwitterMediaContainer) []fxTwitterMediaItem {
	if m == nil {
		return nil
	}
	if len(m.Videos) > 0 {
		return m.Videos
	}
	var out []fxTwitterMediaItem
	for _, item := range m.All {
		t := strings.ToLower(strings.TrimSpace(item.Type))
		if t == "video" || t == "gif" {
			out = append(out, item)
		}
	}
	return out
}

func buildFxVariants(rv fxTwitterMediaItem) []XVideoVariant {
	seen := make(map[string]bool)
	variants := make([]XVideoVariant, 0, len(rv.Variants)+len(rv.Formats)+1)

	add := func(rawURL, contentType string, bitrate int64) {
		u := strings.TrimSpace(rawURL)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		ct := strings.TrimSpace(contentType)
		isHLS := isXVariantHLS(u, ct)
		if ct == "" {
			if isHLS {
				ct = "application/x-mpegURL"
			} else {
				ct = "video/mp4"
			}
		}
		w, h := parseXVariantDims(u, rv.Width, rv.Height, isHLS)
		variants = append(variants, XVideoVariant{
			Label:       formatXVariantLabel(w, h, bitrate, isHLS),
			URL:         u,
			ContentType: ct,
			Width:       w,
			Height:      h,
			Bitrate:     bitrate,
			IsHLS:       isHLS,
		})
	}

	for _, v := range rv.Variants {
		add(v.URL, v.ContentType, v.Bitrate)
	}
	for _, f := range rv.Formats {
		ct := ""
		if strings.EqualFold(f.Container, "m3u8") {
			ct = "application/x-mpegURL"
		} else if strings.EqualFold(f.Container, "mp4") {
			ct = "video/mp4"
		}
		add(f.URL, ct, f.Bitrate)
	}
	if rv.URL != "" {
		add(rv.URL, rv.Format, 0)
	}

	sortXVariants(variants)
	return variants
}

type vxTwitterMediaItem struct {
	DurationMillis int64  `json:"duration_millis"`
	IDStr          string `json:"id_str"`
	Size           struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"size"`
	ThumbnailURL string `json:"thumbnail_url"`
	Type         string `json:"type"`
	URL          string `json:"url"`
}

type vxTwitterResponse struct {
	TweetID        string               `json:"tweetID"`
	TweetURL       string               `json:"tweetURL"`
	Text           string               `json:"text"`
	UserName       string               `json:"user_name"`
	UserScreenName string               `json:"user_screen_name"`
	MediaURLs      []string             `json:"mediaURLs"`
	MediaExtended  []vxTwitterMediaItem `json:"media_extended"`
	Qrt            *vxTwitterResponse   `json:"qrt"`
}

func fetchVxTwitter(ctx context.Context, client *http.Client, user, tweetID string) (*XResolvedTweet, error) {
	endpoint := fmt.Sprintf("%s/%s/status/%s", strings.TrimRight(vxTwitterAPIBase, "/"), url.PathEscape(user), url.PathEscape(tweetID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", xTwitterUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("vxtwitter HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var payload vxTwitterResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed parsing vxtwitter JSON: %w", err)
	}

	rawItems := collectVxVideos(payload.MediaExtended)
	if len(rawItems) == 0 && payload.Qrt != nil {
		rawItems = collectVxVideos(payload.Qrt.MediaExtended)
	}
	if len(rawItems) == 0 {
		return nil, errors.New("vxtwitter: tweet contains no video")
	}

	screenName := strings.TrimSpace(payload.UserScreenName)
	authorName := strings.TrimSpace(payload.UserName)
	channel := buildXChannel(screenName, authorName)
	baseTitle := buildXTweetTitle(payload.Text, screenName, tweetID)
	canonical := fmt.Sprintf("https://x.com/%s/status/%s", user, tweetID)
	if screenName != "" {
		canonical = fmt.Sprintf("https://x.com/%s/status/%s", screenName, tweetID)
	}

	videos := make([]XVideoItem, 0, len(rawItems))
	firstThumb := ""
	for i, item := range rawItems {
		u := strings.TrimSpace(item.URL)
		if u == "" {
			continue
		}
		isHLS := isXVariantHLS(u, "")
		ct := "video/mp4"
		if isHLS {
			ct = "application/x-mpegURL"
		}
		w, h := parseXVariantDims(u, item.Size.Width, item.Size.Height, isHLS)
		variant := XVideoVariant{
			Label:       formatXVariantLabel(w, h, 0, isHLS),
			URL:         u,
			ContentType: ct,
			Width:       w,
			Height:      h,
			IsHLS:       isHLS,
		}
		itemTitle := baseTitle
		if len(rawItems) > 1 {
			itemTitle = fmt.Sprintf("%s (%d/%d)", baseTitle, i+1, len(rawItems))
		}
		thumb := strings.TrimSpace(item.ThumbnailURL)
		if firstThumb == "" && thumb != "" {
			firstThumb = thumb
		}
		vidID := strings.TrimSpace(item.IDStr)
		if vidID == "" {
			vidID = fmt.Sprintf("%s_%d", tweetID, i+1)
		}
		videos = append(videos, XVideoItem{
			ID:        vidID,
			Title:     itemTitle,
			URL:       u,
			Thumbnail: thumb,
			Duration:  float64(item.DurationMillis) / 1000.0,
			Width:     item.Size.Width,
			Height:    item.Size.Height,
			IsGif:     strings.EqualFold(item.Type, "gif"),
			Variants:  []XVideoVariant{variant},
		})
	}
	if len(videos) == 0 {
		return nil, errors.New("vxtwitter: no playable video URLs found")
	}

	return &XResolvedTweet{
		TweetID:      tweetID,
		CanonicalURL: canonical,
		Title:        baseTitle,
		Text:         strings.TrimSpace(payload.Text),
		AuthorName:   authorName,
		ScreenName:   screenName,
		Channel:      channel,
		Thumbnail:    firstThumb,
		Videos:       videos,
		Referer:      xTwitterReferer,
	}, nil
}

func collectVxVideos(items []vxTwitterMediaItem) []vxTwitterMediaItem {
	var out []vxTwitterMediaItem
	for _, it := range items {
		t := strings.ToLower(strings.TrimSpace(it.Type))
		if (t == "video" || t == "gif") && strings.TrimSpace(it.URL) != "" {
			out = append(out, it)
		}
	}
	return out
}

func isXVariantHLS(rawURL, contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "mpegurl") {
		return true
	}
	clean := strings.ToLower(strings.Split(strings.Split(rawURL, "?")[0], "#")[0])
	return strings.HasSuffix(clean, ".m3u8")
}

func parseXVariantDims(rawURL string, fallbackW, fallbackH int, isHLS bool) (int, int) {
	if m := xVariantDimRe.FindStringSubmatch(rawURL); len(m) == 3 {
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w > 0 && h > 0 {
			return w, h
		}
	}
	if !isHLS && fallbackW > 0 && fallbackH > 0 {
		return fallbackW, fallbackH
	}
	return 0, 0
}

func effectiveXQualityDim(w, h int) int {
	if w > 0 && h > 0 {
		if w < h {
			return w
		}
		return h
	}
	if h > 0 {
		return h
	}
	return w
}

func formatXVariantLabel(w, h int, bitrate int64, isHLS bool) string {
	dim := effectiveXQualityDim(w, h)
	if dim > 0 {
		return fmt.Sprintf("%dp", dim)
	}
	if bitrate > 0 {
		return fmt.Sprintf("%dk", bitrate/1000)
	}
	if isHLS {
		return "HLS"
	}
	return "MP4"
}

func sortXVariants(variants []XVideoVariant) {
	sort.SliceStable(variants, func(i, j int) bool {
		if variants[i].IsHLS != variants[j].IsHLS {
			return !variants[i].IsHLS
		}
		if variants[i].Bitrate != variants[j].Bitrate {
			return variants[i].Bitrate > variants[j].Bitrate
		}
		areaI := variants[i].Width * variants[i].Height
		areaJ := variants[j].Width * variants[j].Height
		return areaI > areaJ
	})
}

func buildXChannel(screenName, authorName string) string {
	if screenName != "" {
		return "@" + strings.TrimPrefix(screenName, "@")
	}
	if authorName != "" {
		return authorName
	}
	return "X / Twitter"
}

func buildXTweetTitle(text, screenName, tweetID string) string {
	cleaned := xTcoStripRe.ReplaceAllString(text, " ")
	cleaned = xPicStripRe.ReplaceAllString(cleaned, " ")
	cleaned = xMultiSpaceRe.ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned != "" {
		runes := []rune(cleaned)
		if len(runes) > 85 {
			cleaned = strings.TrimSpace(string(runes[:82])) + "..."
		}
		return cleaned
	}
	if screenName != "" {
		return fmt.Sprintf("@%s — X Video %s", strings.TrimPrefix(screenName, "@"), tweetID)
	}
	return "X Video " + tweetID
}

// SelectXVideo chooses the 1-based videoIndex from a resolved tweet (defaulting to the first video).
func SelectXVideo(tweet *XResolvedTweet, videoIndex int) (XVideoItem, bool) {
	if tweet == nil || len(tweet.Videos) == 0 {
		return XVideoItem{}, false
	}
	if videoIndex >= 1 && videoIndex <= len(tweet.Videos) {
		return tweet.Videos[videoIndex-1], true
	}
	return tweet.Videos[0], true
}

// SelectXVariant selects the best matching variant for formatPref ("highest", "1080p", "720p", "480p", "360p").
// Direct MP4 variants are always preferred over HLS playlists when available.
func SelectXVariant(variants []XVideoVariant, formatPref string) XVideoVariant {
	if len(variants) == 0 {
		return XVideoVariant{}
	}
	sorted := make([]XVideoVariant, len(variants))
	copy(sorted, variants)
	sortXVariants(sorted)

	mp4s := make([]XVideoVariant, 0, len(sorted))
	for _, v := range sorted {
		if !v.IsHLS {
			mp4s = append(mp4s, v)
		}
	}
	pool := mp4s
	if len(pool) == 0 {
		pool = sorted
	}

	f := strings.ToLower(strings.TrimSpace(formatPref))
	maxDim := 0
	switch f {
	case "1080", "1080p", "fhd", "high", "optimal":
		maxDim = 1080
	case "720", "720p", "hd", "normal":
		maxDim = 720
	case "480", "480p", "sd", "low":
		maxDim = 480
	case "360", "360p", "saver":
		maxDim = 360
	}
	if maxDim > 0 {
		for _, v := range pool {
			dim := effectiveXQualityDim(v.Width, v.Height)
			if dim > 0 && dim <= maxDim {
				return v
			}
		}
		return pool[len(pool)-1]
	}
	return pool[0]
}

// downloadThumbnailToFile fetches a thumbnail image to destPath (best-effort, 8s timeout).
func downloadThumbnailToFile(ctx context.Context, thumbURL, destPath string) error {
	u := strings.TrimSpace(thumbURL)
	if u == "" {
		return errors.New("empty thumbnail URL")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", av01UserAgent)
	req.Header.Set("Referer", xTwitterReferer)

	client := NewResilientHTTPClient(8 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("thumbnail HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil || len(data) == 0 {
		return errors.New("empty thumbnail body")
	}
	return os.WriteFile(destPath, data, 0o644)
}

// extractAudioFromFileWithFFmpeg extracts an audio file (mp3, m4a, flac, wav) from a local MP4 using ffmpeg.
func extractAudioFromFileWithFFmpeg(ctx context.Context, srcPath, targetDir, stem, format string) (string, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("ffmpeg not found: %w", err)
	}
	f := strings.ToLower(strings.TrimSpace(format))
	ext := ".mp3"
	var codecArgs []string
	switch f {
	case "audio_m4a", "m4a", "aac":
		ext = ".m4a"
		codecArgs = []string{"-c:a", "aac", "-b:a", "192k"}
	case "audio_flac", "flac":
		ext = ".flac"
		codecArgs = []string{"-c:a", "flac"}
	case "audio_wav", "wav":
		ext = ".wav"
		codecArgs = []string{"-c:a", "pcm_s16le"}
	default:
		ext = ".mp3"
		codecArgs = []string{"-c:a", "libmp3lame", "-q:a", "2"}
	}

	outPath := filepath.Join(targetDir, stem+ext)
	args := []string{"-y", "-loglevel", "warning", "-i", srcPath, "-vn"}
	args = append(args, codecArgs...)
	args = append(args, outPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return "", fmt.Errorf("ffmpeg audio extract failed: %s", errText)
	}
	return outPath, nil
}

func buildXFormatChoices(variants []XVideoVariant) []FormatChoice {
	seenLabel := make(map[string]bool)
	choices := make([]FormatChoice, 0, len(variants)+1)
	for _, v := range variants {
		if seenLabel[v.Label] {
			continue
		}
		seenLabel[v.Label] = true
		kindLabel := "MP4"
		if v.IsHLS {
			kindLabel = "M3U8 HLS"
		}
		dim := effectiveXQualityDim(v.Width, v.Height)
		fmtID := "highest"
		if dim > 0 {
			fmtID = strconv.Itoa(dim)
		}
		choices = append(choices, FormatChoice{
			Label:      fmt.Sprintf("%s · %s", v.Label, kindLabel),
			Resolution: v.Label,
			Height:     dim,
			FormatID:   fmtID,
			Kind:       "video",
		})
	}
	choices = append(choices, FormatChoice{
		Label:      "Audio Only · MP3",
		Resolution: "Audio",
		Height:     0,
		FormatID:   "audio_mp3",
		Kind:       "audio",
		ACodec:     "mp3",
	})
	return choices
}
