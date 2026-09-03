package media

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestScanGalleryAndFolders(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("MR_DOWNLOAD_DIR", tempDir)

	vid1 := filepath.Join(tempDir, "videos", "Thạch Kính Cận", "Đầm em mặc.mp4")
	thumb1 := filepath.Join(tempDir, "thumbnails", "Thạch Kính Cận", "Đầm em mặc.jpg")
	vid2 := filepath.Join(tempDir, "videos", "Linhchanh.2k", "Vẫn thiếu người đồng hành.mp4")
	thumb2 := filepath.Join(tempDir, "thumbnails", "Linhchanh.2k", "Vẫn thiếu người đồng hành.jpg")
	vid3 := filepath.Join(tempDir, "videos", "Standalone_Video.mp4")
	thumb3 := filepath.Join(tempDir, "thumbnails", "Standalone_Video.jpg")

	for _, p := range []string{vid1, thumb1, vid2, thumb2, vid3, thumb3} {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("test content"), 0o644)
	}

	resp, err := ScanGallery()
	if err != nil {
		t.Fatalf("ScanGallery failed: %v", err)
	}

	if resp.Total != 3 {
		t.Fatalf("expected 3 items in gallery, got %d: %+v", resp.Total, resp.Items)
	}

	if len(resp.Folders) != 2 {
		t.Fatalf("expected 2 folders (Thạch Kính Cận, Linhchanh.2k), got %d: %+v", len(resp.Folders), resp.Folders)
	}

	// Test GalleryDeleteHandler
	req := httptest.NewRequest(http.MethodDelete, "/api/gallery/file?path=videos/Thạch%20Kính%20Cận/Đầm%20em%20mặc.mp4", nil)
	rec := httptest.NewRecorder()
	GalleryDeleteHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GalleryDeleteHandler status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Verify both video and thumbnail were removed from disk
	if _, err := os.Stat(vid1); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", vid1)
	}
	if _, err := os.Stat(thumb1); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", thumb1)
	}

	// Rescan gallery
	resp2, _ := ScanGallery()
	if resp2.Total != 2 {
		t.Errorf("expected 2 items after delete, got %d", resp2.Total)
	}
}
