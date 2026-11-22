package media

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type cachedCodecInfo struct {
	Video string
	Audio string
}

const compatCacheMaxBytes int64 = 512 * 1024 * 1024

var (
	codecProbeCache    sync.Map // string -> cachedCodecInfo
	compatFileLocks    sync.Map // string -> *sync.Mutex
	compatTranscodeSem = make(chan struct{}, 1)
)

/**
This will serve the fetched files to the client
*/

func ServeMedia(w http.ResponseWriter, r *http.Request) {
	relFile := strings.TrimSpace(r.URL.Query().Get("file"))
	if relFile == "" {
		relFile = strings.TrimSpace(r.URL.Query().Get("path"))
	}
	if relFile != "" {
		if hasPathTraversal(relFile) || strings.HasPrefix(relFile, "/") {
			http.Error(w, "Invalid file path", http.StatusBadRequest)
			return
		}
		fullPath := filepath.Join(getDownloadDir(), relFile)
		if fi, err := os.Stat(fullPath); err == nil && !fi.IsDir() {
			streamFileToClient(w, r, fullPath)
			return
		}
	}

	id := r.URL.Query().Get("id")
	log.Info().Msgf("Serving file %s", id)
	if id == "" {
		http.Error(w, "Missing file ID", http.StatusBadRequest)
		return
	} else if !isValidId(id) {
		// Try to parse it just to avoid any type of directory traversal attacks
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}

	mediaIDFallback := strings.TrimSpace(r.URL.Query().Get("mediaId"))
	filename, err := getFileFromId(id)
	if err != nil && mediaIDFallback != "" && isValidId(mediaIDFallback) {
		if fbPath, fbErr := getFileFromId(mediaIDFallback); fbErr == nil {
			filename = fbPath
			err = nil
		}
	}
	if err != nil {
		log.Error().Msgf("error getting file from id %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	streamFileToClient(w, r, filename)
}

func DownloadAllZip(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["id"]
	groupID := strings.TrimSpace(r.URL.Query().Get("groupId"))
	folder := strings.TrimSpace(r.URL.Query().Get("folder"))

	zipName := "kv-download"
	var files []string
	seen := make(map[string]bool)

	if groupID != "" && GlobalQueue != nil {
		for _, t := range GlobalQueue.GetTasks() {
			if t.GroupID == groupID && t.Status == StatusCompleted && t.MediaID != "" {
				if zipName == "kv-download" && t.GroupTitle != "" {
					if clean := sanitizeFolderName(t.GroupTitle); clean != "" {
						zipName = clean
					}
				}
				if p, err := getFileFromId(t.MediaID); err == nil && !seen[p] {
					seen[p] = true
					files = append(files, p)
				}
			}
		}
	} else if folder != "" {
		if clean := sanitizeFolderName(folder); clean != "" {
			zipName = clean
		}
		if gallery, err := ScanGallery(); err == nil && gallery != nil {
			dir := getDownloadDir()
			for _, it := range gallery.Items {
				if strings.EqualFold(it.Channel, folder) || strings.EqualFold(it.Group, folder) {
					full := filepath.Join(dir, it.Path)
					if fi, err := os.Stat(full); err == nil && !fi.IsDir() && !seen[full] {
						seen[full] = true
						files = append(files, full)
					}
				}
			}
		}
	}

	for _, id := range ids {
		if !isValidId(id) {
			http.Error(w, "Invalid file ID", http.StatusBadRequest)
			return
		}
		path, err := getFileFromId(id)
		if err != nil || seen[path] {
			continue
		}
		seen[path] = true
		files = append(files, path)
	}

	if len(files) == 0 {
		http.Error(w, "No files found", http.StatusBadRequest)
		return
	}

	log.Info().Msgf("Zipping %d files for download (%s.zip)", len(files), zipName)

	asciiZip := asciiSafeFilename(zipName + ".zip")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s.zip`, asciiZip, url.PathEscape(zipName)))

	zw := zip.NewWriter(w)
	defer zw.Close()

	seenNames := make(map[string]int)
	for _, f := range files {
		name := filepath.Base(f)
		seenNames[name]++
		if seenNames[name] > 1 {
			name = fmt.Sprintf("%d-%s", seenNames[name]-1, name)
		}

		entry, err := zw.Create(name)
		if err != nil {
			log.Error().Msgf("zip: failed to create entry %s: %v", name, err)
			return
		}

		openfile, err := os.Open(f)
		if err != nil {
			log.Error().Msgf("zip: failed to open %s: %v", f, err)
			continue
		}
		_, err = io.Copy(entry, openfile)
		openfile.Close()
		if err != nil {
			log.Error().Msgf("zip: failed to copy %s: %v", f, err)
			return
		}
	}
}

func streamFileToClientById(w http.ResponseWriter, r *http.Request, id string) {
	filename, err := getFileFromId(id)
	if err != nil {
		log.Error().Msgf("error getting file from id %s: %v", id, err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	streamFileToClient(w, r, filename)
}

func getMimeType(filename string) string {
	clean := strings.Split(strings.Split(filename, "?")[0], "#")[0]
	ext := strings.ToLower(filepath.Ext(clean))
	switch ext {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".mpd":
		return "application/dash+xml"
	case ".m4s":
		return "video/iso.segment"
	case ".ts":
		return "video/mp2t"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".aac":
		return "audio/mp4"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	default:
		return "video/mp4"
	}
}

// asciiSafeFilename builds an RFC 6266 compliant ASCII fallback filename while preserving extension.
func asciiSafeFilename(name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	cleanExt := filterASCIIBytes(ext)
	cleanStem := strings.TrimSpace(filterASCIIBytes(stem))
	// Strip runs of underscores produced by replacing multi-byte emojis/accents
	for strings.Contains(cleanStem, "__") {
		cleanStem = strings.ReplaceAll(cleanStem, "__", "_")
	}
	cleanStem = strings.Trim(cleanStem, "_ ")
	if cleanStem == "" {
		cleanStem = "media"
	}
	return cleanStem + cleanExt
}

func filterASCIIBytes(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r <= 0x7E && r != '"' && r != '\\' && r != ';' && r != '/' {
			b.WriteRune(r)
		} else if r > 0x7E {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func compatCacheKey(filename string, fi os.FileInfo) string {
	raw := fmt.Sprintf("%s|%d|%d", filename, fi.Size(), fi.ModTime().UnixNano())
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func probeMediaCodecs(filename string, fi os.FileInfo) (videoCodec, audioCodec string) {
	key := compatCacheKey(filename, fi)
	if cached, ok := codecProbeCache.Load(key); ok {
		info := cached.(cachedCodecInfo)
		return info.Video, info.Audio
	}

	if _, err := exec.LookPath("ffprobe"); err != nil {
		return "", ""
	}
	out, err := exec.Command(
		"ffprobe",
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name",
		"-of", "json",
		filename,
	).Output()
	if err != nil {
		return "", ""
	}

	var parsed struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	if json.Unmarshal(out, &parsed) != nil {
		return "", ""
	}
	for _, s := range parsed.Streams {
		switch strings.ToLower(s.CodecType) {
		case "video":
			if videoCodec == "" {
				videoCodec = strings.ToLower(strings.TrimSpace(s.CodecName))
			}
		case "audio":
			if audioCodec == "" {
				audioCodec = strings.ToLower(strings.TrimSpace(s.CodecName))
			}
		}
	}
	codecProbeCache.Store(key, cachedCodecInfo{Video: videoCodec, Audio: audioCodec})
	return videoCodec, audioCodec
}

func needsBrowserCompatTranscode(ext, videoCodec, audioCodec string, forceCompat bool) bool {
	return needsBrowserCompatTranscodeForUA("", ext, videoCodec, audioCodec, forceCompat)
}

func isChromiumDesktopUA(ua string) bool {
	lower := strings.ToLower(ua)
	if strings.Contains(lower, "iphone") ||
		strings.Contains(lower, "ipad") ||
		strings.Contains(lower, "ipod") ||
		strings.Contains(lower, "crios/") ||
		strings.Contains(lower, "fxios/") ||
		strings.Contains(lower, "edgios/") {
		return false
	}
	return strings.Contains(lower, "chrome/") ||
		strings.Contains(lower, "chromium/") ||
		strings.Contains(lower, "edg/") ||
		strings.Contains(lower, "opr/")
}

func needsBrowserCompatTranscodeForUA(ua, ext, videoCodec, audioCodec string, forceCompat bool) bool {
	if forceCompat {
		return true
	}
	if isNativeMobilePlayerUA(ua) {
		return false
	}
	e := strings.ToLower(strings.TrimSpace(ext))
	switch e {
	case ".mkv", ".avi", ".mov", ".ts", ".mts", ".m2ts", ".flv", ".wmv":
		return true
	case ".mp4", ".m4v":
		if isChromiumDesktopUA(ua) {
			// Chrome/Edge/Android Chrome decode H.264/VP8/VP9/AV1/HEVC natively.
			// If hardware decode fails on a specific client, MiniPlayerBar falls back to ?compat=1.
			return audioCodec == "ac3" || audioCodec == "eac3" || audioCodec == "dts"
		}
		if videoCodec != "" && videoCodec != "h264" {
			// Firefox/Safari reject VP9/HEVC/AV1/MPEG4 muxed inside MP4 with
			// "No video with supported format and MIME type found."
			return true
		}
		if audioCodec == "opus" || audioCodec == "vorbis" || audioCodec == "ac3" || audioCodec == "eac3" || audioCodec == "dts" {
			return true
		}
	}
	return false
}

func getCompatLock(key string) *sync.Mutex {
	v, _ := compatFileLocks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func isNativeMobilePlayerUA(ua string) bool {
	lower := strings.ToLower(ua)
	return strings.Contains(lower, "exoplayer") ||
		strings.Contains(lower, "media3") ||
		strings.Contains(lower, "okhttp/") ||
		strings.Contains(lower, "dalvik/")
}

func compatFFmpegThreads() string {
	raw := strings.TrimSpace(os.Getenv("MR_FFMPEG_THREADS"))
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n < 1 {
				n = 1
			} else if n > 4 {
				n = 4
			}
			return strconv.Itoa(n)
		}
	}
	return "2"
}

func execLowPriorityCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	if nicePath, err := exec.LookPath("nice"); err == nil && nicePath != "" {
		niceArgs := append([]string{"-n", "15", name}, args...)
		return exec.CommandContext(ctx, nicePath, niceArgs...)
	}
	return exec.CommandContext(ctx, name, args...)
}

// CleanupCompatCache removes unfinished *.tmp.mp4 files on startup and enforces the disk cache limit.
func CleanupCompatCache() {
	cacheDir := filepath.Join(getDownloadDir(), ".compat_cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	removedTmp := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tmp.mp4") {
			if os.Remove(filepath.Join(cacheDir, entry.Name())) == nil {
				removedTmp++
			}
		}
	}
	if removedTmp > 0 {
		log.Info().Msgf("[CompatCache] Cleaned up %d unfinished temp transcode file(s) on startup", removedTmp)
	}
	pruneCompatCache(compatCacheMaxBytes)
}

func pruneCompatCache(maxBytes int64) {
	if maxBytes <= 0 {
		return
	}
	cacheDir := filepath.Join(getDownloadDir(), ".compat_cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	type cachedFile struct {
		path    string
		size    int64
		modTime time.Time
	}
	var files []cachedFile
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".tmp.mp4") {
			continue
		}
		info, iErr := entry.Info()
		if iErr != nil {
			continue
		}
		total += info.Size()
		files = append(files, cachedFile{
			path:    filepath.Join(cacheDir, entry.Name()),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}
	if total <= maxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, f := range files {
		if total <= maxBytes {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}

func hasCachedBrowserCompatibleMP4(filename string, fi os.FileInfo) (string, bool) {
	key := compatCacheKey(filename, fi)
	outPath := filepath.Join(getDownloadDir(), ".compat_cache", key+".mp4")
	if outInfo, err := os.Stat(outPath); err == nil && outInfo.Size() > 0 {
		return outPath, true
	}
	return "", false
}

func buildCompatFFmpegArgs(filename, videoCodec, audioCodec string, fragmented bool, outputTarget string) []string {
	threads := compatFFmpegThreads()
	args := []string{"-y", "-loglevel", "error", "-threads", threads, "-i", filename}
	if videoCodec == "h264" {
		args = append(args, "-c:v", "copy")
	} else {
		vf := "scale=trunc(iw/2)*2:trunc(ih/2)*2"
		if fragmented {
			vf = "scale='min(1280,iw)':trunc(ow/a/2)*2,scale=trunc(iw/2)*2:trunc(ih/2)*2"
		}
		args = append(
			args,
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-crf", "26",
			"-x264-params", "threads="+threads+":lookahead_threads=1",
			"-pix_fmt", "yuv420p",
			"-vf", vf,
		)
	}
	if audioCodec == "" {
		args = append(args, "-an")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2")
	}
	if fragmented {
		args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", outputTarget)
	} else {
		args = append(args, "-movflags", "+faststart", "-f", "mp4", outputTarget)
	}
	return args
}

func ensureBrowserCompatibleMP4(ctx context.Context, filename string, fi os.FileInfo, videoCodec, audioCodec string) (string, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("ffmpeg unavailable: %w", err)
	}
	if cached, ok := hasCachedBrowserCompatibleMP4(filename, fi); ok {
		return cached, nil
	}

	semWaitCtx, semCancel := context.WithTimeout(ctx, 15*time.Second)
	defer semCancel()
	select {
	case compatTranscodeSem <- struct{}{}:
		defer func() { <-compatTranscodeSem }()
	case <-semWaitCtx.Done():
		return "", fmt.Errorf("compat transcoder busy: %w", semWaitCtx.Err())
	}

	key := compatCacheKey(filename, fi)
	lock := getCompatLock(key)
	lock.Lock()
	defer lock.Unlock()

	cacheDir := filepath.Join(getDownloadDir(), ".compat_cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(cacheDir, key+".mp4")
	if outInfo, err := os.Stat(outPath); err == nil && outInfo.Size() > 0 {
		return outPath, nil
	}

	tmpPath := filepath.Join(cacheDir, key+".tmp.mp4")
	_ = os.Remove(tmpPath)

	runCtx, runCancel := context.WithTimeout(ctx, 90*time.Second)
	defer runCancel()

	args := buildCompatFFmpegArgs(filename, videoCodec, audioCodec, false, tmpPath)
	log.Info().Msgf("[CompatTranscode] Preparing H.264/AAC MP4 for %s (v=%s, a=%s)", filepath.Base(filename), videoCodec, audioCodec)
	if outBytes, err := execLowPriorityCommand(runCtx, "ffmpeg", args...).CombinedOutput(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("ffmpeg compat failed (%v): %s", err, strings.TrimSpace(string(outBytes)))
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	pruneCompatCache(compatCacheMaxBytes)
	return outPath, nil
}

func streamFragmentedCompatMP4(w http.ResponseWriter, r *http.Request, filename, baseName, videoCodec, audioCodec string, isInline bool) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg unavailable: %w", err)
	}

	semWaitCtx, semCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer semCancel()
	select {
	case compatTranscodeSem <- struct{}{}:
		defer func() { <-compatTranscodeSem }()
	case <-semWaitCtx.Done():
		return fmt.Errorf("compat transcoder busy: %w", semWaitCtx.Err())
	}

	args := buildCompatFFmpegArgs(filename, videoCodec, audioCodec, true, "pipe:1")
	log.Info().Msgf("[CompatStream] Streaming bounded H.264/AAC MP4 for %s (v=%s, a=%s)", baseName, videoCodec, audioCodec)
	cmd := execLowPriorityCommand(r.Context(), "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	setMediaDispositionHeaders(w, baseName, "video/mp4", isInline)
	w.WriteHeader(http.StatusOK)
	_, copyErr := io.Copy(w, stdout)
	waitErr := cmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	return waitErr
}

func setMediaDispositionHeaders(w http.ResponseWriter, baseName, contentType string, isInline bool) {
	disposition := "attachment"
	if isInline {
		disposition = "inline"
	}
	escapedName := url.PathEscape(baseName)
	asciiName := asciiSafeFilename(baseName)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, asciiName, escapedName))
	w.Header().Set("Content-Type", contentType)
}

func streamFileToClient(w http.ResponseWriter, r *http.Request, filename string) {
	const maxSyncCompatBytes = 35 * 1024 * 1024

	fi, err := os.Stat(filename)
	if err != nil || fi.IsDir() {
		log.Error().Msgf("error finding file %s: %v", filename, err)
		http.Error(w, "File not found.", http.StatusNotFound)
		return
	}

	isInline := r.URL.Query().Get("inline") == "true"
	forceCompat := r.URL.Query().Get("compat") == "1"

	servePath := filename
	serveInfo := fi
	serveExt := filepath.Ext(filename)
	fileContentType := getMimeType(filename)

	if (isInline || forceCompat) && isVideoExt(serveExt) {
		vCodec, aCodec := probeMediaCodecs(filename, fi)
		if needsBrowserCompatTranscodeForUA(r.UserAgent(), serveExt, vCodec, aCodec, forceCompat) {
			if cachedPath, ok := hasCachedBrowserCompatibleMP4(filename, fi); ok {
				if cFi, sErr := os.Stat(cachedPath); sErr == nil && cFi.Size() > 0 {
					servePath = cachedPath
					serveInfo = cFi
					fileContentType = "video/mp4"
				}
			} else if fi.Size() <= maxSyncCompatBytes && r.Method != http.MethodHead {
				if compatPath, cErr := ensureBrowserCompatibleMP4(r.Context(), filename, fi, vCodec, aCodec); cErr == nil {
					if cFi, sErr := os.Stat(compatPath); sErr == nil && cFi.Size() > 0 {
						servePath = compatPath
						serveInfo = cFi
						fileContentType = "video/mp4"
					}
				} else {
					log.Warn().Msgf("[CompatTranscode] Falling back to raw file for %s: %v", filename, cErr)
				}
			} else if fi.Size() > maxSyncCompatBytes {
				baseName := filepath.Base(filename)
				if r.Method == http.MethodHead {
					setMediaDispositionHeaders(w, baseName, "video/mp4", isInline)
					w.WriteHeader(http.StatusOK)
					return
				}
				if sErr := streamFragmentedCompatMP4(w, r, filename, baseName, vCodec, aCodec, isInline); sErr == nil {
					return
				} else {
					log.Warn().Msgf("[CompatStream] Falling back to raw file for %s: %v", filename, sErr)
				}
			}
		}
	}

	f, err := os.Open(servePath)
	if err != nil {
		log.Error().Msgf("error opening file %s: %v", servePath, err)
		http.Error(w, "Error opening file.", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	baseName := filepath.Base(filename)
	if fileContentType == "" {
		fileContentType = getMimeType(servePath)
	}

	setMediaDispositionHeaders(w, baseName, fileContentType, isInline)
	w.Header().Set("Accept-Ranges", "bytes")

	http.ServeContent(w, r, baseName, serveInfo.ModTime(), f)
}
