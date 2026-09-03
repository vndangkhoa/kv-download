package media

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
)

type GalleryItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Title       string    `json:"title"`
	Channel     string    `json:"channel"`
	Category    string    `json:"category"`
	Path        string    `json:"path"`
	SizeInBytes int64     `json:"sizeInBytes"`
	HumanSize   string    `json:"humanSize"`
	ModTime     time.Time `json:"modTime"`
	Thumbnail   string    `json:"thumbnail"`
	URL         string    `json:"url"`
	Type        string    `json:"type"` // "video", "audio", "photo"
}

type GalleryFolder struct {
	Name       string `json:"name"`
	Category   string `json:"category"`
	FileCount  int    `json:"fileCount"`
	TotalBytes int64  `json:"totalBytes"`
	HumanSize  string `json:"humanSize"`
	Thumbnail  string `json:"thumbnail"`
}

type GalleryResponse struct {
	Items   []GalleryItem   `json:"items"`
	Folders []GalleryFolder `json:"folders"`
	Total   int             `json:"total"`
}

// ScanGallery scans the download directory directly from disk and returns all media items and channel folders.
func ScanGallery() (*GalleryResponse, error) {
	dir := getDownloadDir()
	resp := &GalleryResponse{
		Items:   make([]GalleryItem, 0),
		Folders: make([]GalleryFolder, 0),
	}

	folderMap := make(map[string]*GalleryFolder)

	// Scan videos, music, photos
	mediaCategories := []string{CategoryVideos, CategoryMusic, CategoryPhotos}

	for _, cat := range mediaCategories {
		catDir := filepath.Join(dir, cat)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				// Channel / Creator folder
				chanName := e.Name()
				chanDir := filepath.Join(catDir, chanName)
				subEntries, err := os.ReadDir(chanDir)
				if err != nil {
					continue
				}

				folderKey := cat + "/" + chanName
				if _, exists := folderMap[folderKey]; !exists {
					folderMap[folderKey] = &GalleryFolder{
						Name:     chanName,
						Category: cat,
					}
				}
				gf := folderMap[folderKey]

				for _, subE := range subEntries {
					if subE.IsDir() {
						continue
					}
					fn := subE.Name()
					ext := strings.ToLower(filepath.Ext(fn))
					if ext == ".part" || ext == ".ytdl" || ext == ".temp" || ext == ".json" {
						continue
					}
					info, err := subE.Info()
					if err != nil || info.Size() == 0 {
						continue
					}

					baseName := strings.TrimSuffix(fn, filepath.Ext(fn))
					relPath := filepath.Join(cat, chanName, fn)

					// Check matching thumbnail
					thumbURL := ""
					thumbRel := filepath.Join(CategoryThumbnails, chanName, baseName+".jpg")
					if _, err := os.Stat(filepath.Join(dir, thumbRel)); err == nil {
						thumbURL = "/thumbnail?path=" + url.QueryEscape(thumbRel)
					} else {
						// check with original ext or without channel
						thumbRel2 := filepath.Join(CategoryThumbnails, chanName, fn)
						if _, err := os.Stat(filepath.Join(dir, thumbRel2)); err == nil {
							thumbURL = "/thumbnail?path=" + url.QueryEscape(thumbRel2)
						}
					}
					if thumbURL == "" {
						thumbURL = "/thumbnail?path=" + url.QueryEscape(relPath)
					}

					if gf.Thumbnail == "" && thumbURL != "" {
						gf.Thumbnail = thumbURL
					}

					gf.FileCount++
					gf.TotalBytes += info.Size()

					itemType := "video"
					if cat == CategoryMusic {
						itemType = "audio"
					} else if cat == CategoryPhotos {
						itemType = "photo"
					}

					item := GalleryItem{
						ID:          relPath,
						Name:        fn,
						Title:       baseName,
						Channel:     chanName,
						Category:    cat,
						Path:        relPath,
						SizeInBytes: info.Size(),
						HumanSize:   humanize.Bytes(uint64(info.Size())),
						ModTime:     info.ModTime(),
						Thumbnail:   thumbURL,
						URL:         "/download?file=" + url.QueryEscape(relPath),
						Type:        itemType,
					}
					resp.Items = append(resp.Items, item)
				}
			} else {
				// Standalone file in category root
				fn := e.Name()
				ext := strings.ToLower(filepath.Ext(fn))
				if ext == ".part" || ext == ".ytdl" || ext == ".temp" || ext == ".json" {
					continue
				}
				info, err := e.Info()
				if err != nil || info.Size() == 0 {
					continue
				}

				baseName := strings.TrimSuffix(fn, filepath.Ext(fn))
				relPath := filepath.Join(cat, fn)

				thumbURL := ""
				thumbRel := filepath.Join(CategoryThumbnails, baseName+".jpg")
				if _, err := os.Stat(filepath.Join(dir, thumbRel)); err == nil {
					thumbURL = "/thumbnail?path=" + url.QueryEscape(thumbRel)
				} else {
					thumbURL = "/thumbnail?path=" + url.QueryEscape(relPath)
				}

				itemType := "video"
				if cat == CategoryMusic {
					itemType = "audio"
				} else if cat == CategoryPhotos {
					itemType = "photo"
				}

				item := GalleryItem{
					ID:          relPath,
					Name:        fn,
					Title:       baseName,
					Channel:     "",
					Category:    cat,
					Path:        relPath,
					SizeInBytes: info.Size(),
					HumanSize:   humanize.Bytes(uint64(info.Size())),
					ModTime:     info.ModTime(),
					Thumbnail:   thumbURL,
					URL:         "/download?file=" + url.QueryEscape(relPath),
					Type:        itemType,
				}
				resp.Items = append(resp.Items, item)
			}
		}
	}

	// Calculate human sizes for folders
	for _, gf := range folderMap {
		if gf.FileCount > 0 {
			gf.HumanSize = humanize.Bytes(uint64(gf.TotalBytes))
			resp.Folders = append(resp.Folders, *gf)
		}
	}

	// Sort folders alphabetically
	sort.Slice(resp.Folders, func(i, j int) bool {
		return resp.Folders[i].Name < resp.Folders[j].Name
	})

	// Sort items by ModTime descending (newest first)
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].ModTime.After(resp.Items[j].ModTime)
	})

	resp.Total = len(resp.Items)
	return resp, nil
}

// GalleryHandler returns all downloaded files and folders on disk in JSON.
func GalleryHandler(w http.ResponseWriter, r *http.Request) {
	gallery, err := ScanGallery()
	if err != nil {
		log.Error().Err(err).Msg("Failed to scan gallery")
		http.Error(w, "Failed to scan gallery", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(gallery)
}

// GalleryDeleteHandler deletes a media file and its matching thumbnail from disk.
func GalleryDeleteHandler(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if relPath == "" {
		relPath = strings.TrimSpace(r.URL.Query().Get("file"))
	}
	if relPath == "" || strings.Contains(relPath, "..") || strings.HasPrefix(relPath, "/") {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	dir := getDownloadDir()
	fullPath := filepath.Join(dir, relPath)

	// Verify target is within download dir
	if !strings.HasPrefix(fullPath, dir) {
		http.Error(w, "Invalid path traversal", http.StatusBadRequest)
		return
	}

	// 1. Remove media file
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Msgf("Failed to remove file %s", fullPath)
	}

	// 2. Remove matching thumbnail if applicable
	baseName := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) >= 2 {
		chanName := parts[1]
		thumbPath := filepath.Join(dir, CategoryThumbnails, chanName, baseName+".jpg")
		_ = os.Remove(thumbPath)
	} else {
		thumbPath := filepath.Join(dir, CategoryThumbnails, baseName+".jpg")
		_ = os.Remove(thumbPath)
	}

	log.Info().Msgf("Deleted gallery item: %s", relPath)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"deleted": relPath,
	})
}
