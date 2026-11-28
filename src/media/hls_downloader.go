package media

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// errHLSFallbackToYtDlp signals that the native Go HLS engine intentionally deferred
// to yt-dlp (e.g., demuxed audio streams, SAMPLE-AES, or live streams).
var errHLSFallbackToYtDlp = errors.New("hls: fallback to yt-dlp")

const (
	hlsInMemorySoftCapBytes = 24 * 1024 * 1024
	hlsReadChunkBytes       = 64 * 1024
	hlsDiskWriteBufferBytes = 4 * 1024 * 1024
)

var (
	hlsAttrRe      = regexp.MustCompile(`([A-Z0-9-]+)=("([^"]*)"|[^,\s]+)`)
	hlsResRe       = regexp.MustCompile(`(?i)RESOLUTION=(\d+)x(\d+)`)
	hlsBandwidthRe = regexp.MustCompile(`(?i)BANDWIDTH=(\d+)`)
)

// HLSProgress reports live throughput and completion progress from native Go downloaders.
type HLSProgress struct {
	Percent    float64
	Speed      string
	ETA        string
	Downloaded int64
	TotalBytes int64
}

// HLSDownloadOptions configures DownloadHLSNative.
type HLSDownloadOptions struct {
	URL          string
	Referer      string
	Cookies      string
	UserAgent    string
	Format       string
	IsAudio      bool
	TargetDir    string
	OutputStem   string
	Workers      int
	RateLimitBps int64
	HTTPClient   *http.Client // Optional override (primarily for tests)
	OnProgress   func(HLSProgress)
}

type hlsByteRange struct {
	HasRange bool
	Length   int64
	Offset   int64
}

type hlsSegment struct {
	Index               int
	URL                 string
	KeyURL              string
	IV                  []byte
	MediaSequenceNumber uint64
	ByteRange           hlsByteRange
}

type hlsMediaPlaylist struct {
	InitMapURL       string
	InitMapByteRange hlsByteRange
	InitKeyURL       string
	InitIV           []byte
	Segments         []hlsSegment
	IsFmp4           bool
	HasEndList       bool
	PlaylistType     string
}

type hlsMasterVariant struct {
	URL       string
	Width     int
	Height    int
	Bandwidth int64
}

func dialResilientContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return dialer.DialContext(ctx, network, addr)
	}
	resolvedIP, err := resolveHostWithFallback(ctx, host)
	if err != nil {
		return nil, err
	}
	return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedIP, port))
}

var highThroughputTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ForceAttemptHTTP2:     false,
	TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	MaxIdleConns:          256,
	MaxIdleConnsPerHost:   64,
	MaxConnsPerHost:       64,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 25 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DisableCompression:    true,
	ReadBufferSize:        256 * 1024,
	WriteBufferSize:       64 * 1024,
	TLSClientConfig: &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // Matches yt-dlp --no-check-certificates behavior.
	},
	DialContext: dialResilientContext,
}

var http2FallbackTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          128,
	MaxIdleConnsPerHost:   32,
	MaxConnsPerHost:       32,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 25 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DisableCompression:    true,
	ReadBufferSize:        256 * 1024,
	WriteBufferSize:       64 * 1024,
	TLSClientConfig: &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // Matches yt-dlp --no-check-certificates behavior.
	},
	DialContext: dialResilientContext,
}

func newHighThroughputHTTPClient() *http.Client {
	return &http.Client{
		Transport: highThroughputTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func newHTTP2FallbackClient() *http.Client {
	return &http.Client{
		Transport: http2FallbackTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

var (
	sharedHighThroughputClient = newHighThroughputHTTPClient()
	sharedHTTP2FallbackClient  = newHTTP2FallbackClient()
)

func isHLSURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	noQuery := strings.ToLower(strings.Split(strings.Split(trimmed, "?")[0], "#")[0])
	if strings.HasSuffix(noQuery, ".m3u8") {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, ".m3u8?")
}

func parseHLSAttributes(attrText string) map[string]string {
	attrs := make(map[string]string)
	for _, m := range hlsAttrRe.FindAllStringSubmatch(attrText, -1) {
		if len(m) < 3 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(m[1]))
		val := m[2]
		if m[3] != "" || (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) {
			val = m[3]
		}
		attrs[key] = val
	}
	return attrs
}

func parseHLSByteRange(spec string, prevEnd int64) (hlsByteRange, int64, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return hlsByteRange{}, prevEnd, errors.New("empty byte range")
	}
	parts := strings.SplitN(spec, "@", 2)
	length, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || length <= 0 {
		return hlsByteRange{}, prevEnd, fmt.Errorf("invalid byte range length %q", spec)
	}
	offset := prevEnd
	if len(parts) == 2 {
		offset, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || offset < 0 {
			return hlsByteRange{}, prevEnd, fmt.Errorf("invalid byte range offset %q", spec)
		}
	}
	return hlsByteRange{HasRange: true, Length: length, Offset: offset}, offset + length, nil
}

func parseHexIVBytes(hexStr string) ([]byte, error) {
	clean := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(hexStr, "0x"), "0X"))
	if clean == "" {
		return nil, errors.New("empty IV")
	}
	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	raw, err := hex.DecodeString(clean)
	if err != nil {
		return nil, err
	}
	if len(raw) > aes.BlockSize {
		return raw[len(raw)-aes.BlockSize:], nil
	}
	if len(raw) < aes.BlockSize {
		padded := make([]byte, aes.BlockSize)
		copy(padded[aes.BlockSize-len(raw):], raw)
		return padded, nil
	}
	return raw, nil
}

func sequenceIVBytes(seq uint64) []byte {
	iv := make([]byte, aes.BlockSize)
	binary.BigEndian.PutUint64(iv[8:], seq)
	return iv
}

func resolveHLSRef(base *url.URL, ref, accessToken string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	absURL := ref
	if base != nil {
		if rel, err := url.Parse(ref); err == nil {
			absURL = base.ResolveReference(rel).String()
		}
	}
	if accessToken != "" {
		if parsed, err := url.Parse(absURL); err == nil {
			q := parsed.Query()
			if !q.Has("access_token") {
				q.Set("access_token", accessToken)
				parsed.RawQuery = q.Encode()
				absURL = parsed.String()
			}
		}
	}
	return absURL
}

func selectHLSMasterVariant(masterText, baseURL, formatPref string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(masterText, "\r\n", "\n"), "\n")
	baseParsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	accessToken := baseParsed.Query().Get("access_token")

	var variants []hlsMasterVariant
	pendingW, pendingH := 0, 0
	var pendingBW int64
	hasStreamInf := false

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-MEDIA:") {
			attrs := parseHLSAttributes(strings.TrimPrefix(line, "#EXT-X-MEDIA:"))
			if strings.EqualFold(attrs["TYPE"], "AUDIO") && strings.TrimSpace(attrs["URI"]) != "" {
				return "", fmt.Errorf("%w: demuxed audio rendition requires yt-dlp", errHLSFallbackToYtDlp)
			}
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			hasStreamInf = true
			pendingW, pendingH, pendingBW = 0, 0, 0
			if rm := hlsResRe.FindStringSubmatch(line); len(rm) == 3 {
				pendingW, _ = strconv.Atoi(rm[1])
				pendingH, _ = strconv.Atoi(rm[2])
			}
			if bm := hlsBandwidthRe.FindStringSubmatch(line); len(bm) == 2 {
				pendingBW, _ = strconv.ParseInt(bm[1], 10, 64)
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if hasStreamInf {
			variants = append(variants, hlsMasterVariant{
				URL:       resolveHLSRef(baseParsed, line, accessToken),
				Width:     pendingW,
				Height:    pendingH,
				Bandwidth: pendingBW,
			})
			hasStreamInf = false
		}
	}

	if len(variants) == 0 {
		return baseURL, nil
	}

	sort.SliceStable(variants, func(i, j int) bool {
		if variants[i].Height != variants[j].Height {
			return variants[i].Height > variants[j].Height
		}
		return variants[i].Bandwidth > variants[j].Bandwidth
	})

	maxHeight := 0
	switch strings.ToLower(strings.TrimSpace(formatPref)) {
	case "4k", "2160", "2160p", "uhd":
		maxHeight = 2160
	case "1440", "1440p", "2k", "qhd":
		maxHeight = 1440
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
		for _, v := range variants {
			if v.Height > 0 && v.Height <= maxHeight {
				return v.URL, nil
			}
		}
		return variants[len(variants)-1].URL, nil
	}
	return variants[0].URL, nil
}

func parseHLSMediaPlaylist(playlistText, baseURL string) (*hlsMediaPlaylist, error) {
	lines := strings.Split(strings.ReplaceAll(playlistText, "\r\n", "\n"), "\n")
	baseParsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	accessToken := baseParsed.Query().Get("access_token")

	pl := &hlsMediaPlaylist{
		Segments: make([]hlsSegment, 0, 128),
	}

	var mediaSeq uint64
	var currentKeyURL string
	var currentIV []byte
	var pendingRange hlsByteRange
	var prevRangeEnd int64
	var prevMapRangeEnd int64

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"))
			if parsedSeq, sErr := strconv.ParseUint(v, 10, 64); sErr == nil {
				mediaSeq = parsedSeq
			}
		case strings.HasPrefix(line, "#EXT-X-PLAYLIST-TYPE:"):
			pl.PlaylistType = strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-PLAYLIST-TYPE:"))
		case line == "#EXT-X-ENDLIST":
			pl.HasEndList = true
		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			attrs := parseHLSAttributes(strings.TrimPrefix(line, "#EXT-X-KEY:"))
			method := strings.ToUpper(strings.TrimSpace(attrs["METHOD"]))
			if method == "" {
				method = "NONE"
			}
			switch method {
			case "NONE":
				currentKeyURL = ""
				currentIV = nil
			case "AES-128":
				keyURI := strings.TrimSpace(attrs["URI"])
				if keyURI == "" {
					return nil, fmt.Errorf("%w: AES-128 key missing URI", errHLSFallbackToYtDlp)
				}
				currentKeyURL = resolveHLSRef(baseParsed, keyURI, accessToken)
				if ivStr := strings.TrimSpace(attrs["IV"]); ivStr != "" {
					ivBytes, ivErr := parseHexIVBytes(ivStr)
					if ivErr != nil {
						return nil, fmt.Errorf("%w: invalid AES-128 IV %q", errHLSFallbackToYtDlp, ivStr)
					}
					currentIV = ivBytes
				} else {
					currentIV = nil
				}
			default:
				return nil, fmt.Errorf("%w: unsupported HLS key method %s", errHLSFallbackToYtDlp, method)
			}
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			if pl.InitMapURL == "" {
				attrs := parseHLSAttributes(strings.TrimPrefix(line, "#EXT-X-MAP:"))
				mapURI := strings.TrimSpace(attrs["URI"])
				if mapURI != "" {
					pl.InitMapURL = resolveHLSRef(baseParsed, mapURI, accessToken)
					pl.InitKeyURL = currentKeyURL
					if len(currentIV) > 0 {
						pl.InitIV = append([]byte(nil), currentIV...)
					}
					if brSpec := strings.TrimSpace(attrs["BYTERANGE"]); brSpec != "" {
						br, nextEnd, brErr := parseHLSByteRange(brSpec, prevMapRangeEnd)
						if brErr != nil {
							return nil, fmt.Errorf("%w: %v", errHLSFallbackToYtDlp, brErr)
						}
						pl.InitMapByteRange = br
						prevMapRangeEnd = nextEnd
					}
				}
			}
		case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
			brSpec := strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-BYTERANGE:"))
			br, nextEnd, brErr := parseHLSByteRange(brSpec, prevRangeEnd)
			if brErr != nil {
				return nil, fmt.Errorf("%w: %v", errHLSFallbackToYtDlp, brErr)
			}
			pendingRange = br
			prevRangeEnd = nextEnd
		case strings.HasPrefix(line, "#"):
			continue
		default:
			idx := len(pl.Segments)
			segURL := resolveHLSRef(baseParsed, line, accessToken)
			var ivCopy []byte
			if len(currentIV) > 0 {
				ivCopy = append([]byte(nil), currentIV...)
			}
			pl.Segments = append(pl.Segments, hlsSegment{
				Index:               idx,
				URL:                 segURL,
				KeyURL:              currentKeyURL,
				IV:                  ivCopy,
				MediaSequenceNumber: mediaSeq + uint64(idx),
				ByteRange:           pendingRange,
			})
			pendingRange = hlsByteRange{}
		}
	}

	if len(pl.Segments) == 0 {
		return nil, errors.New("HLS playlist contains 0 segments")
	}

	if !pl.HasEndList && len(pl.Segments) < 10 && !strings.EqualFold(pl.PlaylistType, "VOD") {
		return nil, fmt.Errorf("%w: live or incomplete playlist (%d segments without EXT-X-ENDLIST)", errHLSFallbackToYtDlp, len(pl.Segments))
	}

	hasFmp4Ext := false
	for _, seg := range pl.Segments {
		noQ := strings.ToLower(strings.Split(strings.Split(seg.URL, "?")[0], "#")[0])
		if strings.HasSuffix(noQ, ".m4s") || strings.HasSuffix(noQ, ".mp4") || strings.HasSuffix(noQ, ".cmfv") || strings.HasSuffix(noQ, ".cmfa") {
			hasFmp4Ext = true
			break
		}
	}
	pl.IsFmp4 = pl.InitMapURL != "" || hasFmp4Ext
	return pl, nil
}

func cookieHeaderForURL(rawCookies, targetURL string) string {
	trimmed := strings.TrimSpace(rawCookies)
	if trimmed == "" {
		return ""
	}

	var targetHost string
	if u, err := url.Parse(targetURL); err == nil {
		targetHost = strings.ToLower(u.Hostname())
	}

	// JSON array of cookie objects
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var items []struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Domain string `json:"domain"`
		}
		if json.Unmarshal([]byte(trimmed), &items) == nil {
			pairs := make([]string, 0, len(items))
			for _, it := range items {
				if it.Name == "" {
					continue
				}
				if cookieDomainMatches(it.Domain, targetHost) {
					pairs = append(pairs, it.Name+"="+it.Value)
				}
			}
			return strings.Join(pairs, "; ")
		}
	}

	// JSON key/value map
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		var kv map[string]interface{}
		if json.Unmarshal([]byte(trimmed), &kv) == nil {
			pairs := make([]string, 0, len(kv))
			for k, v := range kv {
				if k != "" {
					pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
				}
			}
			return strings.Join(pairs, "; ")
		}
	}

	// Netscape HTTP Cookie File format (tab-separated)
	if strings.Contains(trimmed, "\t") {
		pairs := make([]string, 0, 16)
		for _, line := range strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 7 {
				continue
			}
			domain := strings.TrimSpace(fields[0])
			name := strings.TrimSpace(fields[5])
			val := strings.TrimSpace(fields[6])
			if name == "" {
				continue
			}
			if cookieDomainMatches(domain, targetHost) {
				pairs = append(pairs, name+"="+val)
			}
		}
		return strings.Join(pairs, "; ")
	}

	// Raw Cookie header string ("k1=v1; k2=v2")
	clean := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "Cookie:"), "cookie:"))
	return clean
}

func cookieDomainMatches(cookieDomain, targetHost string) bool {
	if cookieDomain == "" || targetHost == "" {
		return true
	}
	d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookieDomain)), ".")
	h := strings.ToLower(strings.TrimSpace(targetHost))
	if d == h {
		return true
	}
	return strings.HasSuffix(h, "."+d)
}

func applyTransferHeaders(req *http.Request, targetURL, referer, userAgent, rawCookies string, br hlsByteRange) {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		ua = av01UserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	if referer != "" {
		req.Header.Set("Referer", referer)
		if org := originFromURL(referer); org != "" {
			req.Header.Set("Origin", org)
		}
	}
	if ch := cookieHeaderForURL(rawCookies, targetURL); ch != "" {
		req.Header.Set("Cookie", ch)
	}
	if br.HasRange && br.Length > 0 {
		end := br.Offset + br.Length - 1
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", br.Offset, end))
	}
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 256*1024))
	_ = body.Close()
}

type httpStatusError struct {
	Code int
	URL  string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d for %s", e.Code, e.URL)
}

func isAuthBlockedStatus(err error) bool {
	var st *httpStatusError
	if errors.As(err, &st) {
		return st.Code == http.StatusUnauthorized || st.Code == http.StatusForbidden
	}
	return false
}

func doTransferRequest(ctx context.Context, customClient *http.Client, reqBuilder func() (*http.Request, error)) (*http.Response, error) {
	req, err := reqBuilder()
	if err != nil {
		return nil, err
	}
	client := customClient
	if client == nil {
		client = sharedHighThroughputClient
	}
	resp, err := client.Do(req)
	if err != nil && customClient == nil && ctx.Err() == nil {
		// Fall back once to HTTP/2 if origin rejects HTTP/1.1 ALPN or closes connection cleanly.
		if req2, rErr := reqBuilder(); rErr == nil {
			if resp2, err2 := sharedHTTP2FallbackClient.Do(req2); err2 == nil {
				return resp2, nil
			}
		}
	}
	return resp, err
}

func fetchTransferBytes(
	ctx context.Context,
	client *http.Client,
	targetURL string,
	referer string,
	userAgent string,
	rawCookies string,
	br hlsByteRange,
	maxAttempts int,
	failFastOnAuth bool,
	meter *throughputMeter,
	limiter *byteRateLimiter,
	onDelta func(int64),
) ([]byte, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		reqCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		resp, err := doTransferRequest(reqCtx, client, func() (*http.Request, error) {
			req, rErr := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
			if rErr != nil {
				return nil, rErr
			}
			applyTransferHeaders(req, targetURL, referer, userAgent, rawCookies, br)
			return req, nil
		})
		if err != nil {
			cancel()
			lastErr = err
		} else {
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				code := resp.StatusCode
				drainAndClose(resp.Body)
				cancel()
				lastErr = &httpStatusError{Code: code, URL: targetURL}
				if failFastOnAuth && (code == http.StatusUnauthorized || code == http.StatusForbidden) {
					return nil, lastErr
				}
			} else {
				var out bytes.Buffer
				if resp.ContentLength > 0 && resp.ContentLength < 64*1024*1024 {
					out.Grow(int(resp.ContentLength))
				}
				buf := make([]byte, hlsReadChunkBytes)
				var readErr error
				for {
					n, rErr := resp.Body.Read(buf)
					if n > 0 {
						if limiter != nil {
							if wErr := limiter.Wait(ctx, n); wErr != nil {
								readErr = wErr
								break
							}
						}
						out.Write(buf[:n])
						if meter != nil {
							meter.Add(int64(n))
						}
						if onDelta != nil {
							onDelta(int64(n))
						}
					}
					if rErr != nil {
						if rErr != io.EOF {
							readErr = rErr
						}
						break
					}
				}
				drainAndClose(resp.Body)
				cancel()
				if readErr == nil {
					return out.Bytes(), nil
				}
				lastErr = readErr
			}
		}

		if attempt < maxAttempts {
			backoff := time.Duration(250*(1<<(attempt-1))) * time.Millisecond
			if backoff > 3*time.Second {
				backoff = 3 * time.Second
			}
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, lastErr
}

func decryptAES128CBC(ciphertext, key, iv []byte) ([]byte, error) {
	if len(key) != aes.BlockSize {
		return nil, fmt.Errorf("invalid AES-128 key length %d", len(key))
	}
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("invalid AES-128 IV length %d", len(iv))
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid AES-128 ciphertext length %d", len(ciphertext))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)

	padLen := int(plain[len(plain)-1])
	if padLen < 1 || padLen > aes.BlockSize || padLen > len(plain) {
		return nil, errors.New("invalid PKCS#7 padding in AES-128 segment")
	}
	for i := len(plain) - padLen; i < len(plain); i++ {
		if plain[i] != byte(padLen) {
			return nil, errors.New("corrupt PKCS#7 padding in AES-128 segment")
		}
	}
	return plain[:len(plain)-padLen], nil
}

// DownloadHLSNative downloads a VOD HLS (.m3u8) stream concurrently over multi-TCP HTTP/1.1
// keep-alive connections, writes sequentially to disk with a large buffer, and remuxes via ffmpeg.
func DownloadHLSNative(ctx context.Context, opts HLSDownloadOptions) (string, error) {
	targetURL := strings.TrimSpace(opts.URL)
	if targetURL == "" {
		return "", errors.New("empty HLS URL")
	}
	if err := os.MkdirAll(opts.TargetDir, 0o755); err != nil {
		return "", err
	}

	effReferer := strings.TrimSpace(opts.Referer)
	if effReferer == "" && strings.Contains(strings.ToLower(targetURL), "iw01.xyz") {
		effReferer = av01Referer
	}
	ua := strings.TrimSpace(opts.UserAgent)
	if ua == "" {
		ua = av01UserAgent
	}

	// 1. Fetch master or media playlist
	firstBytes, err := fetchTransferBytes(ctx, opts.HTTPClient, targetURL, effReferer, ua, opts.Cookies, hlsByteRange{}, 3, true, nil, nil, nil)
	if err != nil {
		return "", err
	}
	firstText := string(firstBytes)
	if !strings.Contains(firstText, "#EXTM3U") {
		return "", fmt.Errorf("%w: response is not an #EXTM3U playlist", errHLSFallbackToYtDlp)
	}

	activePlaylistURL := targetURL
	activePlaylistText := firstText
	if strings.Contains(firstText, "#EXT-X-STREAM-INF:") {
		variantURL, vErr := selectHLSMasterVariant(firstText, targetURL, opts.Format)
		if vErr != nil {
			return "", vErr
		}
		activePlaylistURL = variantURL
		varBytes, vErr := fetchTransferBytes(ctx, opts.HTTPClient, activePlaylistURL, effReferer, ua, opts.Cookies, hlsByteRange{}, 3, true, nil, nil, nil)
		if vErr != nil {
			return "", vErr
		}
		activePlaylistText = string(varBytes)
	}

	pl, err := parseHLSMediaPlaylist(activePlaylistText, activePlaylistURL)
	if err != nil {
		return "", err
	}

	// 2. Pre-fetch unique AES-128 keys
	keyCache := make(map[string][]byte)
	if pl.InitKeyURL != "" {
		if _, ok := keyCache[pl.InitKeyURL]; !ok {
			kb, kErr := fetchTransferBytes(ctx, opts.HTTPClient, pl.InitKeyURL, effReferer, ua, opts.Cookies, hlsByteRange{}, 4, true, nil, nil, nil)
			if kErr != nil {
				return "", fmt.Errorf("failed fetching HLS init key: %w", kErr)
			}
			if len(kb) != aes.BlockSize {
				return "", fmt.Errorf("%w: unexpected AES-128 key size %d", errHLSFallbackToYtDlp, len(kb))
			}
			keyCache[pl.InitKeyURL] = kb
		}
	}
	for _, seg := range pl.Segments {
		if seg.KeyURL == "" {
			continue
		}
		if _, ok := keyCache[seg.KeyURL]; ok {
			continue
		}
		kb, kErr := fetchTransferBytes(ctx, opts.HTTPClient, seg.KeyURL, effReferer, ua, opts.Cookies, hlsByteRange{}, 4, true, nil, nil, nil)
		if kErr != nil {
			return "", fmt.Errorf("failed fetching HLS key: %w", kErr)
		}
		if len(kb) != aes.BlockSize {
			return "", fmt.Errorf("%w: unexpected AES-128 key size %d", errHLSFallbackToYtDlp, len(kb))
		}
		keyCache[seg.KeyURL] = kb
	}

	// 3. Create same-volume work directory and sequential buffered merged file
	workDir := filepath.Join(opts.TargetDir, ".hls_work_"+uuid.NewString()[:8])
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	mergedExt := ".ts"
	if pl.IsFmp4 {
		mergedExt = ".fmp4"
	}
	mergedPath := filepath.Join(workDir, "merged"+mergedExt)
	mergedFile, err := os.OpenFile(mergedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	mergedClosed := false
	defer func() {
		if !mergedClosed {
			_ = mergedFile.Close()
		}
	}()
	bufWriter := bufio.NewWriterSize(mergedFile, hlsDiskWriteBufferBytes)

	meter := newThroughputMeter()
	limiter := newByteRateLimiter(opts.RateLimitBps)
	var downloadedBytes atomic.Int64
	var doneSegs atomic.Int64
	totalSegs := len(pl.Segments)

	emitProgress := func(force bool, speedOverride, etaOverride string, pctOverride float64) {
		if opts.OnProgress == nil {
			return
		}
		dl := downloadedBytes.Load()
		done := int(doneSegs.Load())
		var estTotal int64
		if done > 0 && dl > 0 {
			avg := float64(dl) / float64(done)
			estTotal = int64(avg * float64(totalSegs))
			if estTotal < dl {
				estTotal = dl
			}
		}

		pct := pctOverride
		if pct < 0 {
			pct = (float64(done) / float64(totalSegs)) * 97.0
			if pct > 97.0 {
				pct = 97.0
			}
		}

		spd := speedOverride
		eta := etaOverride
		if spd == "" {
			bps := meter.SpeedBytesPerSec()
			spd = formatSpeedMiB(bps)
			if eta == "" && bps > 0 && estTotal > dl {
				eta = formatETASeconds(float64(estTotal-dl) / bps)
			}
		}
		opts.OnProgress(HLSProgress{
			Percent:    pct,
			Speed:      spd,
			ETA:        eta,
			Downloaded: dl,
			TotalBytes: estTotal,
		})
	}

	var lastEmitUnix atomic.Int64
	emitThrottled := func() {
		nowNs := time.Now().UnixNano()
		prev := lastEmitUnix.Load()
		if nowNs-prev >= int64(200*time.Millisecond) && lastEmitUnix.CompareAndSwap(prev, nowNs) {
			emitProgress(false, "", "", -1)
		}
	}

	// Write #EXT-X-MAP init segment first if present
	if pl.InitMapURL != "" {
		rawInit, initErr := fetchTransferBytes(
			ctx, opts.HTTPClient, pl.InitMapURL, effReferer, ua, opts.Cookies, pl.InitMapByteRange,
			4, true, meter, limiter, func(delta int64) {
				downloadedBytes.Add(delta)
			},
		)
		if initErr != nil {
			return "", fmt.Errorf("failed downloading HLS init segment: %w", initErr)
		}
		if pl.InitKeyURL != "" {
			iv := pl.InitIV
			if len(iv) == 0 {
				iv = sequenceIVBytes(0)
			}
			rawInit, initErr = decryptAES128CBC(rawInit, keyCache[pl.InitKeyURL], iv)
			if initErr != nil {
				return "", fmt.Errorf("failed decrypting HLS init segment: %w", initErr)
			}
		}
		if _, wErr := bufWriter.Write(rawInit); wErr != nil {
			return "", wErr
		}
		emitProgress(true, "", "", 1.0)
	}

	// 4. Sliding-window parallel fetch + ordered sequential writer
	workers := opts.Workers
	if workers < 1 {
		workers = defaultConnectionsPerTask
	}
	if workers > totalSegs {
		workers = totalSegs
	}
	maxLookahead := workers * 2
	if maxLookahead < 32 {
		maxLookahead = 32
	}

	type segSlot struct {
		ready     bool
		data      []byte
		spillPath string
	}

	slots := make([]segSlot, totalSegs)
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	nextWriteIdx := 0
	var memBufferedBytes int64
	var nextFetchIdx atomic.Int64

	dlCtx, dlCancel := context.WithCancel(ctx)
	defer dlCancel()

	var firstErr error
	var errOnce sync.Once
	setErr := func(e error) {
		if e == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = e
			dlCancel()
			mu.Lock()
			cond.Broadcast()
			mu.Unlock()
		})
	}

	go func() {
		<-dlCtx.Done()
		mu.Lock()
		cond.Broadcast()
		mu.Unlock()
	}()

	writerErrCh := make(chan error, 1)
	go func() {
		defer close(writerErrCh)
		for idx := 0; idx < totalSegs; idx++ {
			mu.Lock()
			for !slots[idx].ready && dlCtx.Err() == nil {
				cond.Wait()
			}
			if dlCtx.Err() != nil && !slots[idx].ready {
				mu.Unlock()
				writerErrCh <- dlCtx.Err()
				return
			}
			slot := slots[idx]
			slots[idx] = segSlot{}
			if len(slot.data) > 0 {
				memBufferedBytes -= int64(len(slot.data))
				if memBufferedBytes < 0 {
					memBufferedBytes = 0
				}
			}
			nextWriteIdx = idx + 1
			cond.Broadcast()
			mu.Unlock()

			if slot.spillPath != "" {
				sf, sErr := os.Open(slot.spillPath)
				if sErr != nil {
					writerErrCh <- sErr
					return
				}
				_, cErr := io.Copy(bufWriter, sf)
				_ = sf.Close()
				_ = os.Remove(slot.spillPath)
				if cErr != nil {
					writerErrCh <- cErr
					return
				}
			} else if len(slot.data) > 0 {
				if _, wErr := bufWriter.Write(slot.data); wErr != nil {
					writerErrCh <- wErr
					return
				}
			}

			doneSegs.Add(1)
			emitThrottled()
		}
		writerErrCh <- nil
	}()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if dlCtx.Err() != nil {
					return
				}
				idx := int(nextFetchIdx.Add(1) - 1)
				if idx >= totalSegs {
					return
				}

				// Bound lookahead so memory stays bounded while keeping workers fed.
				mu.Lock()
				for idx >= nextWriteIdx+maxLookahead && dlCtx.Err() == nil {
					cond.Wait()
				}
				mu.Unlock()
				if dlCtx.Err() != nil {
					return
				}

				seg := pl.Segments[idx]
				failFastAuth := idx < workers
				rawSeg, sErr := fetchTransferBytes(
					dlCtx, opts.HTTPClient, seg.URL, effReferer, ua, opts.Cookies, seg.ByteRange,
					5, failFastAuth, meter, limiter, func(delta int64) {
						downloadedBytes.Add(delta)
						emitThrottled()
					},
				)
				if sErr != nil {
					setErr(fmt.Errorf("segment %d failed: %w", idx, sErr))
					return
				}

				plainSeg := rawSeg
				if seg.KeyURL != "" {
					iv := seg.IV
					if len(iv) == 0 {
						iv = sequenceIVBytes(seg.MediaSequenceNumber)
					}
					var dErr error
					plainSeg, dErr = decryptAES128CBC(rawSeg, keyCache[seg.KeyURL], iv)
					if dErr != nil {
						setErr(fmt.Errorf("segment %d decrypt failed: %w", idx, dErr))
						return
					}
				}

				mu.Lock()
				if idx != nextWriteIdx && memBufferedBytes+int64(len(plainSeg)) > hlsInMemorySoftCapBytes {
					spillPath := filepath.Join(workDir, fmt.Sprintf("spill_%06d.bin", idx))
					mu.Unlock()
					if wErr := os.WriteFile(spillPath, plainSeg, 0o600); wErr != nil {
						setErr(wErr)
						return
					}
					mu.Lock()
					slots[idx] = segSlot{ready: true, spillPath: spillPath}
				} else {
					memBufferedBytes += int64(len(plainSeg))
					slots[idx] = segSlot{ready: true, data: plainSeg}
				}
				cond.Broadcast()
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	wErr := <-writerErrCh

	if firstErr != nil {
		return "", firstErr
	}
	if wErr != nil {
		return "", wErr
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	if err := bufWriter.Flush(); err != nil {
		return "", err
	}
	if err := mergedFile.Close(); err != nil {
		return "", err
	}
	mergedClosed = true

	// 5. Finalize container via ffmpeg (copy/remux to faststart MP4 or extract requested audio)
	emitProgress(true, "Finalizing", "Remuxing MP4", 98.0)

	stem := sanitizeCaptionFilename(opts.OutputStem)
	if stem == "" {
		if opts.IsAudio {
			stem = "audio"
		} else {
			stem = "video"
		}
	}

	_, ffErr := exec.LookPath("ffmpeg")
	if opts.IsAudio {
		if ffErr != nil {
			return "", fmt.Errorf("ffmpeg required for audio extraction: %w", ffErr)
		}
		ext := ".mp3"
		var audioArgs []string
		switch strings.ToLower(strings.TrimSpace(opts.Format)) {
		case "audio_m4a", "m4a", "aac":
			ext = ".m4a"
			audioArgs = []string{"-y", "-loglevel", "warning", "-i", mergedPath, "-vn", "-c:a", "aac", "-b:a", "192k"}
		case "audio_flac", "flac":
			ext = ".flac"
			audioArgs = []string{"-y", "-loglevel", "warning", "-i", mergedPath, "-vn", "-c:a", "flac"}
		case "audio_wav", "wav":
			ext = ".wav"
			audioArgs = []string{"-y", "-loglevel", "warning", "-i", mergedPath, "-vn", "-c:a", "pcm_s16le"}
		default:
			ext = ".mp3"
			audioArgs = []string{"-y", "-loglevel", "warning", "-i", mergedPath, "-vn", "-c:a", "libmp3lame", "-q:a", "2"}
		}
		finalPath := filepath.Join(opts.TargetDir, stem+ext)
		audioArgs = append(audioArgs, finalPath)
		if out, rErr := exec.CommandContext(ctx, "ffmpeg", audioArgs...).CombinedOutput(); rErr != nil {
			return "", fmt.Errorf("ffmpeg audio extraction failed: %s", strings.TrimSpace(string(out)))
		}
		return finalPath, nil
	}

	finalMP4 := filepath.Join(opts.TargetDir, stem+".mp4")
	if ffErr != nil {
		if pl.IsFmp4 {
			if err := os.Rename(mergedPath, finalMP4); err != nil {
				return "", err
			}
			return finalMP4, nil
		}
		finalTS := filepath.Join(opts.TargetDir, stem+".ts")
		if err := os.Rename(mergedPath, finalTS); err != nil {
			return "", err
		}
		return finalTS, nil
	}

	if pl.IsFmp4 {
		err1 := runFFmpegRemux(ctx, []string{"-y", "-loglevel", "warning", "-i", mergedPath, "-c", "copy", "-movflags", "+faststart", finalMP4})
		if err1 != nil {
			_ = os.Remove(finalMP4)
			err2 := runFFmpegRemux(ctx, []string{"-y", "-loglevel", "warning", "-fflags", "+genpts", "-i", mergedPath, "-c", "copy", "-movflags", "+faststart", finalMP4})
			if err2 != nil {
				_ = os.Remove(finalMP4)
				log.Warn().Msgf("[NativeHLS] ffmpeg fMP4 remux failed (%v), keeping raw fMP4 container", err2)
				if rErr := os.Rename(mergedPath, finalMP4); rErr != nil {
					return "", rErr
				}
			}
		}
		return finalMP4, nil
	}

	err1 := runFFmpegRemux(ctx, []string{"-y", "-loglevel", "warning", "-i", mergedPath, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", finalMP4})
	if err1 != nil {
		_ = os.Remove(finalMP4)
		err2 := runFFmpegRemux(ctx, []string{"-y", "-loglevel", "warning", "-i", mergedPath, "-c", "copy", "-movflags", "+faststart", finalMP4})
		if err2 != nil {
			_ = os.Remove(finalMP4)
			log.Warn().Msgf("[NativeHLS] ffmpeg TS remux failed (%v), preserving .ts file", err2)
			finalTS := filepath.Join(opts.TargetDir, stem+".ts")
			if rErr := os.Rename(mergedPath, finalTS); rErr != nil {
				return "", rErr
			}
			return finalTS, nil
		}
	}
	return finalMP4, nil
}

func runFFmpegRemux(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}
