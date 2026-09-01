package anpan

import "regexp"

type TargetType string

const (
	TargetTorrent TargetType = "torrent"
	TargetDirect  TargetType = "direct"
	TargetArchive TargetType = "archive"
	TargetVideo   TargetType = "video"
)

type ArchiveFile struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Mirrors []string `json:"mirrors,omitempty"`
	Headers []string `json:"headers,omitempty"`
	Size    int64    `json:"size,omitempty"`
}

type ArchivePost struct {
	Title      string        `json:"title"`
	Service    string        `json:"service"`
	User       string        `json:"user,omitempty"`
	ID         string        `json:"id"`
	Files      []ArchiveFile `json:"files"`
	WebpageURL string        `json:"webpage_url"`
}

type CloudDirectFile struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Size     *int64 `json:"size,omitempty"`
}

type TargetInspection struct {
	Type        TargetType       `json:"type"`
	Target      string           `json:"target,omitempty"`
	Name        string           `json:"name,omitempty"`
	URL         string           `json:"url,omitempty"`
	Filename    string           `json:"filename,omitempty"`
	Size        *int64           `json:"size,omitempty"`
	ArchivePost *ArchivePost     `json:"archivePost,omitempty"`
	CleanURL    string           `json:"cleanUrl,omitempty"`
	TimeRange   string           `json:"timeRange,omitempty"`
	TimeLabel   string           `json:"timeLabel,omitempty"`
}

var invalidCharRegex = regexp.MustCompile(`[/\\?%*:|"<>]+`)

func SanitizeFilename(name string) string {
	return invalidCharRegex.ReplaceAllString(name, "_")
}
