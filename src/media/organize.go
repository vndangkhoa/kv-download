package media

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

const (
	CategoryVideos     = "videos"
	CategoryThumbnails = "thumbnails"
	CategoryMusic      = "music"
	CategoryPhotos     = "photos"
	CategoryOther      = "other"
)

var knownCategories = []string{CategoryVideos, CategoryThumbnails, CategoryMusic, CategoryPhotos, CategoryOther}

type TaskIndex struct {
	ID         string   `json:"id"`
	URL        string   `json:"url,omitempty"`
	Platform   string   `json:"platform,omitempty"`
	Channel    string   `json:"channel,omitempty"`
	GroupTitle string   `json:"groupTitle,omitempty"`
	Title      string   `json:"title,omitempty"`
	Duration   string   `json:"duration,omitempty"`
	Files      []string `json:"files"` // relative paths from downloadDir, e.g. "videos/ChannelName/Caption.mp4"
}

func getTaskIndexPath(id string) string {
	return filepath.Join(getDownloadDir(), "json", id+".json")
}

func readTaskIndex(id string) (*TaskIndex, error) {
	p := getTaskIndexPath(id)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var idx TaskIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

func saveTaskIndex(idx *TaskIndex) error {
	_ = os.MkdirAll(filepath.Join(getDownloadDir(), "json"), 0o755)
	p := getTaskIndexPath(idx.ID)
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func isCategoryName(name string) bool {
	for _, cat := range knownCategories {
		if name == cat {
			return true
		}
	}
	return false
}

// sanitizeFolderName removes invalid filesystem characters and trims the channel name.
func sanitizeFolderName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	// Strip " — Facebook Videos", " (TikTok Channel)", etc.
	if idx := strings.Index(s, " — Facebook"); idx != -1 {
		s = s[:idx]
	}
	if idx := strings.Index(s, " (TikTok"); idx != -1 {
		s = s[:idx]
	}
	invalidChars := regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)
	s = invalidChars.ReplaceAllString(s, " ")
	spaceRe := regexp.MustCompile(`\s+`)
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 60 {
		s = string(runes[:60])
	}
	return strings.TrimRight(strings.TrimSpace(s), ". ")
}

// sanitizeCaptionFilename converts a video title/caption into a clean, safe filename.
func sanitizeCaptionFilename(caption string) string {
	s := strings.TrimSpace(caption)
	if s == "" {
		return ""
	}
	invalidChars := regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)
	s = invalidChars.ReplaceAllString(s, " ")
	spaceRe := regexp.MustCompile(`\s+`)
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 90 {
		runes = runes[:90]
	}
	// Also cap UTF-8 byte length to 140 bytes so multi-byte Unicode titles never hit NAME_MAX
	for len(runes) > 0 && len(string(runes)) > 140 {
		runes = runes[:len(runes)-1]
	}
	s = string(runes)
	return strings.TrimRight(strings.TrimSpace(s), ". ")
}

// getCategoryByExt returns the media category folder for a file extension.
func getCategoryByExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	// video
	case ".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".flv", ".wmv", ".mpg", ".mpeg",
		".ts", ".mts", ".m2ts", ".3gp", ".3g2", ".f4v", ".asf", ".m2v", ".mp2":
		return CategoryVideos
	// audio / music
	case ".mp3", ".m4a", ".m4b", ".aac", ".flac", ".wav", ".ogg", ".oga", ".opus",
		".wma", ".aiff", ".alac", ".aif", ".aifc", ".mid", ".mka", ".ac3", ".dts",
		".amr", ".awb", ".ra":
		return CategoryMusic
	// thumbnail / cover image
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".heic", ".heif",
		".bmp", ".tiff", ".tif", ".svg", ".ico", ".psd", ".raw", ".cr2", ".nef", ".arw":
		return CategoryThumbnails
	default:
		return CategoryOther
	}
}

// classifyMediaFile returns category for a filename.
func classifyMediaFile(name string) string {
	return getCategoryByExt(filepath.Ext(name))
}

// ensureCategoryDirs creates videos/thumbnails/music/photos/other subfolders under downloadDir.
func ensureCategoryDirs() {
	dir := getDownloadDir()
	for _, cat := range knownCategories {
		p := filepath.Join(dir, cat)
		_ = os.MkdirAll(p, 0o755)
	}
	_ = os.MkdirAll(filepath.Join(dir, "json"), 0o755)
}

func cleanLegacyUrlFolders() {
	dir := getDownloadDir()
	for _, cat := range knownCategories {
		catPath := filepath.Join(dir, cat)
		entries, err := os.ReadDir(catPath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				// Also check if file itself is named like a URL
				if isUrlLike(e.Name()) {
					ext := filepath.Ext(e.Name())
					base := strings.TrimSuffix(e.Name(), ext)
					if isUrlLike(base) {
						// Clean filename or remove orphan
						log.Info().Msgf("Cleaning url-like file %s in %s", e.Name(), cat)
					}
				}
				continue
			}
			subName := e.Name()
			if isUrlLike(subName) {
				subPath := filepath.Join(catPath, subName)
				subEntries, sErr := os.ReadDir(subPath)
				if sErr == nil {
					fallbackFolder := "Single Videos"
					if cat == CategoryMusic {
						fallbackFolder = "Singles"
					} else if cat == CategoryThumbnails {
						fallbackFolder = "Single Videos"
					}
					targetFolder := filepath.Join(catPath, fallbackFolder)
					_ = os.MkdirAll(targetFolder, 0o755)

					for _, se := range subEntries {
						if se.IsDir() {
							continue
						}
						oldFile := filepath.Join(subPath, se.Name())
						newFileName := se.Name()
						if isUrlLike(newFileName) {
							ext := filepath.Ext(newFileName)
							newFileName = "media" + ext
						}
						newFile := filepath.Join(targetFolder, newFileName)
						_ = os.Rename(oldFile, newFile)
					}
				}
				_ = os.RemoveAll(subPath)
			}
		}
	}
}

// migrateAllLegacy scans the download root for legacy flat hash directories
// and organizes them into typed subfolders. Called once at startup.
func migrateAllLegacy() {
	cleanLegacyUrlFolders()
	dir := getDownloadDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if isCategoryName(name) || name == "json" || strings.HasPrefix(name, ".") {
			continue
		}
		if isHexHashDir(name) {
			subPath := filepath.Join(dir, name)
			subEntries, err := os.ReadDir(subPath)
			if err != nil {
				continue
			}
			hasPlayable := false
			hasActive := false
			for _, se := range subEntries {
				ext := strings.ToLower(filepath.Ext(se.Name()))
				if isVideoExt(ext) || isAudioExt(ext) {
					hasPlayable = true
					break
				}
				if ext == ".part" || ext == ".ytdl" || ext == ".temp" {
					hasActive = true
				}
			}
			if hasPlayable {
				_ = organizeDownloadedFiles(name)
			} else if !hasActive {
				_ = os.RemoveAll(subPath)
			}
		}
	}
}

// getTypedMediaDirectory returns downloadDir/category/id/
func getTypedMediaDirectory(id string, category string) string {
	dir := getDownloadDir()
	if category == "" {
		return dir + id + "/"
	}
	return filepath.Join(dir, category, id) + "/"
}

// resolveMediaDirectory finds the actual directory for a hash id.
func resolveMediaDirectory(id string) string {
	dir := getDownloadDir()
	// Check TaskIndex first
	if idx, err := readTaskIndex(id); err == nil && idx != nil && len(idx.Files) > 0 {
		return filepath.Dir(filepath.Join(dir, idx.Files[0])) + "/"
	}
	candidates := candidateMediaDirs(id)
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
	}
	return filepath.Join(dir, id) + "/"
}

// candidateMediaDirs returns all candidate directories for a hash id.
func candidateMediaDirs(id string) []string {
	dir := getDownloadDir()
	candidates := make([]string, 0, 24)
	for _, cat := range knownCategories {
		candidates = append(candidates, filepath.Join(dir, cat, id)+"/")
		candidates = append(candidates, filepath.Join(dir, cat)+"/")
	}
	candidates = append(candidates, filepath.Join(dir, id)+"/")

	// Also check nested subdirectories under categories (up to 2 levels)
	for _, cat := range knownCategories {
		catPath := filepath.Join(dir, cat)
		if entries, err := os.ReadDir(catPath); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					subPath := filepath.Join(catPath, e.Name())
					candidates = append(candidates, filepath.Join(subPath, id)+"/")
					candidates = append(candidates, subPath+"/")
					if subEntries, err2 := os.ReadDir(subPath); err2 == nil {
						for _, se := range subEntries {
							if se.IsDir() {
								nested := filepath.Join(subPath, se.Name())
								candidates = append(candidates, filepath.Join(nested, id)+"/")
								candidates = append(candidates, nested+"/")
							}
						}
					}
				}
			}
		}
	}
	return candidates
}

// candidateFilePaths returns all candidate full paths for a MediaID (hash/filename).
func candidateFilePaths(mediaID string) []string {
	dir := getDownloadDir()
	dirHash, filename, ok := strings.Cut(mediaID, "/")
	if !ok {
		dirHash = mediaID
		filename = filepath.Base(mediaID)
	}

	paths := make([]string, 0, 24)
	if ok && !hasPathTraversal(mediaID) {
		paths = append(paths, filepath.Join(dir, mediaID))
	}

	// 1. Check TaskIndex
	if idx, err := readTaskIndex(dirHash); err == nil && idx != nil {
		for _, rel := range idx.Files {
			if filepath.Base(rel) == filename || filename == "" {
				paths = append(paths, filepath.Join(dir, rel))
			}
		}
	}

	// 2. Direct category candidate paths
	baseFilename := filepath.Base(filename)
	for _, cat := range knownCategories {
		paths = append(paths, filepath.Join(dir, cat, filename))
		paths = append(paths, filepath.Join(dir, cat, dirHash, filename))
		if baseFilename != filename {
			paths = append(paths, filepath.Join(dir, cat, baseFilename))
		}
		// Check channel & nested subfolders (up to 2 levels)
		catDir := filepath.Join(dir, cat)
		if entries, err := os.ReadDir(catDir); err == nil {
			for _, e := range entries {
				if e.IsDir() && e.Name() != dirHash {
					subDir := filepath.Join(catDir, e.Name())
					paths = append(paths, filepath.Join(subDir, baseFilename))
					paths = append(paths, filepath.Join(subDir, dirHash, baseFilename))
					if subEntries, err2 := os.ReadDir(subDir); err2 == nil {
						for _, se := range subEntries {
							if se.IsDir() && se.Name() != dirHash {
								nested := filepath.Join(subDir, se.Name())
								paths = append(paths, filepath.Join(nested, baseFilename))
								paths = append(paths, filepath.Join(nested, dirHash, baseFilename))
							}
						}
					}
				}
			}
		}
	}

	paths = append(paths, filepath.Join(dir, dirHash, filename))
	return paths
}

// findMediaFile returns the first existing file for a mediaID.
func findMediaFile(mediaID string) (string, error) {
	for _, p := range candidateFilePaths(mediaID) {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// deleteTaskFiles permanently removes all files associated with a task ID from disk.
func deleteTaskFiles(id string, mediaID string) {
	dir := getDownloadDir()
	// 1. Remove all files tracked in task index
	if idx, err := readTaskIndex(id); err == nil && idx != nil {
		for _, rel := range idx.Files {
			full := filepath.Join(dir, rel)
			_ = os.Remove(full)
		}
		_ = os.Remove(getTaskIndexPath(id))
	}

	// 2. Remove by mediaID resolution
	if mediaID != "" {
		if path, err := getFileFromId(mediaID); err == nil && path != "" {
			_ = os.Remove(path)
		}
	}

	// 3. Clean up any legacy directory
	legacyDir := filepath.Join(dir, id)
	_ = os.RemoveAll(legacyDir)
}

// organizeDownloadedFiles inspects the freshly downloaded hash directory and organizes
// video files into videos/ and thumbnails into thumbnails/.
func organizeDownloadedFiles(id string) error {
	return organizeDownloadedTaskFiles(id, "", "")
}

func isUrlLike(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return false
	}
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "http ") ||
		strings.HasPrefix(s, "https ") ||
		strings.HasPrefix(s, "www.") ||
		strings.Contains(s, "youtu.be") ||
		strings.Contains(s, "youtube.com") ||
		strings.Contains(s, "tiktok.com") ||
		strings.Contains(s, "instagram.com") ||
		strings.Contains(s, "facebook.com") ||
		strings.Contains(s, "twitter.com") ||
		strings.Contains(s, "x.com") ||
		strings.Contains(s, "fb.watch") ||
		strings.Contains(s, "si=") ||
		strings.Contains(s, "mibextid=") ||
		strings.Contains(s, "watch?v=")
}

// organizeDownloadedTaskFiles organizes downloaded media directly into dedicated category folders
// (e.g. videos/<ChannelName>/<Caption>.mp4 and thumbnails/<ChannelName>/<Caption>.jpg)
// without intermediate hash folders, ensuring 1:1 filename matching and clean organization.
func organizeDownloadedTaskFiles(id, channelName, captionTitle string) error {
	return organizeDownloadedTaskFilesWithGroup(id, channelName, "", "", captionTitle)
}

func resolveOrganizedRelativeDir(cat, platformClean, channelClean, groupClean string) string {
	mode := getOrganizeMode()

	folderName := channelClean
	if folderName == "" || isUrlLike(folderName) {
		folderName = groupClean
	}
	if folderName == "" || isUrlLike(folderName) {
		if cat == CategoryMusic {
			folderName = "Singles"
		} else {
			folderName = "Single Videos"
		}
	}

	subGroup := ""
	if groupClean != "" && !isUrlLike(groupClean) && !strings.EqualFold(groupClean, folderName) &&
		folderName != "Single Videos" && folderName != "Singles" {
		subGroup = groupClean
	}

	switch mode {
	case "flat_category":
		return cat
	case "channel_category":
		if subGroup != "" {
			return filepath.Join(folderName, subGroup, cat)
		}
		return filepath.Join(folderName, cat)
	case "platform_channel":
		pName := platformClean
		if pName == "" || isUrlLike(pName) {
			pName = "Web"
		}
		if subGroup != "" {
			return filepath.Join(cat, pName, folderName, subGroup)
		}
		return filepath.Join(cat, pName, folderName)
	default: // "category_channel"
		if subGroup != "" {
			return filepath.Join(cat, folderName, subGroup)
		}
		return filepath.Join(cat, folderName)
	}
}

func organizeDownloadedTaskFilesWithGroup(id, channelName, groupTitle, platformName, captionTitle string) error {
	ensureCategoryDirs()
	dir := getDownloadDir()
	legacyPath := filepath.Join(dir, id)

	channelClean := sanitizeFolderName(channelName)
	groupClean := sanitizeFolderName(groupTitle)
	platformClean := sanitizeFolderName(platformName)
	captionClean := sanitizeCaptionFilename(captionTitle)
	if isUrlLike(captionClean) {
		captionClean = ""
	}
	if isUrlLike(channelClean) {
		channelClean = ""
	}
	if isUrlLike(groupClean) {
		groupClean = ""
	}

	// Determine source directory containing the raw downloads
	sourceDir := legacyPath
	if fi, err := os.Stat(sourceDir); err != nil || !fi.IsDir() {
		for _, cand := range candidateMediaDirs(id) {
			if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
				sourceDir = cand
				break
			}
		}
	}

	fi, err := os.Stat(sourceDir)
	if err != nil || !fi.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	var durationStr string
	var webpageURL string

	// Inspect only .json files inside sourceDir and this specific task's JSON files in downloads/json/
	var jsonFilesToRead []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			jsonFilesToRead = append(jsonFilesToRead, filepath.Join(sourceDir, e.Name()))
		}
	}
	jsonFilesToRead = append(jsonFilesToRead,
		filepath.Join(dir, "json", id+".info.json"),
		filepath.Join(dir, "json", id+".json"),
	)

	for _, jPath := range jsonFilesToRead {
		jsonBytes, err := os.ReadFile(jPath)
		if err != nil {
			continue
		}
		var meta struct {
			Title          string `json:"title"`
			Channel        string `json:"channel"`
			Uploader       string `json:"uploader"`
			Artist         string `json:"artist"`
			Creator        string `json:"creator"`
			PlaylistTitle  string `json:"playlist_title"`
			ExtractorKey   string `json:"extractor_key"`
			WebpageURL     string `json:"webpage_url"`
			DurationString string `json:"duration_string"`
		}
		if json.Unmarshal(jsonBytes, &meta) == nil {
			if (captionClean == "" || isUrlLike(captionClean)) && meta.Title != "" && !isUrlLike(meta.Title) {
				captionClean = sanitizeCaptionFilename(meta.Title)
			}
			if channelClean == "" || isUrlLike(channelClean) {
				if meta.Channel != "" && !isUrlLike(meta.Channel) {
					channelClean = sanitizeFolderName(meta.Channel)
				} else if meta.Uploader != "" && !isUrlLike(meta.Uploader) {
					channelClean = sanitizeFolderName(meta.Uploader)
				} else if meta.Artist != "" && !isUrlLike(meta.Artist) {
					channelClean = sanitizeFolderName(meta.Artist)
				} else if meta.Creator != "" && !isUrlLike(meta.Creator) {
					channelClean = sanitizeFolderName(meta.Creator)
				}
			}
			if groupClean == "" && meta.PlaylistTitle != "" && !isUrlLike(meta.PlaylistTitle) {
				groupClean = sanitizeFolderName(meta.PlaylistTitle)
			}
			if platformClean == "" && meta.ExtractorKey != "" {
				platformClean = sanitizeFolderName(meta.ExtractorKey)
			}
			if durationStr == "" && meta.DurationString != "" {
				durationStr = meta.DurationString
			}
			if webpageURL == "" && meta.WebpageURL != "" {
				webpageURL = meta.WebpageURL
			}
		}
	}

	taskIdx := &TaskIndex{
		ID:         id,
		URL:        webpageURL,
		Platform:   platformClean,
		Channel:    channelClean,
		GroupTitle: groupClean,
		Title:      captionClean,
		Duration:   durationStr,
		Files:      make([]string, 0),
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		oldName := e.Name()
		ext := strings.ToLower(filepath.Ext(oldName))
		if ext == ".part" || ext == ".ytdl" || ext == ".temp" {
			continue
		}
		if ext == ".json" {
			targetJsonName := id + ".json"
			if strings.HasSuffix(strings.ToLower(oldName), ".info.json") {
				targetJsonName = id + ".info.json"
			}
			jsonTarget := filepath.Join(dir, "json", targetJsonName)
			_ = os.Rename(filepath.Join(sourceDir, oldName), jsonTarget)
			continue
		}

		info, err := e.Info()
		if err != nil || info.Size() == 0 {
			continue
		}

		cat := getCategoryByExt(ext)
		relDir := resolveOrganizedRelativeDir(cat, platformClean, channelClean, groupClean)
		targetDir := filepath.Join(dir, relDir)
		_ = os.MkdirAll(targetDir, 0o755)

		// Form clean filename based on real video title
		baseName := captionClean
		if baseName == "" || isUrlLike(baseName) {
			rawBase := strings.TrimSuffix(oldName, filepath.Ext(oldName))
			if !isUrlLike(rawBase) {
				baseName = sanitizeCaptionFilename(rawBase)
			}
		}
		if baseName == "" || isUrlLike(baseName) {
			baseName = "media"
		}
		newName := baseName + ext

		srcPath := filepath.Join(sourceDir, oldName)
		dstPath := filepath.Join(targetDir, newName)

		// If destination already exists and is a different file, avoid collision
		if fiDst, err := os.Stat(dstPath); err == nil && !os.SameFile(info, fiDst) {
			if fiDst.Size() != info.Size() {
				newName = fmt.Sprintf("%s_%s%s", baseName, id[:6], ext)
				dstPath = filepath.Join(targetDir, newName)
			}
		}

		if srcPath != dstPath {
			if err := os.Rename(srcPath, dstPath); err != nil {
				if copyErr := copyFile(srcPath, dstPath); copyErr == nil {
					_ = os.Remove(srcPath)
				}
			}
		}

		relPath := filepath.Join(relDir, newName)
		taskIdx.Files = append(taskIdx.Files, relPath)
		if taskIdx.Title == "" || isUrlLike(taskIdx.Title) {
			taskIdx.Title = baseName
		}
		log.Info().Msgf("Organized %s file: %s → %s", cat, oldName, dstPath)
	}

	// Save task index mapping
	if len(taskIdx.Files) > 0 {
		_ = saveTaskIndex(taskIdx)
	}

	// Clean up empty source directory if it was a temporary staging dir
	if sourceDir == legacyPath {
		if rem, err := os.ReadDir(legacyPath); err == nil && len(rem) == 0 {
			_ = os.Remove(legacyPath)
		}
	}

	return nil
}

// TidyAndOrganizeLibrary retroactively organizes legacy staging folders, URL-named folders/files,
// and re-applies the active folder organization mode using queue & TaskIndex metadata.
func TidyAndOrganizeLibrary() (int, error) {
	ensureCategoryDirs()
	migrateAllLegacy()

	dir := getDownloadDir()
	organizedCount := 0

	// Map known mediaId/filename -> task metadata from GlobalQueue
	taskMetaByMedia := make(map[string]*DownloadTask)
	if GlobalQueue != nil {
		for _, t := range GlobalQueue.GetTasks() {
			if t.MediaID != "" {
				taskMetaByMedia[t.MediaID] = t
				taskMetaByMedia[filepath.Base(t.MediaID)] = t
			}
			if t.MediaName != "" {
				taskMetaByMedia[t.MediaName] = t
			}
		}
	}

	// Walk files inside downloadDir (excluding json/)
	type moveJob struct {
		srcPath string
		relPath string
		cat     string
	}
	var jobs []moveJob

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == "json" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".part" || ext == ".ytdl" || ext == ".temp" || ext == ".json" {
			return nil
		}
		cat := getCategoryByExt(ext)
		if cat == CategoryOther {
			return nil
		}
		jobs = append(jobs, moveJob{srcPath: path, relPath: filepath.ToSlash(rel), cat: cat})
		return nil
	})

	for _, job := range jobs {
		fn := filepath.Base(job.srcPath)
		ext := filepath.Ext(fn)
		stem := strings.TrimSuffix(fn, ext)
		parentFolder := filepath.Base(filepath.Dir(job.srcPath))
		if filepath.Clean(filepath.Dir(job.srcPath)) == filepath.Clean(dir) {
			parentFolder = ""
		}

		var channelName, groupTitle, platformName, captionTitle string
		if t, ok := taskMetaByMedia[fn]; ok && t != nil {
			channelName = t.Channel
			if channelName == "" {
				channelName = t.Uploader
			}
			groupTitle = t.GroupTitle
			if groupTitle == "" {
				groupTitle = t.PlaylistTitle
			}
			platformName = t.Platform
			if !isUrlLike(t.Title) {
				captionTitle = t.Title
			}
		}

		if channelName == "" && !isCategoryName(parentFolder) && !isHexHashDir(parentFolder) && !isUrlLike(parentFolder) {
			channelName = parentFolder
		}
		if captionTitle == "" && !isUrlLike(stem) {
			captionTitle = stem
		}

		channelClean := sanitizeFolderName(channelName)
		groupClean := sanitizeFolderName(groupTitle)
		platformClean := sanitizeFolderName(platformName)
		captionClean := sanitizeCaptionFilename(captionTitle)
		if captionClean == "" {
			captionClean = stem
		}

		// Recompute destination path according to current organizeMode
		if channelClean == "" && groupClean == "" && !isUrlLike(parentFolder) && parentFolder != "" && !isCategoryName(parentFolder) && !isHexHashDir(parentFolder) {
			channelClean = sanitizeFolderName(parentFolder)
		}

		targetRelDir := resolveOrganizedRelativeDir(job.cat, platformClean, channelClean, groupClean)
		targetRelPath := filepath.ToSlash(filepath.Join(targetRelDir, captionClean+ext))
		if targetRelPath == job.relPath {
			continue
		}

		dstPath := filepath.Join(dir, targetRelPath)
		_ = os.MkdirAll(filepath.Dir(dstPath), 0o755)
		if _, err := os.Stat(dstPath); err == nil {
			// Avoid overwriting an existing distinct file
			continue
		}
		if err := os.Rename(job.srcPath, dstPath); err != nil {
			if copyErr := copyFile(job.srcPath, dstPath); copyErr == nil {
				_ = os.Remove(job.srcPath)
			} else {
				continue
			}
		}
		organizedCount++
	}

	// Prune empty non-root category subdirectories
	for _, cat := range knownCategories {
		catDir := filepath.Join(dir, cat)
		_ = filepath.WalkDir(catDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() || filepath.Clean(path) == filepath.Clean(catDir) {
				return nil
			}
			if entries, err := os.ReadDir(path); err == nil && len(entries) == 0 {
				_ = os.Remove(path)
			}
			return nil
		})
	}

	return organizedCount, nil
}

// GalleryOrganizeHandler handles POST /api/gallery/organize to tidy & reorganize the media library.
func GalleryOrganizeHandler(w http.ResponseWriter, r *http.Request) {
	count, err := TidyAndOrganizeLibrary()
	if err != nil {
		http.Error(w, "Failed to organize gallery: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"organizedCount": count,
		"organizeMode":   getOrganizeMode(),
	})
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if fi, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, fi.Mode())
	}
	return nil
}
