package media

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"html/template"
	"io"
	"media-roller/src/utils"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Media struct {
	Id          string
	Name        string
	SizeInBytes int64
	HumanSize   string
}

var fetchIndexTmpl = template.Must(template.ParseFiles("templates/media/index.html"))

var downloadDir = getDownloadDir()
var idCharSet = regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString

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
	name := getMediaDirectory(id) + "%(id)s.%(ext)s"

	log.Info().Msgf("Downloading %s to %s", url, name)

	cookiesPath := getCookiesPath()

	defaultArgs := map[string]string{
		"--format":                "best",
		"--trim-filenames":        "100",
		"--recode-video":          "mp4",
		"--restrict-filenames":    "",
		"--write-info-json":       "",
		"--output":                name,
		"--no-check-certificates": "",
		"--extractor-args":        "instagram:image_persist=1",
	}

	if _, err := os.Stat(cookiesPath); err == nil {
		defaultArgs["--cookies"] = cookiesPath
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
		return "", err.Error(), err
	}

	eg := errgroup.Group{}

	eg.Go(func() error {
		_, errStdout = io.Copy(stdout, stdoutIn)
		return nil
	})

	_, errStderr = io.Copy(stderr, stderrIn)
	_ = eg.Wait()
	log.Info().Msgf("Done with %s", id)

	err = cmd.Wait()
	if err != nil {
		log.Error().Err(err).Msgf("cmd.Run() failed with %s", err)
		return "", strings.TrimSpace(stderrBuf.String()), err
	} else if errStdout != nil {
		log.Error().Msgf("failed to capture stdout: %v", errStdout)
	} else if errStderr != nil {
		log.Error().Msgf("failed to capture stderr: %v", errStderr)
	}

	moveJsonFiles(id)

	return id, "", nil
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
	if len(files) == 0 {
		return nil, errors.New("ID not found: " + id)
	}

	var medias []Media

	for _, f := range files {
		if !strings.HasSuffix(f, ".json") {
			fi, err2 := os.Stat(root + f)
			var size int64 = 0
			if err2 == nil {
				size = fi.Size()
			}

			media := Media{
				Id:          id,
				Name:        filepath.Base(f),
				SizeInBytes: size,
				HumanSize:   humanize.Bytes(uint64(size)),
			}
			medias = append(medias, media)
		}
	}

	return medias, nil
}

func getFileFromId(id string) (string, error) {
	root := getMediaDirectory(id)
	file, err := os.Open(root)
	if err != nil {
		return "", err
	}
	files, _ := file.Readdirnames(0)
	if len(files) == 0 {
		return "", errors.New("ID not found")
	}

	for _, f := range files {
		if !strings.HasSuffix(f, ".json") {
			return root + f, nil
		}
	}

	return "", errors.New("unable to find file")
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
	return idCharSet(id)
}

func getDownloadDir() string {
	dir := os.Getenv("MR_DOWNLOAD_DIR")
	if dir != "" {
		if !strings.HasSuffix(dir, "/") {
			return dir + "/"
		}
		return dir
	}
	return "downloads/"
}

func getCookiesPath() string {
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
