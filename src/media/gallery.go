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
	Group       string    `json:"group,omitempty"`
	Platform    string    `json:"platform,omitempty"`
	Duration    string    `json:"duration,omitempty"`
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
	Name       string    `json:"name"`
	Category   string    `json:"category"`
	Platform   string    `json:"platform,omitempty"`
	Subfolders []string  `json:"subfolders,omitempty"`
	FileCount  int       `json:"fileCount"`
	TotalBytes int64     `json:"totalBytes"`
	HumanSize  string    `json:"humanSize"`
	Thumbnail  string    `json:"thumbnail"`
	ModTime    time.Time `json:"modTime"`
}

type GalleryResponse struct {
	Items   []GalleryItem   `json:"items"`
	Folders []GalleryFolder `json:"folders"`
	Total   int             `json:"total"`
}

// findThumbnailForMedia resolves the best matching thumbnail image for a video/audio file.
func findThumbnailForMedia(dir, cat, folder, baseName, relPath string) string {
	candidates := []string{
		filepath.Join(CategoryThumbnails, folder, baseName+".jpg"),
		filepath.Join(CategoryThumbnails, folder, baseName+".webp"),
		filepath.Join(CategoryThumbnails, folder, baseName+".png"),
		filepath.Join(CategoryThumbnails, folder, "thumb.jpg"),
		filepath.Join(CategoryThumbnails, folder, "thumb.webp"),
		filepath.Join(CategoryThumbnails, folder, "thumb.png"),
		filepath.Join(CategoryThumbnails, baseName+".jpg"),
		filepath.Join(CategoryThumbnails, baseName+".webp"),
		filepath.Join(CategoryThumbnails, baseName+".png"),
		filepath.Join(cat, folder, baseName+".jpg"),
		filepath.Join(cat, folder, baseName+".webp"),
		filepath.Join(cat, folder, baseName+".png"),
		filepath.Join(cat, folder, "thumb.jpg"),
		filepath.Join(cat, folder, "thumb.webp"),
		filepath.Join(cat, folder, "thumb.png"),
		filepath.Join(cat, baseName+".jpg"),
		filepath.Join(cat, baseName+".webp"),
	}

	for _, cand := range candidates {
		if fi, err := os.Stat(filepath.Join(dir, cand)); err == nil && fi.Size() > 0 {
			return "/thumbnail?path=" + url.QueryEscape(cand)
		}
	}

	return "/thumbnail?path=" + url.QueryEscape(relPath)
}

// ScanGallery scans the download directory directly from disk and returns all media items and channel folders.
func ScanGallery() (*GalleryResponse, error) {
	dir := getDownloadDir()
	resp := &GalleryResponse{
		Items:   make([]GalleryItem, 0),
		Folders: make([]GalleryFolder, 0),
	}

	folderMap := make(map[string]*GalleryFolder)
	subfolderSeen := make(map[string]map[string]bool)

	// Build a lookup from filename/relPath -> task metadata for platform & duration enrichment
	metaByFile := make(map[string]*DownloadTask)
	if GlobalQueue != nil {
		for _, t := range GlobalQueue.GetTasks() {
			if t.MediaName != "" {
				metaByFile[t.MediaName] = t
			}
			if t.MediaID != "" {
				metaByFile[filepath.Base(t.MediaID)] = t
			}
		}
	}

	addFolderFile := func(folderKey, chanName, cat, subGroup, thumbURL, platform string, size int64, modTime time.Time) {
		if chanName == "" {
			return
		}
		gf, exists := folderMap[folderKey]
		if !exists {
			gf = &GalleryFolder{
				Name:     chanName,
				Category: cat,
				Platform: platform,
				ModTime:  modTime,
			}
			folderMap[folderKey] = gf
			subfolderSeen[folderKey] = make(map[string]bool)
		}
		if gf.Platform == "" && platform != "" {
			gf.Platform = platform
		}
		if gf.Thumbnail == "" && thumbURL != "" {
			gf.Thumbnail = thumbURL
		}
		if modTime.After(gf.ModTime) {
			gf.ModTime = modTime
		}
		if subGroup != "" && !subfolderSeen[folderKey][subGroup] {
			subfolderSeen[folderKey][subGroup] = true
			gf.Subfolders = append(gf.Subfolders, subGroup)
		}
		gf.FileCount++
		gf.TotalBytes += size
	}

	// 1. Scan standard category folders (videos, music, photos), supporting nested subfolders up to 3 levels
	mediaCategories := []string{CategoryVideos, CategoryMusic, CategoryPhotos}

	for _, cat := range mediaCategories {
		catDir := filepath.Join(dir, cat)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				chanName := e.Name()
				chanDir := filepath.Join(catDir, chanName)
				folderKey := cat + "/" + chanName

				_ = filepath.WalkDir(chanDir, func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return nil
					}
					relFromChan, rErr := filepath.Rel(chanDir, path)
					if rErr != nil {
						return nil
					}
					depth := 0
					if relFromChan != "." {
						depth = len(strings.Split(filepath.ToSlash(relFromChan), "/"))
					}
					if d.IsDir() {
						if depth > 3 {
							return filepath.SkipDir
						}
						return nil
					}

					fn := d.Name()
					ext := strings.ToLower(filepath.Ext(fn))
					if ext == ".part" || ext == ".ytdl" || ext == ".temp" || ext == ".json" {
						return nil
					}
					if cat == CategoryVideos && !isVideoExt(ext) {
						return nil
					}
					if cat == CategoryMusic && !isAudioExt(ext) {
						return nil
					}
					if cat == CategoryPhotos {
						if !isImageExt(ext) || isThumbnailFilename(fn) {
							return nil
						}
					}

					info, err := d.Info()
					if err != nil || info.Size() == 0 {
						return nil
					}

					baseName := strings.TrimSuffix(fn, filepath.Ext(fn))
					relFromDir, _ := filepath.Rel(dir, path)
					relPath := filepath.ToSlash(relFromDir)

					subGroup := ""
					subDirName := filepath.Base(filepath.Dir(path))
					if subDirName != chanName {
						subGroup = subDirName
					}

					thumbURL := findThumbnailForMedia(dir, cat, chanName, baseName, relPath)
					if thumbURL == "/thumbnail?path="+url.QueryEscape(relPath) && subGroup != "" {
						altThumb := filepath.Join(CategoryThumbnails, chanName, subGroup, baseName+".jpg")
						if fiT, errT := os.Stat(filepath.Join(dir, altThumb)); errT == nil && fiT.Size() > 0 {
							thumbURL = "/thumbnail?path=" + url.QueryEscape(altThumb)
						}
					}

					itemType := "video"
					if cat == CategoryMusic {
						itemType = "audio"
					} else if cat == CategoryPhotos {
						itemType = "photo"
					}

					platform := ""
					duration := ""
					if t, ok := metaByFile[fn]; ok && t != nil {
						platform = t.Platform
						duration = t.Duration
						if subGroup == "" && t.GroupTitle != "" && t.GroupTitle != chanName {
							subGroup = t.GroupTitle
						}
					}

					addFolderFile(folderKey, chanName, cat, subGroup, thumbURL, platform, info.Size(), info.ModTime())

					item := GalleryItem{
						ID:          relPath,
						Name:        fn,
						Title:       baseName,
						Channel:     chanName,
						Group:       subGroup,
						Platform:    platform,
						Duration:    duration,
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
					return nil
				})
			} else {
				// Standalone file in category root
				fn := e.Name()
				ext := strings.ToLower(filepath.Ext(fn))
				if ext == ".part" || ext == ".ytdl" || ext == ".temp" || ext == ".json" {
					continue
				}

				if cat == CategoryVideos && !isVideoExt(ext) {
					continue
				}
				if cat == CategoryMusic && !isAudioExt(ext) {
					continue
				}
				if cat == CategoryPhotos {
					if !isImageExt(ext) || isThumbnailFilename(fn) {
						continue
					}
				}

				info, err := e.Info()
				if err != nil || info.Size() == 0 {
					continue
				}

				baseName := strings.TrimSuffix(fn, filepath.Ext(fn))
				relPath := filepath.ToSlash(filepath.Join(cat, fn))

				thumbURL := findThumbnailForMedia(dir, cat, "", baseName, relPath)

				itemType := "video"
				if cat == CategoryMusic {
					itemType = "audio"
				} else if cat == CategoryPhotos {
					itemType = "photo"
				}

				platform := ""
				duration := ""
				if t, ok := metaByFile[fn]; ok && t != nil {
					platform = t.Platform
					duration = t.Duration
				}

				item := GalleryItem{
					ID:          relPath,
					Name:        fn,
					Title:       baseName,
					Channel:     "",
					Platform:    platform,
					Duration:    duration,
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

	// 2. Also scan any unorganized legacy root folders or channel-first folders in download directory
	if rootEntries, err := os.ReadDir(dir); err == nil {
		for _, re := range rootEntries {
			if !re.IsDir() {
				continue
			}
			folderName := re.Name()
			if isCategoryName(folderName) || folderName == "json" || strings.HasPrefix(folderName, ".") {
				continue
			}

			folderDir := filepath.Join(dir, folderName)
			subEntries, err := os.ReadDir(folderDir)
			if err != nil {
				continue
			}

			// Check if this folder contains any video or audio files (directly or 1 level deep)
			hasMedia := false
			hasActiveDownload := false
			_ = filepath.WalkDir(folderDir, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(d.Name()))
				if isVideoExt(ext) || isAudioExt(ext) {
					hasMedia = true
				}
				if ext == ".part" || ext == ".ytdl" || ext == ".temp" {
					hasActiveDownload = true
				}
				return nil
			})

			// Never treat leftover thumbnails inside a 32-char MD5 staging folder as gallery items
			if isHexHashDir(folderName) && !hasMedia {
				if !hasActiveDownload {
					_ = os.RemoveAll(folderDir)
				}
				continue
			}

			_ = subEntries
			_ = filepath.WalkDir(folderDir, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				fn := d.Name()
				ext := strings.ToLower(filepath.Ext(fn))
				if ext == ".part" || ext == ".ytdl" || ext == ".temp" || ext == ".json" {
					return nil
				}
				if hasMedia && isImageExt(ext) {
					return nil
				}
				if isThumbnailFilename(fn) {
					return nil
				}

				info, err := d.Info()
				if err != nil || info.Size() == 0 {
					return nil
				}

				baseName := strings.TrimSuffix(fn, filepath.Ext(fn))
				relFromDir, _ := filepath.Rel(dir, path)
				relPath := filepath.ToSlash(relFromDir)

				itemType := "video"
				cat := CategoryVideos
				if isAudioExt(ext) {
					itemType = "audio"
					cat = CategoryMusic
				} else if isImageExt(ext) {
					itemType = "photo"
					cat = CategoryPhotos
				}

				thumbURL := findThumbnailForMedia(dir, cat, folderName, baseName, relPath)
				platform := ""
				duration := ""
				if t, ok := metaByFile[fn]; ok && t != nil {
					platform = t.Platform
					duration = t.Duration
				}

				addFolderFile(folderName, folderName, cat, "", thumbURL, platform, info.Size(), info.ModTime())

				item := GalleryItem{
					ID:          relPath,
					Name:        fn,
					Title:       baseName,
					Channel:     folderName,
					Platform:    platform,
					Duration:    duration,
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
				return nil
			})
		}
	}

	// Calculate human sizes for folders
	for _, gf := range folderMap {
		if gf.FileCount > 0 {
			sort.Strings(gf.Subfolders)
			gf.HumanSize = humanize.Bytes(uint64(gf.TotalBytes))
			resp.Folders = append(resp.Folders, *gf)
		}
	}

	// Sort folders alphabetically (case-insensitive)
	sort.Slice(resp.Folders, func(i, j int) bool {
		return strings.ToLower(resp.Folders[i].Name) < strings.ToLower(resp.Folders[j].Name)
	})

	// Sort items by ModTime descending (newest first)
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].ModTime.After(resp.Items[j].ModTime)
	})

	resp.Total = len(resp.Items)
	return resp, nil
}

// GalleryHandler returns all downloaded files and folders on disk in JSON, with optional server-side filtering and sorting.
func GalleryHandler(w http.ResponseWriter, r *http.Request) {
	gallery, err := ScanGallery()
	if err != nil {
		log.Error().Err(err).Msg("Failed to scan gallery")
		http.Error(w, "Failed to scan gallery", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	catFilter := strings.TrimSpace(strings.ToLower(q.Get("category")))
	folderFilter := strings.TrimSpace(q.Get("folder"))
	searchFilter := strings.TrimSpace(strings.ToLower(q.Get("search")))
	sortParam := strings.TrimSpace(strings.ToLower(q.Get("sort")))

	if catFilter != "" || folderFilter != "" || searchFilter != "" {
		filtered := make([]GalleryItem, 0, len(gallery.Items))
		for _, it := range gallery.Items {
			if catFilter != "" && catFilter != "all" && strings.ToLower(it.Type) != catFilter && strings.ToLower(it.Category) != catFilter {
				continue
			}
			if folderFilter != "" && !strings.EqualFold(it.Channel, folderFilter) && !strings.EqualFold(it.Group, folderFilter) {
				continue
			}
			if searchFilter != "" {
				if !strings.Contains(strings.ToLower(it.Title), searchFilter) &&
					!strings.Contains(strings.ToLower(it.Name), searchFilter) &&
					!strings.Contains(strings.ToLower(it.Channel), searchFilter) &&
					!strings.Contains(strings.ToLower(it.Group), searchFilter) {
					continue
				}
			}
			filtered = append(filtered, it)
		}
		gallery.Items = filtered
		gallery.Total = len(filtered)
	}

	switch sortParam {
	case "oldest":
		sort.Slice(gallery.Items, func(i, j int) bool {
			return gallery.Items[i].ModTime.Before(gallery.Items[j].ModTime)
		})
	case "size_desc", "size":
		sort.Slice(gallery.Items, func(i, j int) bool {
			return gallery.Items[i].SizeInBytes > gallery.Items[j].SizeInBytes
		})
	case "size_asc":
		sort.Slice(gallery.Items, func(i, j int) bool {
			return gallery.Items[i].SizeInBytes < gallery.Items[j].SizeInBytes
		})
	case "name_asc", "name":
		sort.Slice(gallery.Items, func(i, j int) bool {
			return strings.ToLower(gallery.Items[i].Title) < strings.ToLower(gallery.Items[j].Title)
		})
	case "name_desc":
		sort.Slice(gallery.Items, func(i, j int) bool {
			return strings.ToLower(gallery.Items[i].Title) > strings.ToLower(gallery.Items[j].Title)
		})
	case "channel_asc":
		sort.Slice(gallery.Items, func(i, j int) bool {
			ci, cj := strings.ToLower(gallery.Items[i].Channel), strings.ToLower(gallery.Items[j].Channel)
			if ci == cj {
				return strings.ToLower(gallery.Items[i].Title) < strings.ToLower(gallery.Items[j].Title)
			}
			return ci < cj
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(gallery)
}

func deleteGalleryFileInternal(dir, relPath string) error {
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	relPath = strings.TrimPrefix(relPath, "/")
	fullPath := filepath.Join(dir, relPath)

	cleanDir := filepath.Clean(dir)
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, cleanDir) {
		return os.ErrPermission
	}

	// 1. Remove media file
	if err := os.Remove(cleanPath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Msgf("Failed to remove file %s", cleanPath)
	}

	// 2. Remove matching thumbnails across candidate thumbnail folders
	baseName := strings.TrimSuffix(filepath.Base(cleanPath), filepath.Ext(cleanPath))
	dirOfFile := filepath.Dir(cleanPath)
	folderName := filepath.Base(dirOfFile)

	thumbCandidates := []string{
		filepath.Join(dir, CategoryThumbnails, folderName, baseName+".jpg"),
		filepath.Join(dir, CategoryThumbnails, folderName, baseName+".webp"),
		filepath.Join(dir, CategoryThumbnails, folderName, baseName+".png"),
		filepath.Join(dir, CategoryThumbnails, baseName+".jpg"),
		filepath.Join(dir, CategoryThumbnails, baseName+".webp"),
		filepath.Join(dir, CategoryThumbnails, baseName+".png"),
		filepath.Join(dirOfFile, baseName+".jpg"),
		filepath.Join(dirOfFile, baseName+".webp"),
		filepath.Join(dirOfFile, "thumb.jpg"),
		filepath.Join(dirOfFile, "thumb.webp"),
	}
	for _, tc := range thumbCandidates {
		_ = os.Remove(tc)
	}

	// 3. Clean up any task JSON index files in downloads/json/
	jsonDir := filepath.Join(dir, "json")
	if entries, err := os.ReadDir(jsonDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				jPath := filepath.Join(jsonDir, e.Name())
				if data, err := os.ReadFile(jPath); err == nil {
					var idx TaskIndex
					if err := json.Unmarshal(data, &idx); err == nil {
						for _, f := range idx.Files {
							if filepath.Clean(f) == filepath.Clean(relPath) || filepath.Base(f) == filepath.Base(relPath) {
								_ = os.Remove(jPath)
								break
							}
						}
					}
				}
			}
		}
	}

	// Clean up empty thumbnail folder
	thumbDir := filepath.Join(dir, CategoryThumbnails, folderName)
	if entries, err := os.ReadDir(thumbDir); err == nil && len(entries) == 0 {
		_ = os.Remove(thumbDir)
	}

	// Clean up empty channel/media folder
	if folderName != CategoryVideos && folderName != CategoryMusic && folderName != CategoryPhotos && folderName != CategoryThumbnails && folderName != "." {
		if entries, err := os.ReadDir(dirOfFile); err == nil && len(entries) == 0 {
			_ = os.Remove(dirOfFile)
		}
	}

	log.Info().Msgf("Deleted gallery item: %s", relPath)
	return nil
}

// GalleryDeleteHandler deletes a media file and its matching thumbnail from disk.
func GalleryDeleteHandler(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if relPath == "" {
		relPath = strings.TrimSpace(r.URL.Query().Get("file"))
	}
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	relPath = strings.TrimPrefix(relPath, "/")

	if relPath == "" || hasPathTraversal(relPath) {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	dir := getDownloadDir()
	if err := deleteGalleryFileInternal(dir, relPath); err != nil {
		http.Error(w, "Error deleting file", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"deleted": relPath,
	})
}

// GalleryDeleteMultipleHandler deletes multiple media files and their matching thumbnails from disk.
func GalleryDeleteMultipleHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	dir := getDownloadDir()
	deletedCount := 0

	for _, relPath := range body.Paths {
		relPath = strings.TrimSpace(relPath)
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath == "" || hasPathTraversal(relPath) {
			continue
		}
		if err := deleteGalleryFileInternal(dir, relPath); err == nil {
			deletedCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"deletedCount": deletedCount,
	})
}
