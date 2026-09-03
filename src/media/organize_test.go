package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOrganizeDownloadedTaskFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("MR_DOWNLOAD_DIR", tempDir)

	taskID := "testtask1234567890abcdef123456"
	rawDir := filepath.Join(tempDir, taskID)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("failed to create raw dir: %v", err)
	}

	// Create dummy video, thumbnail and json
	videoFile := filepath.Join(rawDir, "dummy_video.mp4")
	thumbFile := filepath.Join(rawDir, "dummy_video.jpg")
	jsonFile := filepath.Join(rawDir, "metadata.json")

	_ = os.WriteFile(videoFile, []byte("fake video content"), 0o644)
	_ = os.WriteFile(thumbFile, []byte("fake thumbnail content"), 0o644)
	_ = os.WriteFile(jsonFile, []byte("{}"), 0o644)

	channel := "Thạch Kính Cận — Facebook Videos"
	caption := "Áo xinh form rộng #xh / #thoitrang"

	err := organizeDownloadedTaskFiles(taskID, channel, caption)
	if err != nil {
		t.Fatalf("organizeDownloadedTaskFiles failed: %v", err)
	}

	// Verify target paths:
	// videos/Thạch Kính Cận/Áo xinh form rộng #xh #thoitrang.mp4
	// thumbnails/Thạch Kính Cận/Áo xinh form rộng #xh #thoitrang.jpg
	cleanChan := "Thạch Kính Cận"
	cleanCap := "Áo xinh form rộng #xh #thoitrang"

	expectedVideo := filepath.Join(tempDir, "videos", cleanChan, cleanCap+".mp4")
	expectedThumb := filepath.Join(tempDir, "thumbnails", cleanChan, cleanCap+".jpg")

	if _, err := os.Stat(expectedVideo); os.IsNotExist(err) {
		t.Errorf("expected video at %s, not found", expectedVideo)
	}
	if _, err := os.Stat(expectedThumb); os.IsNotExist(err) {
		t.Errorf("expected thumb at %s, not found", expectedThumb)
	}

	// Verify getAllFilesForId finds both
	medias, err := getAllFilesForId(taskID)
	if err != nil {
		t.Fatalf("getAllFilesForId error: %v", err)
	}
	if len(medias) != 2 {
		t.Fatalf("getAllFilesForId expected 2 files (video + thumb), got %d: %+v", len(medias), medias)
	}

	// Playable video should come first
	if medias[0].Name != cleanCap+".mp4" {
		t.Errorf("expected primary media %s, got %s", cleanCap+".mp4", medias[0].Name)
	}

	// Verify findMediaFile
	filePath, err := findMediaFile(taskID + "/" + cleanCap + ".mp4")
	if err != nil || filePath != expectedVideo {
		t.Errorf("findMediaFile(%s) = %s, err = %v, want %s", taskID+"/"+cleanCap+".mp4", filePath, err, expectedVideo)
	}

	thumbPath, err := findMediaFile(taskID + "/" + cleanCap + ".jpg")
	if err != nil || thumbPath != expectedThumb {
		t.Errorf("findMediaFile(%s) = %s, err = %v, want %s", taskID+"/"+cleanCap+".jpg", thumbPath, err, expectedThumb)
	}

	// Verify deletion
	deleteTaskFiles(taskID, taskID+"/"+cleanCap+".mp4")
	if _, err := os.Stat(expectedVideo); !os.IsNotExist(err) {
		t.Errorf("expected video deleted, but still exists: %s", expectedVideo)
	}
	if _, err := os.Stat(expectedThumb); !os.IsNotExist(err) {
		t.Errorf("expected thumb deleted, but still exists: %s", expectedThumb)
	}
}

func TestOrganizeWithoutChannel(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("MR_DOWNLOAD_DIR", tempDir)

	taskID := "tasknochannel9876543210abcdef12"
	rawDir := filepath.Join(tempDir, taskID)
	_ = os.MkdirAll(rawDir, 0o755)

	videoFile := filepath.Join(rawDir, "sample_clip.mp4")
	thumbFile := filepath.Join(rawDir, "sample_clip.jpg")

	_ = os.WriteFile(videoFile, []byte("sample mp4"), 0o644)
	_ = os.WriteFile(thumbFile, []byte("sample jpg"), 0o644)

	err := organizeDownloadedFiles(taskID)
	if err != nil {
		t.Fatalf("organizeDownloadedFiles failed: %v", err)
	}

	expectedVideo := filepath.Join(tempDir, "videos", "sample_clip.mp4")
	expectedThumb := filepath.Join(tempDir, "thumbnails", "sample_clip.jpg")

	if _, err := os.Stat(expectedVideo); os.IsNotExist(err) {
		t.Errorf("expected video at %s, not found", expectedVideo)
	}
	if _, err := os.Stat(expectedThumb); os.IsNotExist(err) {
		t.Errorf("expected thumb at %s, not found", expectedThumb)
	}

	medias, err := getAllFilesForId(taskID)
	if err != nil || len(medias) != 2 {
		t.Fatalf("getAllFilesForId error: %v, count: %d", err, len(medias))
	}
}
