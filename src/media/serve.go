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
		if strings.Contains(relFile, "..") || strings.HasPrefix(relFile, "/") {
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

	fileContentType := getMimeType(filename)
	baseName := filepath.Base(filename)

	// Determine if this should be inline playback or attachment download
	isInline := r.URL.Query().Get("inline") == "true" || r.Header.Get("Range") != ""

	var disposition string
	if isInline {
		disposition = "inline"
	} else {
		disposition = "attachment"
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, url.PathEscape(baseName), url.PathEscape(baseName)))
	w.Header().Set("Content-Type", fileContentType)
	w.Header().Set("Accept-Ranges", "bytes")

	log.Info().Msgf("Opening file for streaming %s (%s, %d bytes, inline=%v)", filename, fileContentType, fi.Size(), isInline)

	serveFileWithRanges(w, r, filename, baseName, fi)
}

// serveFileWithRanges serves a file with proper HTTP range request support.
// This is essential for video/audio seeking in HTML5 video players.
func serveFileWithRanges(w http.ResponseWriter, r *http.Request, filename string, baseName string, fi os.FileInfo) {
	// Check for Range header to support seeking (essential for video playback)
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" && strings.HasPrefix(rangeHeader, "bytes=") {
		// Parse Range header: "bytes=start-end"
		parts := strings.SplitN(rangeHeader[6:], "-", 2)
		var start, end int64

		if len(parts) == 2 && parts[1] != "" {
			// Parse start and end from "bytes=start-end"
			fmt.Sscanf(parts[0], "%d", &start)
			fmt.Sscanf(parts[1], "%d", &end)
		} else if len(parts) == 1 && parts[0] != "" {
			// Parse "bytes=start-"
			fmt.Sscanf(parts[0], "%d", &start)
			end = fi.Size() - 1
		} else {
			// Invalid range, serve full file
			http.ServeFile(w, r, filename)
			return
		}

		if end == 0 {
			end = fi.Size() - 1
		}
		if end >= fi.Size() {
			end = fi.Size() - 1
		}

		if start < 0 || start >= fi.Size() {
			// Invalid range, serve full file
			http.ServeFile(w, r, filename)
			return
		}

		// Open file and seek to start position
		f, err := os.Open(filename)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer f.Close()

		_, err = f.Seek(start, io.SeekStart)
		if err != nil {
			http.Error(w, "Seek error", http.StatusInternalServerError)
			return
		}

		// Send partial content response
		contentLength := end - start + 1
		w.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fi.Size()))
		w.WriteHeader(http.StatusPartialContent)

		// Stream the requested byte range
		buf := make([]byte, 32*1024)
		toRead := contentLength
		for toRead > 0 {
			n := int64(len(buf))
			if n > toRead {
				n = toRead
			}
			nrInt, er := f.Read(buf[:n])
			nr := int64(nrInt)
			if nr > 0 {
				_, ww := w.Write(buf[:nr])
				if ww != nil {
					return
				}
			}
			toRead -= int64(nr)
			if nr == 0 || nr < n {
				break
			}
			// Handle write error
			if er != nil && er != io.EOF {
				log.Error().Msgf("Error reading file for range response: %v", er)
				return
			}
		}
		return
	}

	// No Range header — serve the full file
	http.ServeFile(w, r, filename)
}
