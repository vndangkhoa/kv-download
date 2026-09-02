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

var downloadDir = getDownloadDir()

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
	name := getMediaDirectory(id) + "%(title,channel+id,uploader+id,id)s.%(ext)s"

	log.Info().Msgf("Downloading %s to %s", url, name)

	cookiesPath := getCookiesPath()

	defaultArgs := map[string]string{
		"--format":                "b/bv*+ba/best",
		"--trim-filenames":        "120",
		"--no-playlist":           "",
		"--remux-video":           "mp4",
		"--merge-output-format":   "mp4",
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

	args = append(args, url)

	const maxAttempts = 3
	var lastErr, lastStderr string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr, lastStderr = runYtDlp(args)
		if lastErr == "" {
			moveJsonFiles(id)
			return id, "", nil
		}
		if attempt < maxAttempts {
			log.Warn().Msgf("yt-dlp attempt %d/%d failed for %s: %s — retrying", attempt, maxAttempts, url, lastErr)
			time.Sleep(2 * time.Second)
		}
	}

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
	root := getMediaDirectory(id)
	jsonDir := downloadDir + "json/"

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

	for _, f := range files {
		if strings.HasSuffix(f, ".json") {
			src := root + f
			dst := jsonDir + f
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
	root := getMediaDirectory(id)
	file, err := os.Open(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files, _ := file.Readdirnames(0)
	file.Close()

	var medias []Media

	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if ext != ".json" && ext != ".part" && ext != ".ytdl" && ext != ".temp" {
			fi, err2 := os.Stat(root + f)
			var size int64 = 0
			if err2 == nil {
				size = fi.Size()
			}
			if size > 0 {
				media := Media{
					Id:          id + "/" + f,
					Name:        filepath.Base(f),
					SizeInBytes: size,
					HumanSize:   humanize.Bytes(uint64(size)),
				}
				medias = append(medias, media)
			}
		}
	}

	// Sort so playable video/audio files come first
	isPlayable := func(name string) int {
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".mp4", ".webm", ".mkv", ".mov", ".avi":
			return 1
		case ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".wav":
			return 2
		default:
			return 3
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
	dirID, filename, _ := strings.Cut(id, "/")
	root := getMediaDirectory(dirID)
	if strings.HasSuffix(filename, ".json") || strings.HasSuffix(filename, ".part") || strings.HasSuffix(filename, ".ytdl") {
		return "", errors.New("invalid file type")
	}
	full := filepath.Join(root, filename)
	fi, err := os.Stat(full)
	if err != nil || fi.IsDir() {
		return "", errors.New("file not found")
	}
	return full, nil
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
	// Format is "<dirHash>/<filename>" - validate both parts to avoid path traversal
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		return false
	}
	dirHash, filename := parts[0], parts[1]
	if dirHash == "" || filename == "" {
		return false
	}
	if filepath.Base(filename) != filename || strings.Contains(filename, "..") || strings.Contains(filename, "\\") {
		return false
	}
	return true
}

func getDownloadDir() string {
	dir := os.Getenv("MR_DOWNLOAD_DIR")
	if dir == "" {
		dir = "downloads/"
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	return dir
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
