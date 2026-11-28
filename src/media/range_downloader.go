package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DirectDownloadOptions configures DownloadDirectParallel.
type DirectDownloadOptions struct {
	URL          string
	Referer      string
	Cookies      string
	UserAgent    string
	OutputPath   string
	Connections  int
	ChunkSize    int64
	RateLimitBps int64
	HTTPClient   *http.Client // Optional override (primarily for tests)
	OnProgress   func(HLSProgress)
}

type directChunk struct {
	start int64
	end   int64
}

func parseContentRangeTotal(header string) (int64, bool) {
	// Format: "bytes 0-0/10485760" or "bytes 0-1023/*"
	idx := strings.LastIndex(header, "/")
	if idx == -1 {
		return 0, false
	}
	totalStr := strings.TrimSpace(header[idx+1:])
	if totalStr == "" || totalStr == "*" {
		return 0, false
	}
	n, err := strconv.ParseInt(totalStr, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func probeDirectTarget(ctx context.Context, opts DirectDownloadOptions) (totalBytes int64, acceptRanges bool, liveResp *http.Response, err error) {
	ua := strings.TrimSpace(opts.UserAgent)
	if ua == "" {
		ua = av01UserAgent
	}

	// 1. Try lightweight HEAD probe
	headCtx, headCancel := context.WithTimeout(ctx, 15*time.Second)
	resp, headErr := doTransferRequest(headCtx, opts.HTTPClient, func() (*http.Request, error) {
		req, rErr := http.NewRequestWithContext(headCtx, http.MethodHead, opts.URL, nil)
		if rErr != nil {
			return nil, rErr
		}
		applyTransferHeaders(req, opts.URL, opts.Referer, ua, opts.Cookies, hlsByteRange{})
		return req, nil
	})
	if headErr == nil && resp != nil {
		cl := resp.ContentLength
		ar := strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")
		status := resp.StatusCode
		drainAndClose(resp.Body)
		headCancel()
		if status >= 200 && status < 300 && cl > 0 && (ar || cl < 4*1024*1024) {
			return cl, ar, nil, nil
		}
		if status >= 200 && status < 300 && cl > 0 {
			totalBytes = cl
		}
	} else {
		headCancel()
	}

	// 2. Fallback / confirmation via GET Range: bytes=0-0 (handles servers rejecting HEAD or omitting Accept-Ranges)
	probeCtx, probeCancel := context.WithTimeout(ctx, 20*time.Second)
	rangeResp, rangeErr := doTransferRequest(probeCtx, opts.HTTPClient, func() (*http.Request, error) {
		req, rErr := http.NewRequestWithContext(probeCtx, http.MethodGet, opts.URL, nil)
		if rErr != nil {
			return nil, rErr
		}
		applyTransferHeaders(req, opts.URL, opts.Referer, ua, opts.Cookies, hlsByteRange{})
		req.Header.Set("Range", "bytes=0-0")
		return req, nil
	})
	if rangeErr != nil {
		probeCancel()
		if totalBytes > 0 {
			return totalBytes, false, nil, nil
		}
		return 0, false, nil, rangeErr
	}

	if rangeResp.StatusCode == http.StatusPartialContent {
		cr := rangeResp.Header.Get("Content-Range")
		drainAndClose(rangeResp.Body)
		probeCancel()
		if parsedTotal, ok := parseContentRangeTotal(cr); ok {
			return parsedTotal, true, nil, nil
		}
		if totalBytes > 0 {
			return totalBytes, true, nil, nil
		}
		return 0, false, nil, nil
	}

	if rangeResp.StatusCode >= 200 && rangeResp.StatusCode < 300 {
		// Server ignored Range and started sending the full body — caller can stream liveResp directly!
		if rangeResp.ContentLength > 0 {
			totalBytes = rangeResp.ContentLength
		}
		// Keep probeCtx alive until caller consumes/closes liveResp; cancel only on return error.
		// Note: Do not call probeCancel() here because rangeResp.Body reads under probeCtx.
		// Instead tie probeCancel cleanly inside caller via wrapper or close.
		rangeResp.Body = &cancelOnCloseReadCloser{ReadCloser: rangeResp.Body, cancel: probeCancel}
		return totalBytes, false, rangeResp, nil
	}

	code := rangeResp.StatusCode
	drainAndClose(rangeResp.Body)
	probeCancel()
	return 0, false, nil, &httpStatusError{Code: code, URL: opts.URL}
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnCloseReadCloser) Close() error {
	err := c.ReadCloser.Close()
	if c.cancel != nil {
		c.cancel()
	}
	return err
}

// DownloadDirectParallel downloads a direct HTTP file using parallel Range workers when supported,
// writing chunks directly via pwrite (WriteAt) into a same-volume .kvpart file before atomic rename.
func DownloadDirectParallel(ctx context.Context, opts DirectDownloadOptions) error {
	targetURL := strings.TrimSpace(opts.URL)
	if targetURL == "" {
		return errors.New("empty direct download URL")
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return errors.New("empty output path")
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return err
	}

	ua := strings.TrimSpace(opts.UserAgent)
	if ua == "" {
		ua = av01UserAgent
	}

	totalBytes, acceptRanges, liveResp, err := probeDirectTarget(ctx, opts)
	if err != nil {
		if liveResp != nil {
			_ = liveResp.Body.Close()
		}
		return err
	}

	partPath := opts.OutputPath + ".kvpart"
	_ = os.Remove(partPath)
	success := false
	defer func() {
		if !success {
			_ = os.Remove(partPath)
		}
	}()

	conns := opts.Connections
	if conns < 1 {
		conns = defaultConnectionsPerTask
	}
	if conns > 32 {
		conns = 32
	}

	meter := newThroughputMeter()
	limiter := newByteRateLimiter(opts.RateLimitBps)
	var downloaded atomic.Int64
	var lastEmitUnix atomic.Int64

	emitProgress := func(force bool) {
		if opts.OnProgress == nil {
			return
		}
		if !force {
			nowNs := time.Now().UnixNano()
			prev := lastEmitUnix.Load()
			if nowNs-prev < int64(200*time.Millisecond) || !lastEmitUnix.CompareAndSwap(prev, nowNs) {
				return
			}
		}
		dl := downloaded.Load()
		var pct float64
		if totalBytes > 0 {
			pct = (float64(dl) / float64(totalBytes)) * 100.0
			if pct > 99.9 && dl < totalBytes {
				pct = 99.9
			}
			if pct > 100.0 {
				pct = 100.0
			}
		}
		bps := meter.SpeedBytesPerSec()
		eta := ""
		if bps > 0 && totalBytes > dl {
			eta = formatETASeconds(float64(totalBytes-dl) / bps)
		}
		opts.OnProgress(HLSProgress{
			Percent:    pct,
			Speed:      formatSpeedMiB(bps),
			ETA:        eta,
			Downloaded: dl,
			TotalBytes: totalBytes,
		})
	}

	// Parallel Range path
	if acceptRanges && totalBytes >= 4*1024*1024 && conns > 1 {
		if liveResp != nil {
			_ = liveResp.Body.Close()
		}

		chunkSize := opts.ChunkSize
		if chunkSize <= 0 {
			chunkSize = 4 * 1024 * 1024
			if totalBytes > 512*1024*1024 {
				chunkSize = 8 * 1024 * 1024
			}
		}

		outFile, fErr := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
		if fErr != nil {
			return fErr
		}
		outClosed := false
		defer func() {
			if !outClosed {
				_ = outFile.Close()
			}
		}()

		chunks := make([]directChunk, 0, int((totalBytes+chunkSize-1)/chunkSize))
		for start := int64(0); start < totalBytes; start += chunkSize {
			end := start + chunkSize - 1
			if end >= totalBytes {
				end = totalBytes - 1
			}
			chunks = append(chunks, directChunk{start: start, end: end})
		}

		workerCount := conns
		if workerCount > len(chunks) {
			workerCount = len(chunks)
		}

		chunkCh := make(chan directChunk, len(chunks))
		for _, ch := range chunks {
			chunkCh <- ch
		}
		close(chunkCh)

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
			})
		}

		var wg sync.WaitGroup
		for w := 0; w < workerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				buf := make([]byte, hlsReadChunkBytes)
				for ch := range chunkCh {
					if dlCtx.Err() != nil {
						return
					}
					if cErr := downloadChunkWriteAt(dlCtx, opts, ua, outFile, ch, buf, meter, limiter, &downloaded, emitProgress); cErr != nil {
						setErr(cErr)
						return
					}
				}
			}()
		}
		wg.Wait()

		if firstErr != nil {
			return firstErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cErr := outFile.Close(); cErr != nil {
			return cErr
		}
		outClosed = true

		if rErr := os.Rename(partPath, opts.OutputPath); rErr != nil {
			return rErr
		}
		success = true
		emitProgress(true)
		return nil
	}

	// Single high-buffer stream fallback (non-range servers or small files)
	resp := liveResp
	if resp == nil {
		var rErr error
		resp, rErr = doTransferRequest(ctx, opts.HTTPClient, func() (*http.Request, error) {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
			if reqErr != nil {
				return nil, reqErr
			}
			applyTransferHeaders(req, targetURL, opts.Referer, ua, opts.Cookies, hlsByteRange{})
			return req, nil
		})
		if rErr != nil {
			return rErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			code := resp.StatusCode
			drainAndClose(resp.Body)
			return &httpStatusError{Code: code, URL: targetURL}
		}
		if resp.ContentLength > 0 && totalBytes <= 0 {
			totalBytes = resp.ContentLength
		}
	}
	defer drainAndClose(resp.Body)

	outFile, fErr := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if fErr != nil {
		return fErr
	}
	outClosed := false
	defer func() {
		if !outClosed {
			_ = outFile.Close()
		}
	}()

	bw := bufio.NewWriterSize(outFile, hlsDiskWriteBufferBytes)
	buf := make([]byte, 256*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if limiter != nil {
				if wErr := limiter.Wait(ctx, n); wErr != nil {
					return wErr
				}
			}
			if _, wErr := bw.Write(buf[:n]); wErr != nil {
				return wErr
			}
			downloaded.Add(int64(n))
			meter.Add(int64(n))
			emitProgress(false)
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			return rErr
		}
	}

	if err := bw.Flush(); err != nil {
		return err
	}
	if err := outFile.Close(); err != nil {
		return err
	}
	outClosed = true

	if err := os.Rename(partPath, opts.OutputPath); err != nil {
		return err
	}
	success = true
	emitProgress(true)
	return nil
}

func downloadChunkWriteAt(
	ctx context.Context,
	opts DirectDownloadOptions,
	ua string,
	outFile *os.File,
	ch directChunk,
	buf []byte,
	meter *throughputMeter,
	limiter *byteRateLimiter,
	downloaded *atomic.Int64,
	emitProgress func(bool),
) error {
	const maxAttempts = 5
	expectedTotal := ch.end - ch.start + 1
	var written int64
	var lastErr error

	for attempt := 1; attempt <= maxAttempts && written < expectedTotal; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		curStart := ch.start + written
		reqCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		resp, err := doTransferRequest(reqCtx, opts.HTTPClient, func() (*http.Request, error) {
			req, rErr := http.NewRequestWithContext(reqCtx, http.MethodGet, opts.URL, nil)
			if rErr != nil {
				return nil, rErr
			}
			applyTransferHeaders(req, opts.URL, opts.Referer, ua, opts.Cookies, hlsByteRange{})
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", curStart, ch.end))
			return req, nil
		})
		if err != nil {
			cancel()
			lastErr = err
		} else {
			if resp.StatusCode != http.StatusPartialContent {
				code := resp.StatusCode
				drainAndClose(resp.Body)
				cancel()
				lastErr = fmt.Errorf("expected HTTP 206 for range %d-%d, got HTTP %d", curStart, ch.end, code)
				if code == http.StatusUnauthorized || code == http.StatusForbidden {
					return lastErr
				}
			} else {
				var streamErr error
				for written < expectedTotal {
					n, rErr := resp.Body.Read(buf)
					if n > 0 {
						remaining := expectedTotal - written
						toWrite := n
						if int64(toWrite) > remaining {
							toWrite = int(remaining)
						}
						if limiter != nil {
							if wErr := limiter.Wait(ctx, toWrite); wErr != nil {
								streamErr = wErr
								break
							}
						}
						if _, wErr := outFile.WriteAt(buf[:toWrite], ch.start+written); wErr != nil {
							streamErr = wErr
							break
						}
						written += int64(toWrite)
						downloaded.Add(int64(toWrite))
						meter.Add(int64(toWrite))
						emitProgress(false)
					}
					if rErr != nil {
						if rErr != io.EOF {
							streamErr = rErr
						}
						break
					}
				}
				drainAndClose(resp.Body)
				cancel()
				if streamErr == nil && written >= expectedTotal {
					return nil
				}
				if streamErr != nil {
					lastErr = streamErr
				} else {
					lastErr = fmt.Errorf("short range read: got %d of %d bytes", written, expectedTotal)
				}
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
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("incomplete chunk %d-%d", ch.start, ch.end)
	}
	return lastErr
}
