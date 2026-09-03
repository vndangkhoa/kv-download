package media

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// ExtractThumbnailFromVideo uses ffmpeg to capture a crisp video frame at 0.5s or 0.0s.
func ExtractThumbnailFromVideo(videoPath, thumbPath string) error {
	if _, err := os.Stat(videoPath); err != nil {
		return err
	}

	// Try at 0.5s with scale filter
	cmd := exec.Command("ffmpeg", "-y", "-ss", "00:00:00.500", "-i", videoPath, "-vframes", "1", "-vf", "scale=640:-2:force_original_aspect_ratio=decrease", "-q:v", "2", thumbPath)
	if err := cmd.Run(); err == nil {
		if fi, errStat := os.Stat(thumbPath); errStat == nil && fi.Size() > 0 {
			return nil
		}
	}

	// Fallback to 0.0s if video is very short
	cmd = exec.Command("ffmpeg", "-y", "-ss", "00:00:00.000", "-i", videoPath, "-vframes", "1", "-vf", "scale=640:-2:force_original_aspect_ratio=decrease", "-q:v", "2", thumbPath)
	if err := cmd.Run(); err == nil {
		if fi, errStat := os.Stat(thumbPath); errStat == nil && fi.Size() > 0 {
			return nil
		}
	}

	// Ultra fallback without scale filter
	cmd = exec.Command("ffmpeg", "-y", "-i", videoPath, "-vframes", "1", "-q:v", "2", thumbPath)
	if err := cmd.Run(); err == nil {
		if fi, errStat := os.Stat(thumbPath); errStat == nil && fi.Size() > 0 {
			return nil
		}
	}

	return fmt.Errorf("failed to extract video frame for %s", videoPath)
}

// ExtractThumbnailFromAudio attempts to extract embedded album artwork from audio files.
func ExtractThumbnailFromAudio(audioPath, thumbPath string) error {
	if _, err := os.Stat(audioPath); err != nil {
		return err
	}

	cmd := exec.Command("ffmpeg", "-y", "-i", audioPath, "-an", "-vcodec", "copy", thumbPath)
	if err := cmd.Run(); err == nil {
		if fi, errStat := os.Stat(thumbPath); errStat == nil && fi.Size() > 0 {
			return nil
		}
	}
	return fmt.Errorf("no embedded cover art found in %s", audioPath)
}

// ServeThumbnailHandler handles /thumbnail and /api/thumbnail requests.
// It checks for existing image files in the media folder or dynamically generates
// a thumbnail frame via ffmpeg if missing.
func ServeThumbnailHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Redirect(w, r, "/static/logo.svg", http.StatusTemporaryRedirect)
		return
	}

	// Clean path and prevent directory traversal
	id = strings.ReplaceAll(id, "\\", "/")
	parts := strings.Split(id, "/")
	dirID := parts[0]
	if dirID == "" || strings.Contains(dirID, "..") {
		http.Redirect(w, r, "/static/logo.svg", http.StatusTemporaryRedirect)
		return
	}

	// If the id is a Task ID, check if Queue has it
	if len(parts) == 1 {
		GlobalQueue.mu.RLock()
		task, exists := GlobalQueue.tasks[dirID]
		GlobalQueue.mu.RUnlock()
		if exists && task != nil && task.MediaID != "" {
			taskDir, _, _ := strings.Cut(task.MediaID, "/")
			if taskDir != "" {
				dirID = taskDir
			}
		}
	}

	// support both legacy hash and category/hash ids for thumbnail dir
	if len(parts) == 3 {
		// category/hash/file pattern — dirID already category, but we treat second part as hash
		// Actually for thumbnail id we expect hash or hash/file, not category/hash. Fallback to second part if looks like hash.
		// Keep logic simple: if 3 parts, take middle as hash
		if parts[1] != "" && !strings.Contains(parts[1], ".") {
			dirID = parts[1]
		}
	}
	targetDir := resolveMediaDirectory(dirID)
	if fi, err := os.Stat(targetDir); err != nil || !fi.IsDir() {
		http.Redirect(w, r, "/static/logo.svg", http.StatusTemporaryRedirect)
		return
	}

	// 1. If a specific image filename was requested in id and exists
	if len(parts) > 1 && parts[1] != "" {
		// filename is last part (support category/hash/file)
		filenamePart := filepath.Base(parts[len(parts)-1])
		reqFile := filepath.Join(targetDir, filenamePart)
		ext := strings.ToLower(filepath.Ext(reqFile))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
			if fi, err := os.Stat(reqFile); err == nil && fi.Size() > 0 {
				w.Header().Set("Cache-Control", "public, max-age=86400")
				w.Header().Set("Content-Type", getMimeType(reqFile))
				http.ServeFile(w, r, reqFile)
				return
			}
		}
	}

	// 2. Look for any existing image file in the directory
	var foundImage string
	var foundVideo string
	var foundAudio string

	if entries, err := os.ReadDir(targetDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			ext := strings.ToLower(filepath.Ext(name))
			switch ext {
			case ".jpg", ".jpeg", ".png", ".webp":
				if foundImage == "" || strings.HasPrefix(strings.ToLower(name), "thumb") {
					foundImage = filepath.Join(targetDir, name)
				}
			case ".mp4", ".webm", ".mkv", ".mov", ".avi", ".m4v":
				if foundVideo == "" {
					foundVideo = filepath.Join(targetDir, name)
				}
			case ".mp3", ".m4a", ".flac", ".ogg", ".wav":
				if foundAudio == "" {
					foundAudio = filepath.Join(targetDir, name)
				}
			}
		}
	}

	if foundImage != "" {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("Content-Type", getMimeType(foundImage))
		http.ServeFile(w, r, foundImage)
		return
	}

	// 3. If no image found, generate from video using ffmpeg
	thumbPath := filepath.Join(targetDir, "thumb.jpg")
	if foundVideo != "" {
		if err := ExtractThumbnailFromVideo(foundVideo, thumbPath); err == nil {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Header().Set("Content-Type", "image/jpeg")
			http.ServeFile(w, r, thumbPath)
			return
		} else {
			log.Warn().Err(err).Msgf("Failed generating video thumbnail for %s", foundVideo)
		}
	}

	// 4. Try extracting from audio cover art
	if foundAudio != "" {
		if err := ExtractThumbnailFromAudio(foundAudio, thumbPath); err == nil {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Header().Set("Content-Type", "image/jpeg")
			http.ServeFile(w, r, thumbPath)
			return
		}
	}

	// Fallback placeholder
	http.Redirect(w, r, "/static/logo.svg", http.StatusTemporaryRedirect)
}
