package media

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

const (
	CategoryVideos = "videos"
	CategoryMusic  = "music"
	CategoryPhotos = "photos"
	CategoryOther  = "other"
)

var knownCategories = []string{CategoryVideos, CategoryMusic, CategoryPhotos, CategoryOther}

// getCategoryByExt returns the media category folder for a file extension.
// Empty string is never returned — unknown types go to "other".
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
	// photo / image
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".heic", ".heif",
		".bmp", ".tiff", ".tif", ".svg", ".ico", ".psd", ".raw", ".cr2", ".nef", ".arw":
		return CategoryPhotos
	default:
		return CategoryOther
	}
}

// classifyMediaFile returns category for a filename.
func classifyMediaFile(name string) string {
	return getCategoryByExt(filepath.Ext(name))
}

// ensureCategoryDirs creates videos/music/photos/other subfolders under downloadDir.
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
		if name == "json" || name == "videos" || name == "music" || name == "photos" || name == "other" {
			continue
		}
		// hash dirs are 32-char hex (md5). Be lenient: try to organize any dir that looks like hash.
		if len(name) != 32 {
			continue
		}
		_ = organizeDownloadedFiles(name)
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
// Search order: videos, music, photos, other, legacy flat.
func resolveMediaDirectory(id string) string {
	dir := getDownloadDir()
	for _, cat := range knownCategories {
		p := filepath.Join(dir, cat, id)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p + "/"
		}
	}
	return dir + id + "/"
}

// candidateMediaDirs returns all candidate directories for a hash id (typed + legacy).
func candidateMediaDirs(id string) []string {
	dir := getDownloadDir()
	candidates := make([]string, 0, len(knownCategories)+1)
	for _, cat := range knownCategories {
		candidates = append(candidates, filepath.Join(dir, cat, id)+"/")
	}
	candidates = append(candidates, dir+id+"/")
	return candidates
}

// candidateFilePaths returns all candidate full paths for a MediaID (hash/filename).
func candidateFilePaths(mediaID string) []string {
	dirHash, filename, ok := strings.Cut(mediaID, "/")
	if !ok {
		return nil
	}
	// also support future "category/hash/file" style — tolerate both
	if strings.Contains(filename, "/") {
		// e.g. mediaID = "videos/abcd123/file.mp4"
		parts := strings.SplitN(mediaID, "/", 3)
		if len(parts) == 3 {
			// parts[0]=category, parts[1]=hash, parts[2]=filename
			cat := parts[0]
			hash := parts[1]
			file := parts[2]
			dir := getDownloadDir()
			return []string{
				filepath.Join(dir, cat, hash, file),
				filepath.Join(dir, hash, file),
			}
		}
	}
	dir := getDownloadDir()
	paths := make([]string, 0, len(knownCategories)+1)
	for _, cat := range knownCategories {
		paths = append(paths, filepath.Join(dir, cat, dirHash, filename))
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

// organizeDownloadedFiles inspects the freshly downloaded hash directory and moves it
// into the appropriate category subfolder (videos/music/photos/other) if needed.
// It is a no-op if the directory is already organized or does not exist.
func organizeDownloadedFiles(id string) error {
	ensureCategoryDirs()
	dir := getDownloadDir()
	legacyPath := filepath.Join(dir, id)
	// if already in a typed location, clean up duplicate legacy if present
	for _, cat := range knownCategories {
		typedPath := filepath.Join(dir, cat, id)
		if fi, err := os.Stat(typedPath); err == nil && fi.IsDir() {
			if _, err := os.Stat(legacyPath); err == nil {
				// legacy duplicate exists (e.g. re-download of same URL) — remove it to avoid clutter
				_ = os.RemoveAll(legacyPath)
				log.Info().Msgf("Organized download %s already at %s/, removed duplicate legacy %s", id, cat, legacyPath)
			}
			return nil
		}
	}
	fi, err := os.Stat(legacyPath)
	if err != nil || !fi.IsDir() {
		return nil // nothing to organize
	}
	entries, err := os.ReadDir(legacyPath)
	if err != nil {
		return err
	}
	// determine primary media file (largest playable, similar to getAllFilesForId sorting)
	var primary string
	var primarySize int64 = -1
	isPlayable := func(name string) int {
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".flv", ".wmv", ".mpg", ".mpeg", ".ts", ".mts", ".m2ts", ".3gp":
			return 1
		case ".mp3", ".m4a", ".aac", ".flac", ".wav", ".ogg", ".oga", ".opus", ".wma", ".aiff":
			return 2
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".heic", ".heif", ".bmp", ".tiff":
			return 3
		default:
			// skip non-media for primary decision (json, thumb, part...)
			if ext == ".json" || ext == ".part" || ext == ".ytdl" || ext == ".temp" || ext == ".jpg" {
				// thumbnails are .jpg but we treat them as secondary
				// still consider if no other file
				return 4
			}
			return 5
		}
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".json" || ext == ".part" || ext == ".ytdl" || ext == ".temp" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		size := info.Size()
		if size == 0 {
			continue
		}
		prio := isPlayable(name)
		if prio >= 4 && primary != "" {
			continue // prefer playable over thumb/other
		}
		if primary == "" || prio < isPlayable(primary) || (prio == isPlayable(primary) && size > primarySize) {
			primary = name
			primarySize = size
		}
	}
	if primary == "" {
		// no media found, keep as is (maybe only thumbnails failed)
		return nil
	}
	cat := classifyMediaFile(primary)
	targetParent := filepath.Join(dir, cat)
	_ = os.MkdirAll(targetParent, 0o755)
	targetPath := filepath.Join(targetParent, id)

	if _, err := os.Stat(targetPath); err == nil {
		// already exists (race) — merge or skip
		return nil
	}
	// try atomic rename (same filesystem)
	if err := os.Rename(legacyPath, targetPath); err == nil {
		log.Info().Msgf("Organized download %s → %s/ (%s)", id, cat, primary)
		return nil
	}
	// cross-device fallback: copy recursively then remove
	if err := copyDir(legacyPath, targetPath); err != nil {
		log.Error().Err(err).Msgf("Failed to organize %s to %s", id, cat)
		return err
	}
	_ = os.RemoveAll(legacyPath)
	log.Info().Msgf("Organized download %s → %s/ (%s) [copied]", id, cat, primary)
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
