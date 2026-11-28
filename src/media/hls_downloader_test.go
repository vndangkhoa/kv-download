package media

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func encryptAES128PKCS7(plain, key, iv []byte) []byte {
	padLen := aes.BlockSize - (len(plain) % aes.BlockSize)
	padded := make([]byte, len(plain)+padLen)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out
}

func TestHLSDownloadFmp4ParallelAndOrdered(t *testing.T) {
	const totalSegs = 24
	initBytes := []byte("INIT_MAP_HEADER_BYTES_")
	segPayloads := make([][]byte, totalSegs)
	var expected bytes.Buffer
	expected.Write(initBytes)
	for i := 0; i < totalSegs; i++ {
		chunk := []byte(fmt.Sprintf("[SEG-%02d-%s]", i, strings.Repeat("X", 256)))
		segPayloads[i] = chunk
		expected.Write(chunk)
	}

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/master.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360\nlow.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080\nhigh.m3u8?access_token=tok99\n")
		case r.URL.Path == "/high.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			var sb strings.Builder
			sb.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:4\n")
			sb.WriteString("#EXT-X-MAP:URI=\"init.mp4\"\n")
			for i := 0; i < totalSegs; i++ {
				fmt.Fprintf(&sb, "#EXTINF:4.0,\nseg-%d.m4s\n", i)
			}
			sb.WriteString("#EXT-X-ENDLIST\n")
			_, _ = w.Write([]byte(sb.String()))
		case r.URL.Path == "/init.mp4":
			_, _ = w.Write(initBytes)
		case strings.HasPrefix(r.URL.Path, "/seg-") && strings.HasSuffix(r.URL.Path, ".m4s"):
			cur := inFlight.Add(1)
			for {
				prev := maxInFlight.Load()
				if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
					break
				}
			}
			time.Sleep(15 * time.Millisecond)
			inFlight.Add(-1)

			idxStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/seg-"), ".m4s")
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 0 || idx >= totalSegs {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(segPayloads[idx])
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := t.TempDir()
	var progressUpdates atomic.Int32
	var lastPct float64

	start := time.Now()
	outPath, err := DownloadHLSNative(context.Background(), HLSDownloadOptions{
		URL:        srv.URL + "/master.m3u8",
		Format:     "highest",
		TargetDir:  outDir,
		OutputStem: "parallel_test",
		Workers:    8,
		HTTPClient: srv.Client(),
		OnProgress: func(p HLSProgress) {
			progressUpdates.Add(1)
			lastPct = p.Percent
		},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("DownloadHLSNative failed: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed reading output %s: %v", outPath, err)
	}
	if !bytes.Equal(got, expected.Bytes()) {
		t.Fatalf("assembled output mismatch: got %d bytes, want %d bytes", len(got), expected.Len())
	}
	if maxInFlight.Load() < 3 {
		t.Fatalf("expected concurrent segment fetching (maxInFlight >= 3), got %d", maxInFlight.Load())
	}
	if elapsed >= 300*time.Millisecond {
		t.Logf("note: parallel test took %v (sequential would be >360ms, maxInFlight=%d)", elapsed, maxInFlight.Load())
	}
	if progressUpdates.Load() == 0 || lastPct < 90.0 {
		t.Fatalf("expected progress callbacks reaching finalization, got count=%d lastPct=%.1f", progressUpdates.Load(), lastPct)
	}
}

func TestHLSDownloadAES128Encryption(t *testing.T) {
	key := []byte("0123456789abcdef") // 16-byte AES-128 key
	explicitIV := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	const mediaSeq = 100

	seg0Plain := []byte("FIRST_EXPLICIT_IV_SEGMENT_" + strings.Repeat("A", 120))
	seg1Plain := []byte("SECOND_IMPLICIT_SEQ_IV_SEGMENT_" + strings.Repeat("B", 120))
	var expected bytes.Buffer
	expected.Write(seg0Plain)
	expected.Write(seg1Plain)

	seg0Enc := encryptAES128PKCS7(seg0Plain, key, explicitIV)
	seg1Enc := encryptAES128PKCS7(seg1Plain, key, sequenceIVBytes(mediaSeq+1))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/enc.m3u8":
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXT-X-PLAYLIST-TYPE:VOD\n", mediaSeq)
			fmt.Fprintf(w, "#EXT-X-KEY:METHOD=AES-128,URI=\"/key.bin\",IV=0x0102030405060708090a0b0c0d0e0f10\n")
			fmt.Fprintf(w, "#EXTINF:4.0,\nseg0.ts\n")
			fmt.Fprintf(w, "#EXT-X-KEY:METHOD=AES-128,URI=\"/key.bin\"\n")
			fmt.Fprintf(w, "#EXTINF:4.0,\nseg1.ts\n#EXT-X-ENDLIST\n")
		case "/key.bin":
			_, _ = w.Write(key)
		case "/seg0.ts":
			_, _ = w.Write(seg0Enc)
		case "/seg1.ts":
			_, _ = w.Write(seg1Enc)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := t.TempDir()
	outPath, err := DownloadHLSNative(context.Background(), HLSDownloadOptions{
		URL:        srv.URL + "/enc.m3u8",
		TargetDir:  outDir,
		OutputStem: "aes_test",
		Workers:    2,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("DownloadHLSNative AES-128 failed: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed reading output: %v", err)
	}
	if !bytes.Equal(got, expected.Bytes()) {
		t.Fatalf("decrypted output mismatch: got %q, want %q", string(got), expected.String())
	}
}

func TestHLSDownloadByteRange(t *testing.T) {
	partInit := []byte("MAP_PART_0_")
	partA := []byte("SEGMENT_A_BYTES_")
	partB := []byte("SEGMENT_B_BYTES_")
	var blob bytes.Buffer
	blob.Write(partInit)
	blob.Write(partA)
	blob.Write(partB)
	fullBlob := blob.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/br.m3u8":
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-PLAYLIST-TYPE:VOD\n")
			fmt.Fprintf(w, "#EXT-X-MAP:URI=\"media.mp4\",BYTERANGE=\"%d@0\"\n", len(partInit))
			fmt.Fprintf(w, "#EXTINF:4.0,\n#EXT-X-BYTERANGE:%d@%d\nmedia.mp4\n", len(partA), len(partInit))
			fmt.Fprintf(w, "#EXTINF:4.0,\n#EXT-X-BYTERANGE:%d\nmedia.mp4\n", len(partB))
			fmt.Fprintf(w, "#EXT-X-ENDLIST\n")
		case "/media.mp4":
			rangeHdr := r.Header.Get("Range")
			if !strings.HasPrefix(rangeHdr, "bytes=") {
				http.Error(w, "missing Range header", http.StatusBadRequest)
				return
			}
			parts := strings.Split(strings.TrimPrefix(rangeHdr, "bytes="), "-")
			start, _ := strconv.Atoi(parts[0])
			end, _ := strconv.Atoi(parts[1])
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(fullBlob)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(fullBlob[start : end+1])
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := t.TempDir()
	outPath, err := DownloadHLSNative(context.Background(), HLSDownloadOptions{
		URL:        srv.URL + "/br.m3u8",
		TargetDir:  outDir,
		OutputStem: "byterange_test",
		Workers:    2,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("DownloadHLSNative byte-range failed: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed reading output: %v", err)
	}
	if !bytes.Equal(got, fullBlob) {
		t.Fatalf("byte-range output mismatch: got %q, want %q", string(got), string(fullBlob))
	}
}

func TestHLSDownloadFallbackTriggers(t *testing.T) {
	demuxMaster := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aac",NAME="English",URI="audio.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=4000000,RESOLUTION=1920x1080,AUDIO="aac"
video.m3u8`
	if _, err := selectHLSMasterVariant(demuxMaster, "https://cdn.example.com/master.m3u8", "highest"); !errors.Is(err, errHLSFallbackToYtDlp) {
		t.Fatalf("expected errHLSFallbackToYtDlp for demuxed audio, got: %v", err)
	}

	sampleAES := `#EXTM3U
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="https://cdn.example.com/key"
#EXTINF:4.0,
seg0.ts
#EXT-X-ENDLIST`
	if _, err := parseHLSMediaPlaylist(sampleAES, "https://cdn.example.com/index.m3u8"); !errors.Is(err, errHLSFallbackToYtDlp) {
		t.Fatalf("expected errHLSFallbackToYtDlp for SAMPLE-AES, got: %v", err)
	}

	tinyLive := `#EXTM3U
#EXTINF:4.0,
seg0.ts
#EXTINF:4.0,
seg1.ts`
	if _, err := parseHLSMediaPlaylist(tinyLive, "https://cdn.example.com/live.m3u8"); !errors.Is(err, errHLSFallbackToYtDlp) {
		t.Fatalf("expected errHLSFallbackToYtDlp for live playlist without ENDLIST, got: %v", err)
	}
}

func TestHLSResolveRelativePreservesAccessToken(t *testing.T) {
	base := "https://customers.iw01.xyz/api/v1/videos/226601/manifest/index1080.m3u8?access_token=jwt123"
	playlist := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-MAP:URI="init-sv3-v1-a1.mp4"
#EXTINF:4.0,
/fmp4/226601/seg-1.m4s
#EXTINF:4.0,
https://cdn2.iw01.xyz/fmp4/226601/seg-2.m4s?access_token=already_set
#EXT-X-ENDLIST`

	pl, err := parseHLSMediaPlaylist(playlist, base)
	if err != nil {
		t.Fatalf("parseHLSMediaPlaylist failed: %v", err)
	}
	if !strings.Contains(pl.InitMapURL, "access_token=jwt123") {
		t.Fatalf("expected init map URL to preserve access_token, got %q", pl.InitMapURL)
	}
	if !strings.Contains(pl.Segments[0].URL, "access_token=jwt123") {
		t.Fatalf("expected segment 0 URL to preserve access_token, got %q", pl.Segments[0].URL)
	}
	if !strings.Contains(pl.Segments[1].URL, "access_token=already_set") {
		t.Fatalf("expected segment 1 URL to keep existing access_token, got %q", pl.Segments[1].URL)
	}
}
