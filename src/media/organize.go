package media

import (
	"encoding/json"
	"fmt"
	"io"
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
	ID      string   `json:"id"`
	Channel string   `json:"channel,omitempty"`
	Title   string   `json:"title,omitempty"`
	Files   []string `json:"files"` // relative paths from downloadDir, e.g. "videos/ChannelName/Caption.mp4"
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
	return strings.TrimSpace(s)
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
		s = string(runes[:90])
	}
	return strings.TrimSpace(s)
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

// migrateAllLegacy scans the download root for legacy flat hash directories
// and organizes them into typed subfolders. Called once at startup.
func migrateAllLegacy() {
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
		if isCategoryName(name) || name == "json" {
			continue
		}
		if len(name) == 32 {
			_ = organizeDownloadedFiles(name)
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
	candidates := make([]string, 0, 16)
	for _, cat := range knownCategories {
		candidates = append(candidates, filepath.Join(dir, cat, id)+"/")
		candidates = append(candidates, filepath.Join(dir, cat)+"/")
	}
	candidates = append(candidates, filepath.Join(dir, id)+"/")

	// Also check channel subdirectories under categories
	for _, cat := range knownCategories {
		catPath := filepath.Join(dir, cat)
		if entries, err := os.ReadDir(catPath); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					candidates = append(candidates, filepath.Join(catPath, e.Name(), id)+"/")
					candidates = append(candidates, filepath.Join(catPath, e.Name())+"/")
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

	paths := make([]string, 0, 16)

	// 1. Check TaskIndex
	if idx, err := readTaskIndex(dirHash); err == nil && idx != nil {
		for _, rel := range idx.Files {
			if filepath.Base(rel) == filename || filename == "" {
				paths = append(paths, filepath.Join(dir, rel))
			}
		}
	}

	// 2. Direct category candidate paths
	for _, cat := range knownCategories {
		paths = append(paths, filepath.Join(dir, cat, filename))
		paths = append(paths, filepath.Join(dir, cat, dirHash, filename))
		// Check channel subfolders
		catDir := filepath.Join(dir, cat)
		if entries, err := os.ReadDir(catDir); err == nil {
			for _, e := range entries {
				if e.IsDir() && e.Name() != dirHash {
					paths = append(paths, filepath.Join(catDir, e.Name(), filename))
					paths = append(paths, filepath.Join(catDir, e.Name(), dirHash, filename))
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

// organizeDownloadedTaskFiles organizes downloaded media directly into dedicated category folders
// (e.g. videos/<ChannelName>/<Caption>.mp4 and thumbnails/<ChannelName>/<Caption>.jpg)
// without intermediate hash folders, ensuring 1:1 filename matching.
func organizeDownloadedTaskFiles(id, channelName, captionTitle string) error {
	ensureCategoryDirs()
	dir := getDownloadDir()
	legacyPath := filepath.Join(dir, id)

	channelClean := sanitizeFolderName(channelName)
	captionClean := sanitizeCaptionFilename(captionTitle)

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

	taskIdx := &TaskIndex{
		ID:      id,
		Channel: channelClean,
		Title:   captionClean,
		Files:   make([]string, 0),
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
			jsonTarget := filepath.Join(dir, "json", id+".json")
			_ = os.Rename(filepath.Join(sourceDir, oldName), jsonTarget)
			continue
		}

		info, err := e.Info()
		if err != nil || info.Size() == 0 {
			continue
		}

		cat := getCategoryByExt(ext)

		// Target directory: videos/<Channel>/ or thumbnails/<Channel>/
		var targetDir string
		var relDir string
		if channelClean != "" {
			targetDir = filepath.Join(dir, cat, channelClean)
			relDir = filepath.Join(cat, channelClean)
		} else {
			targetDir = filepath.Join(dir, cat)
			relDir = cat
		}
		_ = os.MkdirAll(targetDir, 0o755)

		// Form clean filename based on caption if available
		baseName := captionClean
		if baseName == "" {
			baseName = strings.TrimSuffix(oldName, filepath.Ext(oldName))
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
