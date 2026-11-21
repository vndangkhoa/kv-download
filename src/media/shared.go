package media

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type dnsCacheEntry struct {
	ip        string
	expiresAt time.Time
	usedDoH   bool
}

var (
	dnsCacheMu      sync.RWMutex
	dnsCache        = make(map[string]dnsCacheEntry)
	dnsBlockedHosts sync.Map
)

var dohBootstrapClient = &http.Client{
	Timeout: 6 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		},
	},
}

// isHostDNSBlocked reports whether resolving host previously failed on system DNS
// and required Cloudflare/Google DoH fallback.
func isHostDNSBlocked(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	v, ok := dnsBlockedHosts.Load(h)
	return ok && v == true
}

func queryDoHJSON(ctx context.Context, endpointHost, pathPrefix, targetHost string) (string, error) {
	reqURL := fmt.Sprintf("https://%s%s?name=%s&type=A", endpointHost, pathPrefix, url.QueryEscape(targetHost))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := dohBootstrapClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DoH %s HTTP %d", endpointHost, resp.StatusCode)
	}
	var payload struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	for _, ans := range payload.Answer {
		if ans.Type == 1 {
			if ip := net.ParseIP(strings.TrimSpace(ans.Data)); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no A record in DoH response from %s", endpointHost)
}

func queryDirectUDPResolver(ctx context.Context, dnsServer, targetHost string) (string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "udp", dnsServer)
		},
	}
	ips, err := r.LookupIPAddr(ctx, targetHost)
	if err != nil {
		return "", err
	}
	for _, ip := range ips {
		if v4 := ip.IP.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsUnspecified() {
			return v4.String(), nil
		}
	}
	for _, ip := range ips {
		if !ip.IP.IsLoopback() && !ip.IP.IsUnspecified() {
			return ip.IP.String(), nil
		}
	}
	return "", fmt.Errorf("no usable IP from %s", dnsServer)
}

// resolveHostWithFallback resolves a hostname using system DNS first, then falls back
// to Cloudflare/Google DNS-over-HTTPS (DoH) and direct public DNS 1.1.1.1 / 8.8.8.8
// when local/ISP resolvers block or poison the domain.
func resolveHostWithFallback(ctx context.Context, host string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return "", fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.String(), nil
	}
	if h == "localhost" {
		return "127.0.0.1", nil
	}

	dnsCacheMu.RLock()
	if entry, ok := dnsCache[h]; ok && time.Now().Before(entry.expiresAt) {
		dnsCacheMu.RUnlock()
		return entry.ip, nil
	}
	dnsCacheMu.RUnlock()

	// 1. Try system DNS first
	sysCtx, sysCancel := context.WithTimeout(ctx, 3*time.Second)
	ips, err := net.DefaultResolver.LookupIPAddr(sysCtx, h)
	sysCancel()
	if err == nil && len(ips) > 0 {
		for _, ip := range ips {
			if v4 := ip.IP.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsUnspecified() {
				res := v4.String()
				dnsCacheMu.Lock()
				dnsCache[h] = dnsCacheEntry{ip: res, expiresAt: time.Now().Add(10 * time.Minute), usedDoH: false}
				dnsCacheMu.Unlock()
				return res, nil
			}
		}
		for _, ip := range ips {
			if !ip.IP.IsLoopback() && !ip.IP.IsUnspecified() {
				res := ip.IP.String()
				dnsCacheMu.Lock()
				dnsCache[h] = dnsCacheEntry{ip: res, expiresAt: time.Now().Add(10 * time.Minute), usedDoH: false}
				dnsCacheMu.Unlock()
				return res, nil
			}
		}
	}

	// 2. System DNS failed or returned sinkhole IP: fallback to Cloudflare & Google DoH
	dohCtx, dohCancel := context.WithTimeout(ctx, 7*time.Second)
	defer dohCancel()

	type fallbackSource struct {
		fn func() (string, error)
	}
	fallbacks := []fallbackSource{
		{fn: func() (string, error) { return queryDoHJSON(dohCtx, "1.1.1.1", "/dns-query", h) }},
		{fn: func() (string, error) { return queryDoHJSON(dohCtx, "8.8.8.8", "/resolve", h) }},
		{fn: func() (string, error) { return queryDirectUDPResolver(dohCtx, "1.1.1.1:53", h) }},
		{fn: func() (string, error) { return queryDirectUDPResolver(dohCtx, "8.8.8.8:53", h) }},
	}

	var lastErr = err
	for _, fb := range fallbacks {
		if resolved, fbErr := fb.fn(); fbErr == nil && resolved != "" {
			dnsBlockedHosts.Store(h, true)
			dnsCacheMu.Lock()
			dnsCache[h] = dnsCacheEntry{ip: resolved, expiresAt: time.Now().Add(10 * time.Minute), usedDoH: true}
			dnsCacheMu.Unlock()
			return resolved, nil
		} else if fbErr != nil {
			lastErr = fbErr
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("lookup %s: no such host", h)
}

var resilientTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, addr)
		}
		resolvedIP, err := resolveHostWithFallback(ctx, host)
		if err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(resolvedIP, port))
	},
}

// NewResilientHTTPClient creates an HTTP client using DoH/public DNS fallback transport.
func NewResilientHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: resilientTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

var httpClient = NewResilientHTTPClient(30 * time.Second)

// isVideoExt returns true if ext matches a recognized video file format.
func isVideoExt(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".flv", ".wmv", ".mpg", ".mpeg",
		".ts", ".mts", ".m2ts", ".3gp", ".3g2", ".f4v", ".asf", ".m2v", ".mp2":
		return true
	}
	return false
}

// isAudioExt returns true if ext matches a recognized audio file format.
func isAudioExt(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".mp3", ".m4a", ".m4b", ".aac", ".flac", ".wav", ".ogg", ".oga", ".opus",
		".wma", ".aiff", ".alac", ".aif", ".aifc", ".mid", ".mka", ".ac3", ".dts",
		".amr", ".awb", ".ra":
		return true
	}
	return false
}

// isImageExt returns true if ext matches an image/thumbnail format.
func isImageExt(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".heic", ".heif",
		".bmp", ".tiff", ".tif", ".svg", ".ico":
		return true
	}
	return false
}

// isThumbnailFilename reports whether a filename is a thumbnail or cover art rather than standalone photo.
func isThumbnailFilename(filename string) bool {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	switch base {
	case "thumb", "thumbnail", "cover", "poster", "preview", "folder", "albumart", "album_art", "default":
		return true
	}
	return strings.HasPrefix(base, "thumb") || strings.HasPrefix(base, "preview_") || strings.HasSuffix(base, "_thumb")
}

// hasPathTraversal reports whether p contains a ".." or "." path segment or backslash.
// Unlike strings.Contains(p, ".."), this allows filenames containing ellipses ("...")
// or multiple dots before an extension (e.g. "Title....mp4").
func hasPathTraversal(p string) bool {
	if strings.Contains(p, "\\") || strings.Contains(p, "\x00") {
		return true
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." {
			return true
		}
	}
	return false
}

// isHexHashDir reports whether name is a 32-character lowercase hex MD5 directory name.
func isHexHashDir(name string) bool {
	if len(name) != 32 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
