package anpan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var archiveOrgRegex = regexp.MustCompile(`(?i)archive\.org/details/([a-zA-Z0-9_\-\.]+)`)

func IsArchiveOrgURL(rawURL string) bool {
	return archiveOrgRegex.MatchString(strings.TrimSpace(rawURL))
}

func ProbeArchiveOrg(ctx context.Context, rawURL string) (*ArchivePost, error) {
	trimmed := strings.TrimSpace(rawURL)
	m := archiveOrgRegex.FindStringSubmatch(trimmed)
	if len(m) < 2 {
		return nil, fmt.Errorf("invalid archive.org details url")
	}
	itemID := m[1]

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 10 * time.Second}
	apiURL := fmt.Sprintf("https://archive.org/metadata/%s", itemID)

	req, err := http.NewRequestWithContext(reqCtx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archive.org metadata request failed: HTTP %d", resp.StatusCode)
	}

	var res struct {
		Server   string `json:"server"`
		Dir      string `json:"dir"`
		Metadata struct {
			Title      string `json:"title"`
			Identifier string `json:"identifier"`
		} `json:"metadata"`
		Files []struct {
			Name   string `json:"name"`
			Format string `json:"format"`
			Source string `json:"source"`
			Size   string `json:"size"`
		} `json:"files"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if len(res.Files) == 0 {
		return nil, fmt.Errorf("no files found in this archive.org item")
	}

	title := SanitizeFilename(res.Metadata.Title)
	if title == "" {
		title = itemID
	}

	var files []ArchiveFile
	for _, f := range res.Files {
		// Ignore internal metadata files
		if f.Format == "Metadata" || f.Format == "Item Tile" || strings.HasSuffix(f.Name, "_meta.xml") || strings.HasSuffix(f.Name, "_files.xml") {
			continue
		}
		cleanName := strings.TrimPrefix(f.Name, "/")
		if cleanName == "" {
			continue
		}

		encodedPath := url.PathEscape(cleanName)
		downloadURL := fmt.Sprintf("https://archive.org/download/%s/%s", itemID, encodedPath)

		var sz int64
		if f.Size != "" {
			sz, _ = strconv.ParseInt(f.Size, 10, 64)
		}

		files = append(files, ArchiveFile{
			Name: SanitizeFilename(cleanName),
			URL:  downloadURL,
			Size: sz,
		})
	}

	if len(files) == 0 {
		for _, f := range res.Files {
			cleanName := strings.TrimPrefix(f.Name, "/")
			if cleanName != "" {
				var sz int64
				if f.Size != "" {
					sz, _ = strconv.ParseInt(f.Size, 10, 64)
				}
				files = append(files, ArchiveFile{
					Name: SanitizeFilename(cleanName),
					URL:  fmt.Sprintf("https://archive.org/download/%s/%s", itemID, url.PathEscape(cleanName)),
					Size: sz,
				})
			}
		}
	}

	return &ArchivePost{
		Title:      title,
		Service:    "archive.org",
		ID:         itemID,
		Files:      files,
		WebpageURL: rawURL,
	}, nil
}
