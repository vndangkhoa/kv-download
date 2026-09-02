package media

import (
	"archive/zip"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

/**
This will serve the fetched files to the client
*/

func ServeMedia(w http.ResponseWriter, r *http.Request) {
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

	streamFileToClientById(w, r, id)
}

func DownloadAllZip(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["id"]
	if len(ids) == 0 {
		http.Error(w, "Missing file IDs", http.StatusBadRequest)
		return
	}

	var files []string
	seen := make(map[string]bool)
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

	log.Info().Msgf("Zipping %d files for download", len(files))

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="kv-download.zip"`)

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
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/mp4"
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

func streamFileToClient(w http.ResponseWriter, r *http.Request, filename string) {
	fi, err := os.Stat(filename)
	if err != nil || fi.IsDir() {
		log.Error().Msgf("error finding file %s: %v", filename, err)
		http.Error(w, "File not found.", http.StatusNotFound)
		return
	}

	fileContentType := getMimeType(filename)
	baseName := filepath.Base(filename)
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "true" || r.Header.Get("Range") != "" {
		disposition = "inline"
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, baseName, url.PathEscape(baseName)))
	w.Header().Set("Content-Type", fileContentType)
	w.Header().Set("Accept-Ranges", "bytes")

	log.Info().Msgf("Opening file for streaming %s (%s, %d bytes)", filename, fileContentType, fi.Size())
	http.ServeFile(w, r, filename)
}
