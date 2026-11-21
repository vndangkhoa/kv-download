package media

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
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

	streamFileToClientById(w, r, id)
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

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"; filename*=UTF-8''%s.zip`, strings.ReplaceAll(zipName, `"`, ""), url.PathEscape(zipName)))

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

func streamFileToClient(w http.ResponseWriter, r *http.Request, filename string) {
	fi, err := os.Stat(filename)
	if err != nil || fi.IsDir() {
		log.Error().Msgf("error finding file %s: %v", filename, err)
		http.Error(w, "File not found.", http.StatusNotFound)
		return
	}

	f, err := os.Open(filename)
	if err != nil {
		log.Error().Msgf("error opening file %s: %v", filename, err)
		http.Error(w, "Error opening file.", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	fileContentType := getMimeType(filename)
	baseName := filepath.Base(filename)

	// Determine if this should be inline playback or attachment download
	isInline := r.URL.Query().Get("inline") == "true"

	var disposition string
	if isInline {
		disposition = "inline"
	} else {
		disposition = "attachment"
	}

	// Clean ASCII fallback name for legacy clients + UTF-8 name for modern browsers
	escapedName := url.PathEscape(baseName)
	cleanName := strings.ReplaceAll(baseName, `"`, `\"`)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, cleanName, escapedName))
	w.Header().Set("Content-Type", fileContentType)
	w.Header().Set("Accept-Ranges", "bytes")

	// Use Go standard library ServeContent for optimal chunking, ranges, zero-copy and smooth seeking
	http.ServeContent(w, r, baseName, fi.ModTime(), f)
}
