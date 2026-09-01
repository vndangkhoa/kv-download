package anpan

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

var directExtensions = map[string]bool{
	"iso": true, "img": true, "zip": true, "tar": true, "gz": true,
	"bz2": true, "xz": true, "7z": true, "rar": true, "tgz": true,
	"bin": true, "pkg": true, "deb": true, "rpm": true, "appimage": true,
	"exe": true, "dmg": true, "pdf": true, "epub": true, "apk": true,
	"jar": true, "mp4": true, "mkv": true, "avi": true, "mov": true,
	"mp3": true, "flac": true, "wav": true, "m4a": true,
}

func ParseMagnetName(magnetURI string) string {
	u, err := url.Parse(magnetURI)
	if err != nil {
		return "BitTorrent Transfer"
	}
	dn := u.Query().Get("dn")
	if dn != "" {
		return strings.ReplaceAll(dn, "+", " ")
	}
	xt := u.Query().Get("xt")
	if xt != "" {
		parts := strings.Split(xt, ":")
		hash := parts[len(parts)-1]
		if len(hash) > 10 {
			hash = hash[:10]
		}
		return "Torrent (" + hash + ")"
	}
	return "BitTorrent Transfer"
}

func InspectTarget(ctx context.Context, rawInput string) (*TargetInspection, error) {
	trimmed := strings.TrimSpace(rawInput)

	if strings.HasPrefix(trimmed, "magnet:?") {
		return &TargetInspection{
			Type:   TargetTorrent,
			Target: trimmed,
			Name:   ParseMagnetName(trimmed),
		}, nil
	}

	if strings.HasSuffix(trimmed, ".torrent") || strings.Contains(trimmed, ".torrent?") {
		parts := strings.Split(trimmed, "?")
		name := strings.TrimSuffix(path.Base(parts[0]), ".torrent")
		return &TargetInspection{
			Type:   TargetTorrent,
			Target: trimmed,
			Name:   name,
		}, nil
	}

	cleanURL := trimmed

	if IsArchivePostURL(cleanURL) {
		archive, err := ProbeArchivePost(ctx, cleanURL)
		if err == nil && archive != nil && len(archive.Files) > 0 {
			return &TargetInspection{
				Type:        TargetArchive,
				ArchivePost: archive,
			}, nil
		}
	}

	if IsPixivURL(cleanURL) {
		archive, err := ProbePixivPost(ctx, cleanURL)
		if err == nil && archive != nil && len(archive.Files) > 0 {
			return &TargetInspection{
				Type:        TargetArchive,
				ArchivePost: archive,
			}, nil
		}
	}

	if IsBooruURL(cleanURL) {
		archive, err := ProbeBooruPost(ctx, cleanURL)
		if err == nil && archive != nil && len(archive.Files) > 0 {
			return &TargetInspection{
				Type:        TargetArchive,
				ArchivePost: archive,
			}, nil
		}
	}

	if IsPixeldrainListURL(cleanURL) {
		archive, err := ProbePixeldrainList(ctx, cleanURL)
		if err == nil && archive != nil && len(archive.Files) > 0 {
			return &TargetInspection{
				Type:        TargetArchive,
				ArchivePost: archive,
			}, nil
		}
	}

	if IsImgurURL(cleanURL) {
		archive, err := ProbeImgurAlbum(ctx, cleanURL)
		if err == nil && archive != nil && len(archive.Files) > 0 {
			return &TargetInspection{
				Type:        TargetArchive,
				ArchivePost: archive,
			}, nil
		}
	}

	if IsArchiveOrgURL(cleanURL) {
		archive, err := ProbeArchiveOrg(ctx, cleanURL)
		if err == nil && archive != nil && len(archive.Files) > 0 {
			return &TargetInspection{
				Type:        TargetArchive,
				ArchivePost: archive,
			}, nil
		}
	}

	if IsCloudHostURL(cleanURL) {
		cloudFile, err := ProbeCloudHost(ctx, cleanURL)
		if err == nil && cloudFile != nil {
			return &TargetInspection{
				Type:     TargetDirect,
				URL:      cloudFile.URL,
				Filename: cloudFile.Filename,
				Size:     cloudFile.Size,
			}, nil
		}
	}

	u, err := url.Parse(cleanURL)
	if err == nil {
		pathname := strings.ToLower(u.Path)
		ext := strings.TrimPrefix(path.Ext(pathname), ".")
		if directExtensions[ext] {
			return &TargetInspection{
				Type:     TargetDirect,
				URL:      cleanURL,
				Filename: path.Base(pathname),
			}, nil
		}
	}

	// HEAD check for direct downloads
	headCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(headCtx, "HEAD", cleanURL, nil)
	if err == nil {
		client := &http.Client{Timeout: 1500 * time.Millisecond}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				disposition := resp.Header.Get("Content-Disposition")
				contentType := strings.ToLower(resp.Header.Get("Content-Type"))
				lengthHeader := resp.Header.Get("Content-Length")
				var size *int64
				if lengthHeader != "" {
					if s, err := strconv.ParseInt(lengthHeader, 10, 64); err == nil {
						size = &s
					}
				}

				dispositionFilename := ""
				if disposition != "" {
					if _, params, err := mime.ParseMediaType(disposition); err == nil {
						dispositionFilename = params["filename"]
					}
				}

				isAttachment := strings.Contains(disposition, "attachment")
				isBinary := strings.Contains(contentType, "application/octet-stream") ||
					strings.Contains(contentType, "application/x-") ||
					strings.Contains(contentType, "application/zip")

				if (isAttachment || isBinary) && (dispositionFilename != "" || (size != nil && *size > 500_000)) {
					filename := dispositionFilename
					if filename == "" && u != nil {
						filename = path.Base(u.Path)
					}
					if filename == "" {
						filename = "download"
					}
					return &TargetInspection{
						Type:     TargetDirect,
						URL:      cleanURL,
						Filename: filename,
						Size:     size,
					}, nil
				}
			}
		}
	}

	return &TargetInspection{
		Type:     TargetVideo,
		CleanURL: cleanURL,
	}, nil
}
