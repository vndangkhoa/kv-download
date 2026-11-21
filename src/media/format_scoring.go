package media

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
)

// RawFormat maps format properties returned in yt-dlp's dump-json.
type RawFormat struct {
	FormatID       string  `json:"format_id"`
	Ext            string  `json:"ext"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	Height         int     `json:"height"`
	Width          int     `json:"width"`
	ABR            float64 `json:"abr"`
	TBR            float64 `json:"tbr"`
	Filesize       int64   `json:"filesize"`
	FilesizeApprox int64   `json:"filesize_approx"`
	FPS            float64 `json:"fps"`
	FormatNote     string  `json:"format_note"`
}

// FormatChoice represents a clean, scored resolution option with estimated file size.
type FormatChoice struct {
	Label         string  `json:"label"`                   // e.g. "1080p · MP4 · ~77 MB"
	Resolution    string  `json:"resolution"`              // "1080p"
	Height        int     `json:"height"`                  // 1080
	FormatID      string  `json:"formatId"`                // "1080"
	EstimatedSize int64   `json:"estimatedSize,omitempty"` // bytes
	HumanSize     string  `json:"humanSize,omitempty"`     // "77 MB"
	Kind          string  `json:"kind"`                    // "video" | "audio"
	VCodec        string  `json:"vcodec,omitempty"`        // "avc1.640028"
	ACodec        string  `json:"acodec,omitempty"`        // "mp4a.40.2"
	Score         float64 `json:"score,omitempty"`
}

// ScoreVideo scores a video format based on the Yoinks codec scoring model:
// Prioritizes universal hardware compatibility (MP4 container + AVC/H.264 video codec)
// so videos play smoothly on iOS, Safari, Android, Smart TVs and Synology NAS without transcoding.
func ScoreVideo(f RawFormat) float64 {
	score := f.TBR
	if score <= 0 {
		score = f.ABR
	}

	// Major bonus for mp4 container (avoids webm/mkv playback incompatibilities)
	if strings.EqualFold(f.Ext, "mp4") {
		score += 10000
	}

	// Major bonus for H.264 / AVC video codecs (universal hardware decoding)
	vcodec := strings.ToLower(f.VCodec)
	if strings.HasPrefix(vcodec, "avc") || strings.HasPrefix(vcodec, "h264") {
		score += 5000
	}

	return score
}

// BuildFormatChoices extracts clean resolution choices with estimated file sizes
// and hardware-compatible codec priority matching Yoinks.
func BuildFormatChoices(formats []RawFormat) []FormatChoice {
	if len(formats) == 0 {
		return nil
	}

	// 1. Identify audio streams
	var audioOnly []RawFormat
	for _, f := range formats {
		if f.ACodec != "" && f.ACodec != "none" && (f.VCodec == "" || f.VCodec == "none") {
			audioOnly = append(audioOnly, f)
		}
	}

	var bestAudio *RawFormat
	var bestAudioSize int64
	if len(audioOnly) > 0 {
		sort.Slice(audioOnly, func(i, j int) bool {
			rateI := audioOnly[i].ABR
			if rateI <= 0 {
				rateI = audioOnly[i].TBR
			}
			rateJ := audioOnly[j].ABR
			if rateJ <= 0 {
				rateJ = audioOnly[j].TBR
			}
			return rateI > rateJ
		})
		bestAudio = &audioOnly[0]
		bestAudioSize = bestAudio.Filesize
		if bestAudioSize <= 0 {
			bestAudioSize = bestAudio.FilesizeApprox
		}
	}

	// 2. Identify video streams with distinct heights
	var videos []RawFormat
	seenHeights := make(map[int]bool)
	var heights []int
	for _, f := range formats {
		if f.VCodec != "" && f.VCodec != "none" && f.Height > 0 {
			videos = append(videos, f)
			if !seenHeights[f.Height] {
				seenHeights[f.Height] = true
				heights = append(heights, f.Height)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(heights)))

	var choices []FormatChoice
	maxChoices := 8
	if len(heights) < maxChoices {
		maxChoices = len(heights)
	}

	for _, h := range heights[:maxChoices] {
		var candidates []RawFormat
		for _, v := range videos {
			if v.Height == h {
				candidates = append(candidates, v)
			}
		}
		if len(candidates) == 0 {
			continue
		}

		sort.Slice(candidates, func(i, j int) bool {
			return ScoreVideo(candidates[i]) > ScoreVideo(candidates[j])
		})
		best := candidates[0]
		isMuxed := best.ACodec != "" && best.ACodec != "none"

		videoSize := best.Filesize
		if videoSize <= 0 {
			videoSize = best.FilesizeApprox
		}
		totalSize := videoSize
		if !isMuxed && bestAudioSize > 0 {
			totalSize += bestAudioSize
		}

		sizeStr := ""
		if totalSize > 0 {
			sizeStr = humanize.Bytes(uint64(totalSize))
		}

		label := fmt.Sprintf("%dp · MP4", h)
		if sizeStr != "" {
			label += fmt.Sprintf(" · ~%s", sizeStr)
		}

		choices = append(choices, FormatChoice{
			Label:         label,
			Resolution:    fmt.Sprintf("%dp", h),
			Height:        h,
			FormatID:      fmt.Sprintf("%d", h),
			EstimatedSize: totalSize,
			HumanSize:     sizeStr,
			Kind:          "video",
			VCodec:        best.VCodec,
			ACodec:        best.ACodec,
			Score:         ScoreVideo(best),
		})
	}

	// Fallback if no specific height streams exist
	if len(choices) == 0 {
		choices = append(choices, FormatChoice{
			Label:      "Best Available · MP4",
			Resolution: "best",
			Height:     0,
			FormatID:   "highest",
			Kind:       "video",
		})
	}

	// 3. Append Audio Only option (MP3)
	audioLabel := "Audio Only · MP3"
	audioHumanSize := ""
	if bestAudioSize > 0 {
		audioHumanSize = humanize.Bytes(uint64(bestAudioSize))
		audioLabel += fmt.Sprintf(" · ~%s", audioHumanSize)
	}
	choices = append(choices, FormatChoice{
		Label:         audioLabel,
		Resolution:    "Audio",
		Height:        0,
		FormatID:      "audio_mp3",
		EstimatedSize: bestAudioSize,
		HumanSize:     audioHumanSize,
		Kind:          "audio",
		ACodec:        "mp3",
	})

	return choices
}

// DefaultFormatSortArgs returns the yt-dlp format sorting arguments matching Yoinks codec scoring.
// Sorts by:
// 1. res: target resolution has first priority
// 2. vcodec:avc:vp9:av1: AVC (H.264) first for hardware acceleration, then VP9, then AV1
// 3. acodec:m4a:aac:opus: AAC/M4A first for direct MP4 container merge, then Opus
// 4. ext:mp4: MP4 container preferred over WebM/MKV
func DefaultFormatSortArgs() []string {
	return []string{"--format-sort", "res,vcodec:avc:vp9:av1,acodec:m4a:aac:opus,ext:mp4"}
}

// BuildYtDlpFormatSelector converts requested format string into Yoinks-compatible format selector.
func BuildYtDlpFormatSelector(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	switch f {
	case "highest", "best", "max", "source", "8k", "4320", "4320p":
		return "bestvideo+bestaudio/best"
	case "4k", "2160", "2160p", "uhd":
		return "bv*[height=2160]+ba/b[height=2160]/bv*[height<=2160]+ba/b"
	case "1440", "1440p", "2k", "qhd":
		return "bv*[height=1440]+ba/b[height=1440]/bv*[height<=1440]+ba/b"
	case "optimal", "1080", "1080p", "fhd", "high":
		return "bv*[height=1080]+ba/b[height=1080]/bv*[height<=1080]+ba/b"
	case "normal", "720", "720p", "hd":
		return "bv*[height=720]+ba/b[height=720]/bv*[height<=720]+ba/b"
	case "low", "480", "480p", "sd":
		return "bv*[height=480]+ba/b[height=480]/bv*[height<=480]+ba/b"
	case "360", "360p", "saver":
		return "bv*[height=360]+ba/b[height=360]/bv*[height<=360]+ba/b"
	default:
		if strings.Contains(format, "+") || strings.Contains(format, "[") {
			return format
		}
		return "bestvideo+bestaudio/best"
	}
}
