package anpan

import (
	"context"
	"strings"
	"testing"
)

func TestParseMagnetName(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:d2474e86c95b19b8bcfdb92bc12c9d44667cfa36&dn=Ubuntu+24.04+LTS+Desktop"
	name := ParseMagnetName(magnet)
	if name != "Ubuntu 24.04 LTS Desktop" {
		t.Fatalf("expected 'Ubuntu 24.04 LTS Desktop', got '%s'", name)
	}

	hashMagnet := "magnet:?xt=urn:btih:d2474e86c95b19b8bcfdb92bc12c9d44667cfa36"
	name2 := ParseMagnetName(hashMagnet)
	if name2 != "Torrent (d2474e86c9)" {
		t.Fatalf("expected 'Torrent (d2474e86c9)', got '%s'", name2)
	}
}

func TestInspectTargetRouter(t *testing.T) {
	ctx := context.Background()

	// 1. Magnet Link
	magnet := "magnet:?xt=urn:btih:1234567890abcdef&dn=TestFile"
	insp, err := InspectTarget(ctx, magnet)
	if err != nil || insp.Type != TargetTorrent {
		t.Fatalf("expected TargetTorrent, got %v (err: %v)", insp, err)
	}

	// 2. Pixiv URL
	pixivURL := "https://www.pixiv.net/en/artworks/12345678"
	if !IsPixivURL(pixivURL) {
		t.Fatalf("expected IsPixivURL to be true")
	}

	// 3. Imgur URL
	imgurURL := "https://imgur.com/a/abc1234"
	if !IsImgurURL(imgurURL) {
		t.Fatalf("expected IsImgurURL to be true")
	}

	// 4. Archive.org URL
	archiveOrgURL := "https://archive.org/details/test-item"
	if !IsArchiveOrgURL(archiveOrgURL) {
		t.Fatalf("expected IsArchiveOrgURL to be true")
	}

	// 5. Kemono / Coomer URL
	kemonoURL := "https://kemono.cr/patreon/user/12345/post/67890"
	if !IsArchivePostURL(kemonoURL) {
		t.Fatalf("expected IsArchivePostURL to be true")
	}

	// 6. Booru URL
	booruURL := "https://yande.re/post/show/123456"
	if !IsBooruURL(booruURL) {
		t.Fatalf("expected IsBooruURL to be true")
	}

	// 7. Cloud Host
	gdrive := "https://drive.google.com/file/d/1a2b3c4d5e/view"
	if !IsCloudHostURL(gdrive) {
		t.Fatalf("expected IsCloudHostURL for Google Drive to be true")
	}

	pixeldrain := "https://pixeldrain.com/u/abc1234"
	if !IsCloudHostURL(pixeldrain) {
		t.Fatalf("expected IsCloudHostURL for Pixeldrain to be true")
	}
}

func TestBuildYtDlpAria2ArgsClampAndLimit(t *testing.T) {
	args := FormatYtDlpAria2Args(32, "50M")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-x 16 -s 16 -k 1M -j 16") {
		t.Fatalf("expected -x clamped to 16, got: %s", joined)
	}
	if !strings.Contains(joined, "--file-allocation=none") {
		t.Fatalf("expected --file-allocation=none, got: %s", joined)
	}
	if !strings.Contains(joined, "--disk-cache=64M") {
		t.Fatalf("expected --disk-cache=64M, got: %s", joined)
	}
	if !strings.Contains(joined, "--max-download-limit=50M") {
		t.Fatalf("expected --max-download-limit=50M, got: %s", joined)
	}

	unlimitedArgs := FormatYtDlpAria2Args(12, "unlimited")
	unlimitedJoined := strings.Join(unlimitedArgs, " ")
	if !strings.Contains(unlimitedJoined, "-x 12 -s 12 -k 1M -j 12") {
		t.Fatalf("expected -x 12, got: %s", unlimitedJoined)
	}
	if strings.Contains(unlimitedJoined, "--max-download-limit") {
		t.Fatalf("did not expect --max-download-limit for unlimited, got: %s", unlimitedJoined)
	}
}
