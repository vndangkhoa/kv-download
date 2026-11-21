package media

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"kv-download/src/utils"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Media struct {
	Id          string
	Name        string
	SizeInBytes int64
	HumanSize   string
}

var fetchIndexTmpl = parseIndexTemplate()

func parseIndexTemplate() *template.Template {
	paths := []string{"templates/media/index.html", "../../templates/media/index.html"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return template.Must(template.ParseFiles(p))
		}
	}
	return template.Must(template.ParseFiles("templates/media/index.html"))
}

var downloadDir string

func init() {
	downloadDir = getDownloadDir()
	ensureCategoryDirs()
	go func() {
		migrateAllLegacy()
	}()
}

func Index(w http.ResponseWriter, _ *http.Request) {
	data := map[string]string{
		"ytDlpVersion": CachedYtDlpVersion,
	}
	if err := fetchIndexTmpl.Execute(w, data); err != nil {
		log.Error().Msgf("Error rendering template: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

func FetchMedia(w http.ResponseWriter, r *http.Request) {
	url, args := getUrl(r)

	media, ytdlpErrorMessage, err := getMediaResults(url, args)
	data := map[string]interface{}{
		"url":          url,
		"media":        media,
		"error":        ytdlpErrorMessage,
		"ytDlpVersion": CachedYtDlpVersion,
	}
	if err != nil {
		_ = fetchIndexTmpl.Execute(w, data)
		return
	}

	if err = fetchIndexTmpl.Execute(w, data); err != nil {
		log.Error().Msgf("Error rendering template: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

func FetchMediaApi(w http.ResponseWriter, r *http.Request) {
	url, args := getUrl(r)
	medias, _, err := getMediaResults(url, args)
	if err != nil {
		log.Error().Msgf("error getting media results: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(medias) == 0 {
		log.Error().Msgf("not media found")
		http.Error(w, "Media not found", http.StatusBadRequest)
		return
	}

	streamFileToClientById(w, r, medias[0].Id)
}

func FetchMediaInfo(w http.ResponseWriter, r *http.Request) {
	url, args := getUrl(r)
	media, ytdlpErrorMessage, err := getMediaResults(url, args)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": ytdlpErrorMessage,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"media": media,
		"error": ytdlpErrorMessage,
	})
}

func ScanMediaApi(w http.ResponseWriter, r *http.Request) {
	url, _ := getUrl(r)
	cookies := strings.TrimSpace(r.URL.Query().Get("cookies"))
	if cookies == "" {
		cookies = strings.TrimSpace(r.Header.Get("X-Cookies"))
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if start <= 0 {
		start = 1
	}
	if limit <= 0 {
		limit = 24
	}
	info, ytdlpErrorMessage, err := ScanUrlWithPagination(url, cookies, start, limit)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": ytdlpErrorMessage,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"scan":   info,
		"rawUrl": url,
		"error":  ytdlpErrorMessage,
	})
}

// StreamScanMediaApi streams scanned entries live over SSE
func StreamScanMediaApi(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	url, _ := getUrl(r)
	cookies := strings.TrimSpace(r.URL.Query().Get("cookies"))
	if cookies == "" {
		cookies = strings.TrimSpace(r.Header.Get("X-Cookies"))
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()

	onMeta := func(title, uploader, channel, thumbnail string, total int) {
		data, _ := json.Marshal(map[string]interface{}{
			"type":      "meta",
			"title":     title,
			"uploader":  uploader,
			"channel":   channel,
			"thumbnail": thumbnail,
			"total":     total,
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	onEntry := func(entry ScanEntry, count int) {
		data, _ := json.Marshal(map[string]interface{}{
			"type":  "item",
			"entry": entry,
			"count": count,
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	err := StreamScanUrl(ctx, url, cookies, onEntry, onMeta)

	doneMsg := map[string]interface{}{
		"type": "done",
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		doneMsg["error"] = err.Error()
	}
	data, _ := json.Marshal(doneMsg)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func getUrl(r *http.Request) (string, map[string]string) {
	u := strings.TrimSpace(r.URL.Query().Get("url"))

	args := make(map[string]string)
	for k, v := range r.URL.Query() {
		if strings.HasPrefix(k, "-") {
			if len(v) > 0 {
				args[k] = v[0]
			} else {
				args[k] = ""
			}
		}
	}

	return u, args
}

func getMediaResults(inputUrl string, args map[string]string) ([]Media, string, error) {
	if inputUrl == "" {
		return nil, "", errors.New("missing URL")
	}

	url := utils.NormalizeUrl(inputUrl)
	log.Info().Msgf("Got input '%s' and extracted '%s' with args %v", inputUrl, url, args)

	id := GetMD5Hash(url, args)
	medias, err := getAllFilesForId(id)
	if err != nil {
		return nil, "", err
	}
	if len(medias) == 0 {
		errMessage := ""
		id, errMessage, err = downloadMedia(url, args)
		if err != nil {
			return nil, errMessage, err
		}
		medias, err = getAllFilesForId(id)
		if err != nil {
			return nil, "", err
		}
	}

	return medias, "", nil
}

func downloadMedia(url string, requestArgs map[string]string) (string, string, error) {
	id := GetMD5Hash(url, requestArgs)
	name := getMediaDirectory(id) + "%(title,description,id).100B.%(ext)s"

	log.Info().Msgf("Downloading %s to %s", url, name)

	cookiesPath := getCookiesPath()

	defaultArgs := map[string]string{
		"--format":                "bestvideo+bestaudio/best",
		"--no-playlist":           "",
		"--remux-video":           "mp4",
		"--merge-output-format":   "mp4",
		"--format-sort":           "res,vcodec:avc:vp9:av1,acodec:m4a:aac:opus,ext:mp4",
		"--output":                name,
		"--no-check-certificates": "",
		"--extractor-args":        "instagram:image_persist=1;threads:app_version=30.0.0",
	}

	if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
		defaultArgs["--impersonate"] = impersonate
	}

	if workingCookies := getWorkingCookiesPath(cookiesPath); workingCookies != "" {
		defaultArgs["--cookies"] = workingCookies
	}

	args := make([]string, 0)

	for arg, value := range defaultArgs {
		if _, has := requestArgs[arg]; !has {
			args = append(args, arg)
			if value != "" {
				args = append(args, value)
			}
		}
	}

	for arg, value := range requestArgs {
		args = append(args, arg)
		if value != "" {
			args = append(args, value)
		}
	}

	for arg, value := range getEnvVars() {
		if _, has := requestArgs[arg]; !has {
			args = append(args, arg)
			if value != "" {
				args = append(args, value)
			}
		}
	}

	targetURL := url
	effReferer := requestArgs["--referer"]
	isAV01 := false
	if _, _, ok := ParseAV01VideoURL(targetURL); ok {
		avCtx, avCancel := context.WithTimeout(context.Background(), 20*time.Second)
		av, avErr := ResolveAV01Video(avCtx, targetURL)
		avCancel()
		if avErr == nil && av != nil && len(av.Variants) > 0 {
			chosen := SelectAV01Variant(av.Variants, "")
			if chosen.URL != "" {
				targetURL = chosen.URL
				effReferer = av.Referer
				isAV01 = true
				safeStem := sanitizeCaptionFilename(av.Title)
				if safeStem != "" {
					for i := 0; i < len(args)-1; i++ {
						if args[i] == "--output" {
							args[i+1] = getMediaDirectory(id) + safeStem + ".%(ext)s"
						} else if args[i] == "--format" {
							args[i+1] = "bv*+ba/b/best"
						}
					}
				}
			}
		}
	} else if IsFacebookShareURL(targetURL) {
		if resolved := resolveFbShareURL(targetURL, fbCookieHeader("", targetURL)); resolved != "" && !IsFacebookShareURL(resolved) {
			targetURL = resolved
		}
	}
	if effReferer == "" && strings.Contains(strings.ToLower(targetURL), "iw01.xyz") {
		effReferer = av01Referer
	}
	if effReferer != "" && requestArgs["--referer"] == "" {
		args = append(args, "--referer", effReferer)
		if effOrigin := originFromURL(effReferer); effOrigin != "" {
			args = append(args, "--add-header", "Origin:"+effOrigin)
		}
	}
	isStreamURL := isHLSOrDashURL(targetURL)
	if isStreamURL {
		args = append(args, "--concurrent-fragments", "8")
	}

	infoJSONPath := ""
	if !isAV01 {
		if p, ok := GetCachedInfoJSON(targetURL); ok {
			infoJSONPath = p
		}
	}

	const maxAttempts = 3
	var lastErr, lastStderr string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptArgs := make([]string, len(args))
		copy(attemptArgs, args)
		if isStreamURL && attempt == 1 {
			attemptArgs = append(attemptArgs, "--hls-prefer-native")
		}

		usedInfoJSON := false
		if attempt == 1 && infoJSONPath != "" {
			if fi, err := os.Stat(infoJSONPath); err == nil && !fi.IsDir() && fi.Size() > 0 && time.Since(fi.ModTime()) < 4*time.Hour {
				usedInfoJSON = true
				attemptArgs = append(attemptArgs, "--load-info-json", infoJSONPath)
				log.Info().Msgf("[FastStart] Direct download using cached info-json for %s", targetURL)
			}
		}

		if !usedInfoJSON {
			attemptArgs = append(attemptArgs, targetURL)
		}

		lastErr, lastStderr = runYtDlp(attemptArgs)
		if lastErr == "" {
			moveJsonFiles(id)
			_ = organizeDownloadedFiles(id)
			return id, "", nil
		}

		if attempt == maxAttempts && isStreamURL {
			ffOut := filepath.Join(getMediaDirectory(id), id+".mp4")
			if ffErr := downloadHLSWithFFmpeg(context.Background(), targetURL, effReferer, ffOut, false); ffErr == nil {
				_ = organizeDownloadedFiles(id)
				return id, "", nil
			} else {
				lastErr = ffErr.Error()
			}
		}

		if usedInfoJSON {
			log.Warn().Msgf("Download with cached info-json failed for %s, invalidating cache for retry: %s", targetURL, lastErr)
			InvalidateCachedInfoJSON(targetURL)
		}

		if attempt < maxAttempts {
			log.Warn().Msgf("yt-dlp attempt %d/%d failed for %s: %s — retrying", attempt, maxAttempts, url, lastErr)
			time.Sleep(2 * time.Second)
		}
	}

	_ = os.RemoveAll(getMediaDirectory(id))
	return "", lastStderr, errors.New(lastErr)
}

func runYtDlp(args []string) (string, string) {
	cmd := exec.Command("yt-dlp", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutIn, _ := cmd.StdoutPipe()
	stderrIn, _ := cmd.StderrPipe()

	var errStdout, errStderr error
	stdout := io.MultiWriter(os.Stdout, &stdoutBuf)
	stderr := io.MultiWriter(os.Stderr, &stderrBuf)

	err := cmd.Start()
	if err != nil {
		log.Error().Msgf("Error starting command: %v", err)
		return err.Error(), err.Error()
	}

	eg := errgroup.Group{}

	eg.Go(func() error {
		_, errStdout = io.Copy(stdout, stdoutIn)
		return nil
	})

	_, errStderr = io.Copy(stderr, stderrIn)
	_ = eg.Wait()

	err = cmd.Wait()
	if err != nil {
		log.Error().Err(err).Msgf("cmd.Run() failed with %s", err)
		errMsg := strings.TrimSpace(stderrBuf.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return errMsg, errMsg
	} else if errStdout != nil {
		log.Error().Msgf("failed to capture stdout: %v", errStdout)
	} else if errStderr != nil {
		log.Error().Msgf("failed to capture stderr: %v", errStderr)
	}

	return "", ""
}

func moveJsonFiles(id string) {
	root := resolveMediaDirectory(id)
	jsonDir := filepath.Join(getDownloadDir(), "json")

	if err := os.MkdirAll(jsonDir, 0755); err != nil {
		log.Error().Msgf("failed to create json dir: %v", err)
		return
	}

	file, err := os.Open(root)
	if err != nil {
		return
	}
	files, _ := file.Readdirnames(0)
	file.Close()

	movedInfo := false
	for _, f := range files {
		if strings.HasSuffix(f, ".json") {
			src := filepath.Join(root, f)
			dst := filepath.Join(jsonDir, f)
			if strings.HasSuffix(strings.ToLower(f), ".info.json") && !movedInfo {
				dst = filepath.Join(jsonDir, id+".info.json")
				movedInfo = true
			}
			if err := os.Rename(src, dst); err != nil {
				log.Error().Msgf("failed to move json %s: %v", f, err)
			} else {
				log.Info().Msgf("moved json metadata to %s", dst)
			}
		}
	}
}

func getMediaDirectory(id string) string {
	return downloadDir + id + "/"
}

func getAllFilesForId(id string) ([]Media, error) {
	dir := getDownloadDir()
	var medias []Media
	seenFiles := make(map[string]bool)

	// 1. Check TaskIndex first
	if idx, err := readTaskIndex(id); err == nil && idx != nil && len(idx.Files) > 0 {
		for _, rel := range idx.Files {
			full := filepath.Join(dir, rel)
			if fi, err := os.Stat(full); err == nil && !fi.IsDir() && fi.Size() > 0 {
				filename := filepath.Base(rel)
				if !seenFiles[filename] {
					seenFiles[filename] = true
					medias = append(medias, Media{
						Id:          id + "/" + filename,
						Name:        filename,
						SizeInBytes: fi.Size(),
						HumanSize:   humanize.Bytes(uint64(fi.Size())),
					})
				}
			}
		}
	}

	// 2. Scan candidate directories (channel subdirs, category subdirs, legacy dirs)
	if len(medias) == 0 {
		candidates := candidateMediaDirs(id)
		for _, root := range candidates {
			f, err := os.Open(root)
			if err != nil {
				continue
			}
			names, _ := f.Readdirnames(0)
			f.Close()

			for _, filename := range names {
				ext := strings.ToLower(filepath.Ext(filename))
				if ext != ".json" && ext != ".part" && ext != ".ytdl" && ext != ".temp" {
					fullPath := filepath.Join(root, filename)
					fi, err2 := os.Stat(fullPath)
					var size int64 = 0
					if err2 == nil {
						size = fi.Size()
					}
					if size > 0 && !seenFiles[filename] {
						seenFiles[filename] = true
						media := Media{
							Id:          id + "/" + filename,
							Name:        filepath.Base(filename),
							SizeInBytes: size,
							HumanSize:   humanize.Bytes(uint64(size)),
						}
						medias = append(medias, media)
					}
				}
			}
		}
	}

	if len(medias) == 0 {
		return nil, nil
	}

	// Sort so playable video/audio files come first, followed by thumbnails
	isPlayable := func(name string) int {
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".flv", ".wmv", ".mpg", ".mpeg", ".ts":
			return 1
		case ".mp3", ".m4a", ".aac", ".flac", ".wav", ".ogg", ".oga", ".opus", ".wma", ".aiff":
			return 2
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".heic", ".heif", ".bmp", ".tiff":
			return 3
		default:
			return 4
		}
	}

	sort.Slice(medias, func(i, j int) bool {
		pi, pj := isPlayable(medias[i].Name), isPlayable(medias[j].Name)
		if pi != pj {
			return pi < pj
		}
		return medias[i].SizeInBytes > medias[j].SizeInBytes
	})

	return medias, nil
}

func getFileFromId(id string) (string, error) {
	if !isValidId(id) {
		return "", errors.New("invalid id")
	}
	if strings.HasSuffix(id, ".json") || strings.Contains(id, ".json/") || strings.HasSuffix(id, ".part") || strings.HasSuffix(id, ".ytdl") {
		return "", errors.New("invalid file type")
	}

	if GlobalQueue != nil && !strings.Contains(id, "/") {
		GlobalQueue.mu.RLock()
		t, ok := GlobalQueue.tasks[id]
		var mediaID, mediaName, title string
		if ok && t != nil {
			mediaID = t.MediaID
			mediaName = t.MediaName
			title = t.Title
		}
		GlobalQueue.mu.RUnlock()

		if ok {
			if mediaID != "" {
				if p, err := findMediaFile(mediaID); err == nil {
					return p, nil
				}
			}
			if mediaName != "" {
				if p, err := findMediaFile(mediaName); err == nil {
					return p, nil
				}
			}
			if clean := sanitizeCaptionFilename(title); clean != "" {
				for _, ext := range []string{".mp4", ".mkv", ".webm", ".mp3", ".m4a"} {
					if p, err := findMediaFile(clean + ext); err == nil {
						return p, nil
					}
				}
			}
		}
	}

	return findMediaFile(id)
}

func GetMD5Hash(url string, args map[string]string) string {
	id := url
	if len(args) > 0 {
		tmp := make([]string, 0)
		for k, v := range args {
			tmp = append(tmp, k, v)
		}
		sort.Strings(tmp)
		id += ":" + strings.Join(tmp, ",")
	}
	return fmt.Sprintf("%x", md5.Sum([]byte(id)))
}

func isValidId(id string) bool {
	if hasPathTraversal(id) {
		return false
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		return parts[0] != ""
	}
	if len(parts) == 2 {
		return parts[0] != "" && parts[1] != ""
	}
	if len(parts) == 3 {
		return parts[0] != "" && parts[1] != "" && parts[2] != ""
	}
	return false
}

var (
	downloadDirMu     sync.RWMutex
	customDownloadDir string
	customOrganizeMode string
)

func normalizeOrganizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "channel_category":
		return "channel_category"
	case "platform_channel":
		return "platform_channel"
	case "flat_category":
		return "flat_category"
	default:
		return "category_channel"
	}
}

func getOrganizeMode() string {
	if envMode := strings.TrimSpace(os.Getenv("MR_ORGANIZE_MODE")); envMode != "" {
		return normalizeOrganizeMode(envMode)
	}
	downloadDirMu.RLock()
	if customOrganizeMode != "" {
		m := customOrganizeMode
		downloadDirMu.RUnlock()
		return m
	}
	downloadDirMu.RUnlock()

	settingsPaths := []string{"degoog-data/server-settings.json", "../../degoog-data/server-settings.json"}
	for _, p := range settingsPaths {
		if data, err := os.ReadFile(p); err == nil {
			var cfg struct {
				Settings struct {
					OrganizeMode string `json:"organizeMode"`
				} `json:"settings"`
			}
			if err := json.Unmarshal(data, &cfg); err == nil && cfg.Settings.OrganizeMode != "" {
				m := normalizeOrganizeMode(cfg.Settings.OrganizeMode)
				downloadDirMu.Lock()
				customOrganizeMode = m
				downloadDirMu.Unlock()
				return m
			}
		}
	}
	return "category_channel"
}

func SetOrganizeMode(mode string) error {
	m := normalizeOrganizeMode(mode)
	downloadDirMu.Lock()
	customOrganizeMode = m
	downloadDirMu.Unlock()

	settingsFile := "degoog-data/server-settings.json"
	var cfg map[string]interface{}
	if data, err := os.ReadFile(settingsFile); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}
	settingsMap, ok := cfg["settings"].(map[string]interface{})
	if !ok {
		settingsMap = make(map[string]interface{})
		cfg["settings"] = settingsMap
	}
	settingsMap["organizeMode"] = m
	if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		_ = os.MkdirAll("degoog-data", 0o755)
		_ = os.WriteFile(settingsFile, b, 0o644)
	}
	return nil
}

func getDownloadDir() string {
	if envDir := os.Getenv("MR_DOWNLOAD_DIR"); envDir != "" {
		if strings.HasPrefix(envDir, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				envDir = filepath.Join(home, envDir[2:])
			}
		}
		if abs, err := filepath.Abs(envDir); err == nil {
			envDir = abs
		}
		if !strings.HasSuffix(envDir, string(filepath.Separator)) {
			envDir += string(filepath.Separator)
		}
		return envDir
	}

	downloadDirMu.RLock()
	if customDownloadDir != "" {
		d := customDownloadDir
		downloadDirMu.RUnlock()
		return d
	}
	downloadDirMu.RUnlock()

	// Check degoog-data/server-settings.json
	settingsPaths := []string{"degoog-data/server-settings.json", "../../degoog-data/server-settings.json"}
	for _, p := range settingsPaths {
		if data, err := os.ReadFile(p); err == nil {
			var cfg struct {
				Settings struct {
					DownloadDir string `json:"downloadDir"`
				} `json:"settings"`
			}
			if err := json.Unmarshal(data, &cfg); err == nil && cfg.Settings.DownloadDir != "" {
				d := cfg.Settings.DownloadDir
				if strings.HasPrefix(d, "~/") {
					if home, err := os.UserHomeDir(); err == nil {
						d = filepath.Join(home, d[2:])
					}
				}
				if abs, err := filepath.Abs(d); err == nil {
					d = abs
				}
				if !strings.HasSuffix(d, string(filepath.Separator)) {
					d += string(filepath.Separator)
				}
				downloadDirMu.Lock()
				customDownloadDir = d
				downloadDir = d
				downloadDirMu.Unlock()
				return d
			}
		}
	}

	dir := "downloads/"
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	downloadDirMu.Lock()
	customDownloadDir = dir
	downloadDir = dir
	downloadDirMu.Unlock()
	return dir
}

func SetDownloadDir(dirPath string) error {
	dirPath = strings.TrimSpace(dirPath)
	if dirPath == "" {
		return errors.New("empty path")
	}
	if strings.HasPrefix(dirPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dirPath = filepath.Join(home, dirPath[2:])
		}
	}
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(absPath, string(filepath.Separator)) {
		absPath += string(filepath.Separator)
	}

	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return err
	}

	downloadDirMu.Lock()
	customDownloadDir = absPath
	downloadDir = absPath
	downloadDirMu.Unlock()

	ensureCategoryDirs()

	// Persist to degoog-data/server-settings.json
	settingsFile := "degoog-data/server-settings.json"
	var cfg map[string]interface{}
	if data, err := os.ReadFile(settingsFile); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}
	settingsMap, ok := cfg["settings"].(map[string]interface{})
	if !ok {
		settingsMap = make(map[string]interface{})
		cfg["settings"] = settingsMap
	}
	settingsMap["downloadDir"] = absPath
	if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		_ = os.MkdirAll("degoog-data", 0o755)
		_ = os.WriteFile(settingsFile, b, 0o644)
	}

	log.Info().Msgf("Updated storage destination to %s", absPath)
	return nil
}

func GetStorageSettingHandler(w http.ResponseWriter, r *http.Request) {
	dir := getDownloadDir()
	mode := getOrganizeMode()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"path":         dir,
		"downloadDir":  dir,
		"organizeMode": mode,
	})
}

func SetStorageSettingHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path         string `json:"path"`
		DownloadDir  string `json:"downloadDir"`
		OrganizeMode string `json:"organizeMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	newPath := strings.TrimSpace(body.DownloadDir)
	if newPath == "" {
		newPath = strings.TrimSpace(body.Path)
	}
	if newPath != "" {
		if err := SetDownloadDir(newPath); err != nil {
			http.Error(w, "Failed to set storage directory: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if strings.TrimSpace(body.OrganizeMode) != "" {
		_ = SetOrganizeMode(body.OrganizeMode)
	}
	if newPath == "" && strings.TrimSpace(body.OrganizeMode) == "" {
		http.Error(w, "Path cannot be empty", http.StatusBadRequest)
		return
	}
	dir := getDownloadDir()
	mode := getOrganizeMode()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"path":         dir,
		"downloadDir":  dir,
		"organizeMode": mode,
	})
}

func getCookiesPath() string {
	if p := strings.TrimSpace(os.Getenv("MR_COOKIES_PATH")); p != "" {
		return p
	}
	execDir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	return filepath.Join(execDir, "cookies.txt")
}

func getEnvVars() map[string]string {
	vars := make(map[string]string)
	if ev := strings.TrimSpace(os.Getenv("MR_PROXY")); ev != "" {
		vars["--proxy"] = ev
	}
	return vars
}
