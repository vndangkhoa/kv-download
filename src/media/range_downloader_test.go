package media

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func buildDeterministicPayload(size int) ([]byte, [32]byte) {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i*31 + 7) & 0xFF)
	}
	return data, sha256.Sum256(data)
}

func TestDownloadDirectParallelRanges(t *testing.T) {
	const totalSize = 10 * 1024 * 1024
	payload, wantHash := buildDeterministicPayload(totalSize)

	var rangeReqCount atomic.Int32
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(totalSize))
			w.WriteHeader(http.StatusOK)
			return
		}
		rng := r.Header.Get("Range")
		if !strings.HasPrefix(rng, "bytes=") {
			w.Header().Set("Content-Length", strconv.Itoa(totalSize))
			_, _ = w.Write(payload)
			return
		}
		rangeReqCount.Add(1)
		cur := inFlight.Add(1)
		for {
			prev := maxInFlight.Load()
			if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		inFlight.Add(-1)

		parts := strings.Split(strings.TrimPrefix(rng, "bytes="), "-")
		start, _ := strconv.Atoi(parts[0])
		end, _ := strconv.Atoi(parts[1])
		if start < 0 || end >= totalSize || start > end {
			http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "parallel.bin")
	var lastProgress HLSProgress
	var progressCount atomic.Int32

	err := DownloadDirectParallel(t.Context(), DirectDownloadOptions{
		URL:         srv.URL + "/file.bin",
		OutputPath:  outPath,
		Connections: 8,
		ChunkSize:   1 * 1024 * 1024,
		HTTPClient:  srv.Client(),
		OnProgress: func(p HLSProgress) {
			progressCount.Add(1)
			lastProgress = p
		},
	})
	if err != nil {
		t.Fatalf("DownloadDirectParallel failed: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed reading output: %v", err)
	}
	gotHash := sha256.Sum256(got)
	if gotHash != wantHash {
		t.Fatalf("SHA-256 mismatch: got %x, want %x (size %d vs %d)", gotHash, wantHash, len(got), totalSize)
	}
	if rangeReqCount.Load() < 10 {
		t.Fatalf("expected >= 10 parallel range requests, got %d", rangeReqCount.Load())
	}
	if maxInFlight.Load() < 3 {
		t.Fatalf("expected concurrent range fetches (maxInFlight >= 3), got %d", maxInFlight.Load())
	}
	if progressCount.Load() == 0 || lastProgress.Downloaded != totalSize || lastProgress.Percent < 99.9 {
		t.Fatalf("unexpected progress summary: count=%d last=%+v", progressCount.Load(), lastProgress)
	}
}

func TestDownloadDirectFallbackSingleStream(t *testing.T) {
	const totalSize = 5 * 1024 * 1024
	payload, wantHash := buildDeterministicPayload(totalSize)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Ignore Range header and always return 200 OK
		w.Header().Set("Content-Length", strconv.Itoa(totalSize))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "single.bin")
	err := DownloadDirectParallel(t.Context(), DirectDownloadOptions{
		URL:         srv.URL + "/stream.bin",
		OutputPath:  outPath,
		Connections: 8,
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatalf("DownloadDirectParallel single-stream fallback failed: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed reading output: %v", err)
	}
	if sha256.Sum256(got) != wantHash {
		t.Fatalf("SHA-256 mismatch for single-stream fallback")
	}
}
