package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kv-download/src/anpan"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type TaskStatus string

const (
	StatusQueued      TaskStatus = "queued"
	StatusDownloading TaskStatus = "downloading"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
	StatusCancelled   TaskStatus = "cancelled"
)

type DownloadTask struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	Thumbnail     string     `json:"thumbnail"`
	Format        string     `json:"format"`
	Cookies       string     `json:"cookies,omitempty"`
	RateLimit     string     `json:"rateLimit,omitempty"`
	Status        TaskStatus `json:"status"`
	Percent       float64    `json:"percent"`
	Speed         string     `json:"speed"`
	ETA           string     `json:"eta"`
	Downloaded    int64      `json:"downloaded"`
	TotalBytes    int64      `json:"totalBytes"`
	HumanSize     string     `json:"humanSize"`
	ErrorMessage  string     `json:"errorMessage"`
	MediaID       string     `json:"mediaId"`
	MediaName     string     `json:"mediaName"`
	PlaylistTitle string     `json:"playlistTitle,omitempty"`
	Channel       string     `json:"channel,omitempty"`
	Uploader      string     `json:"uploader,omitempty"`
	GroupID       string     `json:"groupId,omitempty"`
	GroupTitle    string     `json:"groupTitle,omitempty"`
	GroupType     string     `json:"groupType,omitempty"`
	Platform      string     `json:"platform,omitempty"`
	Duration      string     `json:"duration,omitempty"`
	InfoJSONPath  string     `json:"infoJsonPath,omitempty"`
	Referer       string     `json:"referer,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`

	cancelFunc context.CancelFunc `json:"-"`
}

type QueueManager struct {
	mu            sync.RWMutex
	tasks         map[string]*DownloadTask
	queue         []string
	maxWorkers    int
	activeWorkers int
}

var GlobalQueue *QueueManager

func init() {
	defaultWorkers := 3
	if env := strings.TrimSpace(os.Getenv("MR_MAX_WORKERS")); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			defaultWorkers = n
		}
	}
	GlobalQueue = NewQueueManager(defaultWorkers)
	GlobalQueue.LoadState()
}

func NewQueueManager(maxWorkers int) *QueueManager {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	return &QueueManager{
		tasks:      make(map[string]*DownloadTask),
		queue:      make([]string, 0),
		maxWorkers: maxWorkers,
	}
}

func (q *QueueManager) Add(url string, format string) *DownloadTask {
	return q.AddWithCookies(url, format, "")
}

func (q *QueueManager) AddWithCookies(url string, format string, cookies string) *DownloadTask {
	return q.AddFull(url, format, cookies, "")
}

func (q *QueueManager) AddFull(url string, format string, cookies string, rateLimit string) *DownloadTask {
	return q.AddFullWithMeta(url, format, cookies, rateLimit, "", "", "", "", "")
}

func detectPlatformName(rawURL string) string {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.Contains(u, "youtube.com") || strings.Contains(u, "youtu.be"):
		return "YouTube"
	case strings.Contains(u, "tiktok.com"):
		return "TikTok"
	case strings.Contains(u, "facebook.com") || strings.Contains(u, "fb.watch") || strings.Contains(u, "fb.com"):
		return "Facebook"
	case strings.Contains(u, "instagram.com"):
		return "Instagram"
	case strings.Contains(u, "twitter.com") || strings.Contains(u, "x.com"):
		return "X / Twitter"
	case strings.Contains(u, "soundcloud.com"):
		return "SoundCloud"
	case strings.Contains(u, "bilibili.com"):
		return "Bilibili"
	case strings.Contains(u, "vimeo.com"):
		return "Vimeo"
	case strings.Contains(u, "av01.media") || strings.Contains(u, "av01.tv") || strings.Contains(u, "iw01.xyz"):
		return "AV01"
	case strings.Contains(u, ".m3u8") || strings.Contains(u, "/m3u8"):
		return "HLS Stream"
	default:
		if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
			host := strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
			return host
		}
		return "Direct Link"
	}
}

func isChannelOrPlaylistURL(rawURL string) (bool, string) {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	if u == "" {
		return false, ""
	}
	if strings.Contains(u, "tiktok.com/@") && !strings.Contains(u, "/video/") && !strings.Contains(u, "/photo/") {
		return true, "channel"
	}
	if strings.Contains(u, "youtube.com/@") ||
		strings.Contains(u, "youtube.com/channel/") ||
		strings.Contains(u, "youtube.com/c/") ||
		strings.Contains(u, "youtube.com/user/") ||
		strings.Contains(u, "youtu.be/@") ||
		strings.Contains(u, "space.bilibili.com/") {
		return true, "channel"
	}
	if strings.Contains(u, "list=") ||
		strings.Contains(u, "/playlist") ||
		strings.Contains(u, "/mix") ||
		strings.Contains(u, "/radio") ||
		(strings.Contains(u, "soundcloud.com/") && (strings.Contains(u, "/sets/") || strings.Contains(u, "/albums/"))) ||
		strings.Contains(u, "bilibili.com/medialist") ||
		strings.Contains(u, "imgur.com/a/") ||
		strings.Contains(u, "imgur.com/gallery/") ||
		strings.Contains(u, "archive.org/details/") ||
		strings.Contains(u, "pixeldrain.com/l/") {
		return true, "playlist"
	}
	return false, ""
}

func formatDurationFloat(sec float64) string {
	if sec <= 0 {
		return ""
	}
	total := int(sec)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return strconv.Itoa(h) + ":" + pad2(m) + ":" + pad2(s)
	}
	return strconv.Itoa(m) + ":" + pad2(s)
}

func pad2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func (q *QueueManager) AddFullWithMeta(url string, format string, cookies string, rateLimit string, title string, thumbnail string, playlistTitle string, channel string, uploader string) *DownloadTask {
	return q.AddTaskWithGroup(url, format, cookies, rateLimit, title, thumbnail, playlistTitle, channel, uploader, "", "", "", "")
}

func (q *QueueManager) AddTaskWithGroup(rawURL, format, cookies, rateLimit, title, thumbnail, playlistTitle, channel, uploader, groupID, groupTitle, groupType, duration string) *DownloadTask {
	q.mu.Lock()

	if format == "" {
		format = "highest"
	}

	initialThumb := thumbnail
	if initialThumb == "" {
		ytRegex := regexp.MustCompile(`(?:youtu\.be/|youtube\.com/(?:watch\?v=|shorts/|embed/))([a-zA-Z0-9_-]{11})`)
		if m := ytRegex.FindStringSubmatch(rawURL); len(m) > 1 {
			initialThumb = "https://i.ytimg.com/vi/" + m[1] + "/hqdefault.jpg"
		}
	}

	taskTitle := strings.TrimSpace(title)
	if taskTitle == "" {
		taskTitle = shortUrl(rawURL)
	}

	infoPath := ""
	if p, ok := GetCachedInfoJSON(rawURL); ok {
		infoPath = p
	}

	if groupID == "" && playlistTitle != "" {
		groupID = "pl_" + GetMD5Hash(playlistTitle, nil)[:10]
		groupTitle = playlistTitle
		if groupType == "" {
			groupType = "playlist"
		}
	}

	task := &DownloadTask{
		ID:            uuid.New().String(),
		URL:           rawURL,
		Title:         taskTitle,
		Thumbnail:     initialThumb,
		Format:        format,
		Cookies:       cookies,
		RateLimit:     rateLimit,
		PlaylistTitle: playlistTitle,
		Channel:       channel,
		Uploader:      uploader,
		GroupID:       groupID,
		GroupTitle:    groupTitle,
		GroupType:     groupType,
		Platform:      detectPlatformName(rawURL),
		Duration:      duration,
		InfoJSONPath:  infoPath,
		Status:        StatusQueued,
		CreatedAt:     time.Now(),
	}

	q.tasks[task.ID] = task
	q.queue = append(q.queue, task.ID)

	log.Info().Msgf("Enqueued task %s for URL %s (title: %s, group: %s [%s], channel: %s, format: %s)", task.ID, rawURL, taskTitle, groupTitle, groupID, channel, format)

	q.saveStateLocked()
	q.dispatchLocked()
	needsMeta := (isUrlLike(task.Title) || task.Title == shortUrl(rawURL)) && strings.HasPrefix(rawURL, "http")
	q.mu.Unlock()

	q.broadcastTaskUpdate(task)

	if needsMeta {
		go q.resolveTaskMetadataAsync(task.ID)
	}

	return task
}

func (q *QueueManager) resolveTaskMetadataAsync(taskID string) {
	q.mu.RLock()
	t, exists := q.tasks[taskID]
	if !exists || t == nil {
		q.mu.RUnlock()
		return
	}
	targetURL := t.URL
	cookies := t.Cookies
	q.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	if _, _, ok := ParseAV01VideoURL(targetURL); ok {
		if av, err := ResolveAV01Video(ctx, targetURL); err == nil && av != nil {
			q.mu.Lock()
			if task, exists := q.tasks[taskID]; exists && task != nil {
				changed := false
				if av.Title != "" && (isUrlLike(task.Title) || task.Title == shortUrl(task.URL)) {
					task.Title = av.Title
					changed = true
				}
				if task.Channel == "" {
					task.Channel = "AV01"
					changed = true
				}
				if task.Platform == "" || task.Platform == "HLS Stream" {
					task.Platform = "AV01"
					changed = true
				}
				if task.Thumbnail == "" && av.Thumbnail != "" {
					task.Thumbnail = av.Thumbnail
					changed = true
				}
				if changed {
					q.saveStateLocked()
					q.mu.Unlock()
					q.broadcastTaskUpdate(task)
					return
				}
			}
			q.mu.Unlock()
			return
		}
	}

	args := []string{"--dump-json", "--no-playlist", "--no-warnings", "--no-check-certificates", "--skip-download"}
	if cookies != "" {
		if tmpCookie, cleanup, err := CreateEphemeralCookieFile(cookies, targetURL); err == nil && tmpCookie != "" {
			defer cleanup()
			args = append(args, "--cookies", tmpCookie)
		}
	} else {
		if workingCookies := getWorkingCookiesPath(getCookiesPath()); workingCookies != "" {
			args = append(args, "--cookies", workingCookies)
		}
	}
	args = append(args, targetURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return
	}

	var meta struct {
		Title          string  `json:"title"`
		Channel        string  `json:"channel"`
		Uploader       string  `json:"uploader"`
		Thumbnail      string  `json:"thumbnail"`
		Duration       float64 `json:"duration"`
		DurationString string  `json:"duration_string"`
	}
	if json.Unmarshal(out, &meta) != nil {
		return
	}

	q.mu.Lock()
	task, ok := q.tasks[taskID]
	if !ok || task == nil {
		q.mu.Unlock()
		return
	}
	changed := false
	if meta.Title != "" && !isUrlLike(meta.Title) && (isUrlLike(task.Title) || task.Title == shortUrl(task.URL)) {
		task.Title = meta.Title
		changed = true
	}
	if task.Channel == "" {
		if meta.Channel != "" {
			task.Channel = meta.Channel
			changed = true
		} else if meta.Uploader != "" {
			task.Channel = meta.Uploader
			changed = true
		}
	}
	if task.Thumbnail == "" && meta.Thumbnail != "" {
		task.Thumbnail = meta.Thumbnail
		changed = true
	}
	if task.Duration == "" {
		if meta.DurationString != "" {
			task.Duration = meta.DurationString
			changed = true
		} else if meta.Duration > 0 {
			task.Duration = formatDurationFloat(meta.Duration)
			changed = true
		}
	}
	if changed {
		q.saveStateLocked()
	}
	q.mu.Unlock()

	if changed {
		q.broadcastTaskUpdate(task)
	}
}

func (q *QueueManager) Cancel(id string) bool {
	q.mu.Lock()
	task, exists := q.tasks[id]
	if !exists {
		q.mu.Unlock()
		return false
	}

	if task.Status == StatusQueued {
		task.Status = StatusCancelled
		now := time.Now()
		task.CompletedAt = &now
		q.removeFromQueueLocked(id)
		q.saveStateLocked()
		q.mu.Unlock()
		q.broadcastTaskUpdate(task)
		return true
	}

	if task.Status == StatusDownloading && task.cancelFunc != nil {
		task.Status = StatusCancelled
		task.cancelFunc()
		now := time.Now()
		task.CompletedAt = &now
		q.saveStateLocked()
		q.mu.Unlock()
		q.broadcastTaskUpdate(task)
		return true
	}

	q.mu.Unlock()
	return false
}

func (q *QueueManager) Retry(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[id]
	if !exists {
		return false
	}

	if task.Status == StatusFailed || task.Status == StatusCancelled {
		task.Status = StatusQueued
		task.Percent = 0
		task.Speed = ""
		task.ETA = ""
		task.ErrorMessage = ""
		task.CompletedAt = nil
		q.queue = append(q.queue, id)

		q.saveStateLocked()
		q.broadcastTaskUpdate(task)

		q.dispatchLocked()
		return true
	}
	return false
}

func (q *QueueManager) Delete(id string, deleteFile bool) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[id]
	if !exists {
		return false
	}

	if task.cancelFunc != nil {
		task.cancelFunc()
	}

	q.removeFromQueueLocked(id)

	if deleteFile {
		deleteTaskFiles(id, task.MediaID)
	}

	delete(q.tasks, id)
	q.saveStateLocked()

	GlobalBroadcaster.Broadcast(map[string]interface{}{
		"type":   "task_deleted",
		"taskId": id,
	})

	return true
}

func (q *QueueManager) ClearCompleted() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for id, t := range q.tasks {
		if t.Status == StatusCompleted {
			delete(q.tasks, id)
			count++
		}
	}
	if count > 0 {
		q.saveStateLocked()
		GlobalBroadcaster.Broadcast(map[string]interface{}{
			"type":   "init",
			"tasks":  q.getTasksLocked(),
		})
	}
	return count
}

func (q *QueueManager) RetryAllFailed() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for id, t := range q.tasks {
		if t.Status == StatusFailed || t.Status == StatusCancelled {
			t.Status = StatusQueued
			t.Percent = 0
			t.Speed = ""
			t.ETA = ""
			t.ErrorMessage = ""
			t.CompletedAt = nil
			q.queue = append(q.queue, id)
			q.broadcastTaskUpdate(t)
			count++
		}
	}
	if count > 0 {
		q.saveStateLocked()
		q.dispatchLocked()
	}
	return count
}

func (q *QueueManager) getTasksLocked() []*DownloadTask {
	list := make([]*DownloadTask, 0, len(q.tasks))
	for _, t := range q.tasks {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

func (q *QueueManager) GetTasks() []*DownloadTask {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.getTasksLocked()
}

func (q *QueueManager) SetMaxWorkers(n int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	q.maxWorkers = n
	log.Info().Msgf("Queue max concurrent workers set to %d", n)
	q.dispatchLocked()
}

func (q *QueueManager) GetMaxWorkers() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.maxWorkers
}

func (q *QueueManager) GetActiveWorkers() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.activeWorkers
}

func (q *QueueManager) dispatchLocked() {
	for q.activeWorkers < q.maxWorkers && len(q.queue) > 0 {
		var nextTask *DownloadTask
		var nextIdx = -1
		for i, id := range q.queue {
			if t, ok := q.tasks[id]; ok && t.Status == StatusQueued {
				nextTask = t
				nextIdx = i
				break
			}
		}
		if nextTask == nil || nextIdx == -1 {
			break
		}

		q.queue = append(q.queue[:nextIdx], q.queue[nextIdx+1:]...)

		q.activeWorkers++
		ctx, cancel := context.WithCancel(context.Background())
		nextTask.cancelFunc = cancel
		nextTask.Status = StatusDownloading
		q.saveStateLocked()
		q.broadcastTaskUpdate(nextTask)

		go func(t *DownloadTask, c context.Context) {
			defer func() {
				q.mu.Lock()
				q.activeWorkers--
				q.dispatchLocked()
				q.mu.Unlock()
			}()
			q.processTask(c, t)
		}(nextTask, ctx)
	}
}

func isAudioFormat(format string) bool {
	f := strings.ToLower(strings.TrimSpace(format))
	return strings.HasPrefix(f, "audio_") || f == "mp3" || f == "m4a" || f == "flac" || f == "wav" || f == "aac" || f == "opus" || f == "ogg"
}

func (q *QueueManager) processTask(ctx context.Context, task *DownloadTask) {
	defer func() {
		if task.cancelFunc != nil {
			task.cancelFunc()
			task.cancelFunc = nil
		}
	}()

	dirID := GetMD5Hash(task.URL, map[string]string{"format": task.Format})
	ensureCategoryDirs()
	targetDir := getMediaDirectory(dirID)
	_ = os.MkdirAll(targetDir, 0o755)

	// ── 1. Anpan Target Routing (Torrents & Direct Cloud Accelerated Downloads) ──
	target, _ := anpan.InspectTarget(ctx, task.URL)
	if target != nil {
		if target.Type == anpan.TargetTorrent && anpan.HasAria2c() {
			log.Info().Msgf("Routing torrent task %s via aria2c", task.ID)
			err := anpan.DownloadTorrentAria(ctx, task.URL, targetDir)
			if err != nil && ctx.Err() != context.Canceled {
				q.failTask(task, err.Error())
				return
			}
			ch := task.Channel
			if ch == "" {
				ch = task.Uploader
			}
			if ch == "" {
				ch = task.PlaylistTitle
			}
			grpTitle := task.GroupTitle
			if grpTitle == "" {
				grpTitle = task.PlaylistTitle
			}
			_ = organizeDownloadedTaskFilesWithGroup(dirID, ch, grpTitle, task.Platform, task.Title)
			medias, _ := getAllFilesForId(dirID)
			if len(medias) > 0 {
				q.completeTask(task, medias[0], dirID)
				return
			}
		}

		if target.Type == anpan.TargetDirect && target.URL != "" && anpan.HasAria2c() {
			fn := target.Filename
			if fn == "" {
				fn = "download"
			}
			log.Info().Msgf("Routing direct file %s via aria2c", target.URL)
			err := anpan.DownloadDirectFileAria(ctx, target.URL, targetDir, fn, 16)
			if err == nil {
				ch := task.Channel
				if ch == "" {
					ch = task.Uploader
				}
				if ch == "" {
					ch = task.PlaylistTitle
				}
				grpTitle := task.GroupTitle
				if grpTitle == "" {
					grpTitle = task.PlaylistTitle
				}
				_ = organizeDownloadedTaskFilesWithGroup(dirID, ch, grpTitle, task.Platform, task.Title)
				medias, _ := getAllFilesForId(dirID)
				if len(medias) > 0 {
					q.completeTask(task, medias[0], dirID)
					return
				}
			}
		}
	}

	outputTemplate := targetDir + "%(title,description,uploader_id,id).100B.%(ext)s"

	var lastErr string
	maxAttempts := 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() == context.Canceled {
			_ = os.RemoveAll(targetDir)
			q.mu.Lock()
			task.Status = StatusCancelled
			now := time.Now()
			task.CompletedAt = &now
			q.saveStateLocked()
			q.mu.Unlock()
			q.broadcastTaskUpdate(task)
			return
		}

		isAudio := isAudioFormat(task.Format)

		targetURL := strings.TrimSpace(task.URL)
		effReferer := strings.TrimSpace(task.Referer)
		avTitle := ""
		if _, _, ok := ParseAV01VideoURL(targetURL); ok {
			avCtx, avCancel := context.WithTimeout(ctx, 20*time.Second)
			av, avErr := ResolveAV01Video(avCtx, targetURL)
			avCancel()
			if avErr == nil && av != nil && len(av.Variants) > 0 {
				chosen := SelectAV01Variant(av.Variants, task.Format)
				if chosen.URL != "" {
					targetURL = chosen.URL
					effReferer = av.Referer
					avTitle = av.Title
					q.mu.Lock()
					changed := false
					if av.Title != "" && (task.Title == "" || isUrlLike(task.Title) || task.Title == shortUrl(task.URL) || strings.Contains(task.Title, "master.m3u8")) {
						task.Title = av.Title
						changed = true
					}
					if task.Channel == "" {
						task.Channel = "AV01"
						changed = true
					}
					if task.Platform == "" || task.Platform == "HLS Stream" {
						task.Platform = "AV01"
						changed = true
					}
					if task.Thumbnail == "" && av.Thumbnail != "" {
						task.Thumbnail = av.Thumbnail
						changed = true
					}
					if changed {
						q.saveStateLocked()
					}
					q.mu.Unlock()
					if changed {
						q.broadcastTaskUpdate(task)
					}
				}
			}
		} else if IsFacebookShareURL(targetURL) {
			if resolved := resolveFbShareURL(targetURL, task.Cookies); resolved != "" && !IsFacebookShareURL(resolved) {
				if canon := fbCanonicalVideoUrl(resolved); canon != "" {
					resolved = canon
				}
				targetURL = resolved
			}
		} else if matched, _ := regexp.MatchString(`^\d{6,}$`, targetURL); matched {
			targetURL = "https://www.facebook.com/watch/?v=" + targetURL
		}

		if effReferer == "" && strings.Contains(strings.ToLower(targetURL), "iw01.xyz") {
			effReferer = av01Referer
		}
		isStreamURL := isHLSOrDashURL(targetURL)

		outTmpl := outputTemplate
		if avTitle != "" {
			safeStem := sanitizeCaptionFilename(avTitle)
			if safeStem == "" {
				safeStem = task.ID
			}
			outTmpl = targetDir + safeStem + ".%(ext)s"
		}

		args := []string{
			"--newline",
			"--progress-template", "%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s|%(progress.downloaded_bytes)s|%(progress.total_bytes)s",
			"--no-playlist",
			"--write-info-json",
			"--write-thumbnail",
			"--convert-thumbnails", "jpg",
			"--output", outTmpl,
			"--no-check-certificates",
			"--extractor-args", "instagram:image_persist=1;threads:app_version=30.0.0",
		}

		if !isAudio {
			args = append(args, "--remux-video", "mp4", "--merge-output-format", "mp4")
			// Yoinks codec scoring: prioritize AVC/H.264 over VP9/AV1, AAC over Opus, MP4 container
			args = append(args, DefaultFormatSortArgs()...)
		}

		// Attempt 1: try aria2c if available (skip aria2c for HLS/DASH streams where fragment handling breaks).
		if attempt == 1 && !isStreamURL && anpan.HasAria2c() && !isAudio && !strings.Contains(task.URL, "tiktok.com") {
			if ariaArgs := anpan.BuildYtDlpAria2Args(16); ariaArgs != nil {
				args = append(args, ariaArgs...)
			}
		}
		if isStreamURL {
			args = append(args, "--concurrent-fragments", "8")
			if attempt == 1 {
				args = append(args, "--hls-prefer-native")
			}
		}
		if effReferer != "" {
			args = append(args, "--referer", effReferer)
			if effOrigin := originFromURL(effReferer); effOrigin != "" {
				args = append(args, "--add-header", "Origin:"+effOrigin)
			}
		}

		// Apply Format Selection (Highest / 8K / 4K / 2K / 1080p / 720p / 480p / 360p / Audio)
		if isAudio {
			switch strings.ToLower(strings.TrimSpace(task.Format)) {
			case "audio_mp3", "mp3":
				args = append(args, "--extract-audio", "--audio-format", "mp3", "--audio-quality", "0")
			case "audio_m4a", "m4a", "aac":
				args = append(args, "--extract-audio", "--audio-format", "m4a", "--audio-quality", "0")
			case "audio_flac", "flac":
				args = append(args, "--extract-audio", "--audio-format", "flac", "--audio-quality", "0")
			case "audio_wav", "wav":
				args = append(args, "--extract-audio", "--audio-format", "wav", "--audio-quality", "0")
			default:
				args = append(args, "--extract-audio", "--audio-format", "mp3", "--audio-quality", "0")
			}
		} else if isStreamURL && avTitle != "" {
			// Variant playlist already selected for av01.media
			args = append(args, "--format", "bv*+ba/b/best")
		} else {
			args = append(args, "--format", BuildYtDlpFormatSelector(task.Format))
		}

		if impersonate := strings.TrimSpace(os.Getenv("MR_IMPERSONATE")); impersonate != "" {
			args = append(args, "--impersonate", impersonate)
		}

		rateLimit := strings.TrimSpace(task.RateLimit)
		if rateLimit == "" {
			rateLimit = strings.TrimSpace(os.Getenv("MR_RATE_LIMIT"))
		}
		if rateLimit != "" && rateLimit != "unlimited" && rateLimit != "0" {
			args = append(args, "--limit-rate", rateLimit)
		}

		if task.Cookies != "" {
			tmpCookie, cleanup, err := CreateEphemeralCookieFile(task.Cookies, task.URL)
			if err == nil && tmpCookie != "" {
				defer cleanup()
				args = append(args, "--cookies", tmpCookie)
			}
		} else {
			cookiesPath := getCookiesPath()
			if workingCookies := getWorkingCookiesPath(cookiesPath); workingCookies != "" {
				args = append(args, "--cookies", workingCookies)
			}
		}

		for arg, value := range getEnvVars() {
			args = append(args, arg, value)
		}

		// Fast start: on attempt 1, use cached info-json to skip redundant extractor network requests.
		// If attempt 1 fails (e.g. pre-signed CDN URL expired), subsequent attempts fall back to live URL.
		usedInfoJSON := false
		infoJSONPath := task.InfoJSONPath
		if infoJSONPath == "" {
			if cached, ok := GetCachedInfoJSON(targetURL); ok {
				infoJSONPath = cached
			} else if cachedOrig, okOrig := GetCachedInfoJSON(task.URL); okOrig {
				infoJSONPath = cachedOrig
			}
		}

		if attempt == 1 && infoJSONPath != "" {
			if fi, err := os.Stat(infoJSONPath); err == nil && !fi.IsDir() && fi.Size() > 0 && time.Since(fi.ModTime()) < 4*time.Hour {
				usedInfoJSON = true
				args = append(args, "--load-info-json", infoJSONPath)
				log.Info().Msgf("[FastStart] Fast start via cached info-json for %s (%s)", task.URL, infoJSONPath)
			}
		}

		if !usedInfoJSON {
			args = append(args, targetURL)
		}

		cmd := exec.CommandContext(ctx, "yt-dlp", args...)
		cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			lastErr = err.Error()
			continue
		}
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		if err := cmd.Start(); err != nil {
			lastErr = err.Error()
			continue
		}

		scanner := bufio.NewScanner(stdoutPipe)
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				q.parseProgressLine(task, line)
			}
		}

		err = cmd.Wait()

		if ctx.Err() == context.Canceled {
			_ = os.RemoveAll(targetDir)
			q.mu.Lock()
			task.Status = StatusCancelled
			now := time.Now()
			task.CompletedAt = &now
			q.saveStateLocked()
			q.mu.Unlock()
			q.broadcastTaskUpdate(task)
			return
		}

		if err == nil {
			moveJsonFiles(dirID)
			ch := task.Channel
			if ch == "" {
				ch = task.Uploader
			}
			if ch == "" {
				ch = task.PlaylistTitle
			}
			grpTitle := task.GroupTitle
			if grpTitle == "" {
				grpTitle = task.PlaylistTitle
			}
			_ = organizeDownloadedTaskFilesWithGroup(dirID, ch, grpTitle, task.Platform, task.Title)
			medias, _ := getAllFilesForId(dirID)
			if len(medias) > 0 {
				var chosen *Media
				for i := range medias {
					cat := getCategoryByExt(filepath.Ext(medias[i].Name))
					if cat == CategoryVideos || cat == CategoryMusic {
						chosen = &medias[i]
						break
					}
				}
				if chosen != nil {
					q.completeTask(task, *chosen, dirID)
					return
				}
			}
		}

		errMsg := strings.TrimSpace(stderrBuf.String())
		if errMsg == "" && err != nil {
			errMsg = err.Error()
		}
		lastErr = errMsg

		if usedInfoJSON {
			log.Warn().Msgf("Download with cached info-json failed for %s, invalidating cache to force fresh live extraction on retry: %s", task.URL, errMsg)
			InvalidateCachedInfoJSON(task.URL)
			InvalidateCachedInfoJSON(targetURL)
			task.InfoJSONPath = ""
		}

		log.Warn().Msgf("Download attempt %d/%d failed for task %s (%s): %s", attempt, maxAttempts, task.ID, task.URL, errMsg)
		if attempt == maxAttempts && isStreamURL && ctx.Err() == nil {
			ext := ".mp4"
			if isAudio {
				ext = ".mp3"
			}
			stem := sanitizeCaptionFilename(task.Title)
			if stem == "" {
				stem = task.ID
			}
			ffmpegOut := filepath.Join(targetDir, stem+ext)
			log.Info().Msgf("[HLS-Fallback] Attempting ffmpeg remux for %s -> %s", targetURL, ffmpegOut)
			if ffErr := downloadHLSWithFFmpeg(ctx, targetURL, effReferer, ffmpegOut, isAudio); ffErr == nil {
				ch := task.Channel
				if ch == "" {
					ch = task.Uploader
				}
				if ch == "" {
					ch = task.PlaylistTitle
				}
				grpTitle := task.GroupTitle
				if grpTitle == "" {
					grpTitle = task.PlaylistTitle
				}
				_ = organizeDownloadedTaskFilesWithGroup(dirID, ch, grpTitle, task.Platform, task.Title)
				medias, _ := getAllFilesForId(dirID)
				if len(medias) > 0 {
					q.completeTask(task, medias[0], dirID)
					return
				}
			} else {
				lastErr = ffErr.Error()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	_ = os.RemoveAll(targetDir)
	if lastErr == "" {
		lastErr = "No media file generated"
	}
	q.failTask(task, lastErr)
}

func (q *QueueManager) completeTask(task *DownloadTask, item Media, dirID string) {
	q.mu.Lock()
	task.Status = StatusCompleted
	task.Percent = 100.0
	task.Speed = ""
	task.ETA = ""
	task.MediaID = item.Id
	task.MediaName = item.Name

	cleanTitle := strings.TrimSuffix(item.Name, filepath.Ext(item.Name))
	if isUrlLike(task.Title) || task.Title == shortUrl(task.URL) || task.Title == "" {
		task.Title = cleanTitle
	}

	// Check TaskIndex metadata for real title, channel, platform & duration
	if idx, err := readTaskIndex(dirID); err == nil && idx != nil {
		if idx.Title != "" && !isUrlLike(idx.Title) {
			task.Title = idx.Title
		}
		if idx.Channel != "" && (task.Channel == "" || isUrlLike(task.Channel)) {
			task.Channel = idx.Channel
		}
		if idx.Platform != "" && task.Platform == "" {
			task.Platform = idx.Platform
		}
		if idx.Duration != "" && task.Duration == "" {
			task.Duration = idx.Duration
		}
		if idx.GroupTitle != "" && task.GroupTitle == "" {
			task.GroupTitle = idx.GroupTitle
		}
	}
	if task.Platform == "" {
		task.Platform = detectPlatformName(task.URL)
	}

	task.HumanSize = item.HumanSize
	task.TotalBytes = item.SizeInBytes

	// Find downloaded thumbnail or extract frame using ffmpeg
	targetDir := resolveMediaDirectory(dirID)
	foundThumb := false
	if entries, err := os.ReadDir(targetDir); err == nil {
		for _, e := range entries {
			l := strings.ToLower(e.Name())
			if !e.IsDir() && (strings.HasSuffix(l, ".jpg") || strings.HasSuffix(l, ".jpeg") || strings.HasSuffix(l, ".png") || strings.HasSuffix(l, ".webp")) {
				task.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID+"/"+e.Name())
				foundThumb = true
				break
			}
		}
	}
	if !foundThumb {
		thumbPath := filepath.Join(targetDir, "thumb.jpg")
		videoPath := filepath.Join(targetDir, item.Name)
		videoExt := strings.ToLower(filepath.Ext(item.Name))
		if videoExt == ".mp4" || videoExt == ".mkv" || videoExt == ".webm" || videoExt == ".m4v" || videoExt == ".mov" || videoExt == ".avi" {
			if err := ExtractThumbnailFromVideo(videoPath, thumbPath); err == nil {
				task.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID+"/thumb.jpg")
			}
		} else if videoExt == ".mp3" || videoExt == ".m4a" || videoExt == ".flac" {
			if err := ExtractThumbnailFromAudio(videoPath, thumbPath); err == nil {
				task.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID+"/thumb.jpg")
			}
		}
		if task.Thumbnail == "" {
			task.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID)
		}
	}

	now := time.Now()
	task.CompletedAt = &now
	q.saveStateLocked()
	q.mu.Unlock()
	q.broadcastTaskUpdate(task)

	// Fetch Synced Lyrics from LRCLIB for Audio Downloads
	if isAudioFormat(task.Format) || strings.HasSuffix(item.Name, ".mp3") || strings.HasSuffix(item.Name, ".m4a") || strings.HasSuffix(item.Name, ".flac") {
		go func(m Media, rawTitle string) {
			cleanTitle := strings.TrimSuffix(m.Name, filepath.Ext(m.Name))
			parts := strings.Split(cleanTitle, " - ")
			artist := ""
			track := cleanTitle
			if len(parts) >= 2 {
				artist = strings.TrimSpace(parts[0])
				track = strings.TrimSpace(parts[1])
			}
			lyricsCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if res, err := anpan.FetchLyrics(lyricsCtx, track, artist, 0); err == nil && res != nil {
				filePath := filepath.Join(resolveMediaDirectory(dirID), m.Name)
				_, _ = anpan.SaveLrcFile(filePath, res)
				log.Info().Msgf("Saved synced lyrics for %s (track: %s)", m.Name, track)
			}
		}(item, task.Title)
	}
}

var percentRegex = regexp.MustCompile(`([0-9.]+)%`)

func (q *QueueManager) parseProgressLine(task *DownloadTask, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	updated := false
	q.mu.Lock()

	parts := strings.Split(line, "|")
	if len(parts) >= 3 {
		pStr := strings.TrimSpace(strings.TrimSuffix(parts[0], "%"))
		if pVal, err := strconv.ParseFloat(pStr, 64); err == nil {
			task.Percent = pVal
			updated = true
		}
		task.Speed = strings.TrimSpace(parts[1])
		task.ETA = strings.TrimSpace(parts[2])

		if len(parts) >= 5 {
			if dl, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64); err == nil {
				task.Downloaded = dl
			}
			if tot, err := strconv.ParseInt(strings.TrimSpace(parts[4]), 10, 64); err == nil && tot > 0 {
				task.TotalBytes = tot
				task.HumanSize = humanize.Bytes(uint64(tot))
			}
		}
	} else if matches := percentRegex.FindStringSubmatch(line); len(matches) > 1 {
		if pVal, err := strconv.ParseFloat(matches[1], 64); err == nil {
			task.Percent = pVal
			updated = true
		}
	} else if strings.Contains(line, "[Merger]") || strings.Contains(line, "Merging formats into") {
		task.Speed = "Merging"
		task.ETA = "Combining audio+video"
		updated = true
	} else if strings.Contains(line, "[VideoRemuxer]") || strings.Contains(line, "Remuxing video") {
		task.Speed = "Finalizing"
		task.ETA = "Remuxing container"
		updated = true
	}

	q.mu.Unlock()

	if updated {
		q.broadcastTaskUpdate(task)
	}
}

func (q *QueueManager) failTask(task *DownloadTask, errMsg string) {
	q.mu.Lock()
	task.Status = StatusFailed
	task.ErrorMessage = errMsg
	now := time.Now()
	task.CompletedAt = &now
	q.saveStateLocked()
	q.mu.Unlock()

	log.Error().Msgf("Task %s failed: %s", task.ID, errMsg)
	q.broadcastTaskUpdate(task)
}

func (q *QueueManager) broadcastTaskUpdate(task *DownloadTask) {
	GlobalBroadcaster.Broadcast(map[string]interface{}{
		"type": "task_update",
		"task": task,
	})
}

func (q *QueueManager) removeFromQueueLocked(id string) {
	newQueue := make([]string, 0, len(q.queue))
	for _, item := range q.queue {
		if item != id {
			newQueue = append(newQueue, item)
		}
	}
	q.queue = newQueue
}

func (q *QueueManager) stateFilePath() string {
	dir := getDownloadDir()
	return filepath.Join(dir, "queue_state.json")
}

func (q *QueueManager) saveStateLocked() {
	file := q.stateFilePath()
	tasksList := make([]*DownloadTask, 0, len(q.tasks))
	for _, t := range q.tasks {
		tasksList = append(tasksList, t)
	}
	data, err := json.MarshalIndent(tasksList, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal queue state")
		return
	}
	_ = os.WriteFile(file, data, 0644)
}

func (q *QueueManager) CancelGroup(groupID string) int {
	if groupID == "" {
		return 0
	}
	var toCancel []*DownloadTask
	q.mu.Lock()
	for _, t := range q.tasks {
		if t.GroupID == groupID && (t.Status == StatusQueued || t.Status == StatusDownloading) {
			if t.Status == StatusQueued {
				t.Status = StatusCancelled
				now := time.Now()
				t.CompletedAt = &now
				q.removeFromQueueLocked(t.ID)
				toCancel = append(toCancel, t)
			} else if t.Status == StatusDownloading && t.cancelFunc != nil {
				t.Status = StatusCancelled
				t.cancelFunc()
				now := time.Now()
				t.CompletedAt = &now
				toCancel = append(toCancel, t)
			}
		}
	}
	if len(toCancel) > 0 {
		q.saveStateLocked()
	}
	q.mu.Unlock()
	for _, t := range toCancel {
		q.broadcastTaskUpdate(t)
	}
	return len(toCancel)
}

func (q *QueueManager) RetryGroup(groupID string) int {
	if groupID == "" {
		return 0
	}
	q.mu.Lock()
	var retried []*DownloadTask
	for id, t := range q.tasks {
		if t.GroupID == groupID && (t.Status == StatusFailed || t.Status == StatusCancelled) {
			t.Status = StatusQueued
			t.Percent = 0
			t.Speed = ""
			t.ETA = ""
			t.ErrorMessage = ""
			t.CompletedAt = nil
			q.queue = append(q.queue, id)
			retried = append(retried, t)
		}
	}
	if len(retried) > 0 {
		q.saveStateLocked()
		q.dispatchLocked()
	}
	q.mu.Unlock()
	for _, t := range retried {
		q.broadcastTaskUpdate(t)
	}
	return len(retried)
}

func (q *QueueManager) DeleteGroup(groupID string, deleteFiles bool) int {
	if groupID == "" {
		return 0
	}
	q.mu.Lock()
	var removedIDs []string
	for id, t := range q.tasks {
		if t.GroupID == groupID {
			if t.cancelFunc != nil {
				t.cancelFunc()
			}
			q.removeFromQueueLocked(id)
			if deleteFiles {
				deleteTaskFiles(id, t.MediaID)
			}
			delete(q.tasks, id)
			removedIDs = append(removedIDs, id)
		}
	}
	if len(removedIDs) > 0 {
		q.saveStateLocked()
	}
	tasksSnapshot := q.getTasksLocked()
	q.mu.Unlock()

	if len(removedIDs) > 0 {
		GlobalBroadcaster.Broadcast(map[string]interface{}{
			"type":  "init",
			"tasks": tasksSnapshot,
		})
	}
	return len(removedIDs)
}

func (q *QueueManager) LoadState() {
	q.mu.Lock()
	defer q.mu.Unlock()

	file := q.stateFilePath()
	data, err := os.ReadFile(file)
	if err != nil {
		return
	}

	var tasksList []*DownloadTask
	if err := json.Unmarshal(data, &tasksList); err != nil {
		return
	}

	ytRegex := regexp.MustCompile(`(?:youtu\.be/|youtube\.com/(?:watch\?v=|shorts/|embed/))([a-zA-Z0-9_-]{11})`)
	channelCounts := make(map[string]int)

	for _, t := range tasksList {
		if t.Status == StatusDownloading || t.Status == StatusQueued {
			t.Status = StatusFailed
			t.ErrorMessage = "Interrupted by server restart"
		}

		if t.Platform == "" && t.URL != "" {
			t.Platform = detectPlatformName(t.URL)
		}

		// Backfill real metadata from TaskIndex if available
		dirID := GetMD5Hash(t.URL, map[string]string{"format": t.Format})
		if idx, err := readTaskIndex(dirID); err == nil && idx != nil {
			if idx.Title != "" && !isUrlLike(idx.Title) && (t.Title == "" || isUrlLike(t.Title) || t.Title == shortUrl(t.URL)) {
				t.Title = idx.Title
			}
			if idx.Channel != "" && t.Channel == "" {
				t.Channel = idx.Channel
			}
			if idx.Duration != "" && t.Duration == "" {
				t.Duration = idx.Duration
			}
			if idx.GroupTitle != "" && t.GroupTitle == "" {
				t.GroupTitle = idx.GroupTitle
			}
		}

		if t.GroupID == "" && t.PlaylistTitle != "" {
			t.GroupID = "pl_" + GetMD5Hash(t.PlaylistTitle, nil)[:10]
			t.GroupTitle = t.PlaylistTitle
			if t.GroupType == "" {
				t.GroupType = "playlist"
			}
		}

		if t.Channel != "" && !isUrlLike(t.Channel) {
			channelCounts[t.Channel]++
		}

		// Ensure thumbnail is available for existing tasks
		if t.Thumbnail == "" && t.URL != "" {
			if m := ytRegex.FindStringSubmatch(t.URL); len(m) > 1 {
				t.Thumbnail = "https://i.ytimg.com/vi/" + m[1] + "/hqdefault.jpg"
			}
		}

		if t.Status == StatusCompleted && t.MediaID != "" {
			parts := strings.Split(t.MediaID, "/")
			mediaDirID := parts[0]
			if len(parts) == 3 {
				mediaDirID = parts[1]
			}
			targetDir := resolveMediaDirectory(mediaDirID)
			foundThumb := false
			if entries, err := os.ReadDir(targetDir); err == nil {
				for _, e := range entries {
					l := strings.ToLower(e.Name())
					if !e.IsDir() && (strings.HasSuffix(l, ".jpg") || strings.HasSuffix(l, ".jpeg") || strings.HasSuffix(l, ".png") || strings.HasSuffix(l, ".webp")) {
						t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(mediaDirID+"/"+e.Name())
						foundThumb = true
						break
					}
				}
			}
			if !foundThumb && t.MediaName != "" {
				videoPath := filepath.Join(targetDir, t.MediaName)
				thumbPath := filepath.Join(targetDir, "thumb.jpg")
				videoExt := strings.ToLower(filepath.Ext(t.MediaName))
				if videoExt == ".mp4" || videoExt == ".mkv" || videoExt == ".webm" || videoExt == ".m4v" || videoExt == ".mov" || videoExt == ".avi" {
					if err := ExtractThumbnailFromVideo(videoPath, thumbPath); err == nil {
						t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(mediaDirID+"/thumb.jpg")
					}
				} else if videoExt == ".mp3" || videoExt == ".m4a" || videoExt == ".flac" {
					if err := ExtractThumbnailFromAudio(videoPath, thumbPath); err == nil {
						t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(mediaDirID+"/thumb.jpg")
					}
				}
			}
			if t.Thumbnail == "" {
				t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(mediaDirID)
			}
		}

		q.tasks[t.ID] = t
	}

	// Second pass: group ungrouped tasks sharing a channel (>= 2 items)
	var ungrouped []*DownloadTask
	for _, t := range tasksList {
		if t.GroupID == "" && t.Channel != "" && channelCounts[t.Channel] >= 2 {
			t.GroupID = "ch_" + GetMD5Hash(t.Channel, nil)[:10]
			t.GroupTitle = t.Channel
			t.GroupType = "channel"
		}
		if t.GroupID == "" {
			ungrouped = append(ungrouped, t)
		}
	}

	// Third pass: cluster remaining ungrouped tasks enqueued within 60s with same platform & format
	if len(ungrouped) >= 2 {
		sort.Slice(ungrouped, func(i, j int) bool {
			return ungrouped[i].CreatedAt.Before(ungrouped[j].CreatedAt)
		})
		var cluster []*DownloadTask
		flushCluster := func() {
			if len(cluster) >= 2 {
				gid := "batch_" + GetMD5Hash(cluster[0].ID, nil)[:10]
				pName := cluster[0].Platform
				if pName == "" {
					pName = "Media"
				}
				gTitle := pName + " Batch (" + strconv.Itoa(len(cluster)) + " items)"
				for _, item := range cluster {
					item.GroupID = gid
					item.GroupTitle = gTitle
					item.GroupType = "batch"
				}
			}
			cluster = nil
		}
		for _, t := range ungrouped {
			if len(cluster) == 0 {
				cluster = append(cluster, t)
				continue
			}
			prev := cluster[len(cluster)-1]
			if t.Platform == prev.Platform && t.Format == prev.Format && t.CreatedAt.Sub(prev.CreatedAt) <= 60*time.Second {
				cluster = append(cluster, t)
			} else {
				flushCluster()
				cluster = append(cluster, t)
			}
		}
		flushCluster()
	}

	log.Info().Msgf("Loaded %d download tasks from persistent queue state", len(q.tasks))
}

func shortUrl(u string) string {
	if len(u) > 50 {
		return u[:47] + "..."
	}
	return u
}

// Queue API HTTP Handlers

func QueueAddHandler(w http.ResponseWriter, r *http.Request) {
	type QueueItemInput struct {
		URL          string `json:"url"`
		Title        string `json:"title"`
		Thumbnail    string `json:"thumbnail"`
		Uploader     string `json:"uploader"`
		Channel      string `json:"channel"`
		Duration     string `json:"duration,omitempty"`
		InfoJSONPath string `json:"infoJsonPath,omitempty"`
		Referer      string `json:"referer,omitempty"`
	}

	var body struct {
		URLs          []string         `json:"urls"`
		URL           string           `json:"url"`
		Items         []QueueItemInput `json:"items"`
		Format        string           `json:"format"`
		Cookies       string           `json:"cookies"`
		RateLimit     string           `json:"rateLimit"`
		PlaylistTitle string           `json:"playlistTitle"`
		Channel       string           `json:"channel"`
		GroupID       string           `json:"groupId"`
		GroupTitle    string           `json:"groupTitle"`
		GroupType     string           `json:"groupType"`
		Referer       string           `json:"referer,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	addedTasks := make([]*DownloadTask, 0)
	if len(body.Items) > 0 {
		gid := strings.TrimSpace(body.GroupID)
		gTitle := strings.TrimSpace(body.GroupTitle)
		gType := strings.TrimSpace(body.GroupType)
		if gTitle == "" {
			gTitle = strings.TrimSpace(body.PlaylistTitle)
		}
		if gTitle == "" {
			gTitle = strings.TrimSpace(body.Channel)
		}
		if len(body.Items) > 1 && gid == "" {
			gid = "grp_" + uuid.New().String()[:8]
			if gType == "" {
				if body.PlaylistTitle != "" {
					gType = "playlist"
				} else if body.Channel != "" {
					gType = "channel"
				} else {
					gType = "batch"
				}
			}
			if gTitle == "" {
				gTitle = detectPlatformName(body.Items[0].URL) + " Batch (" + strconv.Itoa(len(body.Items)) + " items)"
			}
		}

		for _, it := range body.Items {
			u := strings.TrimSpace(it.URL)
			if u != "" {
				ch := it.Channel
				if ch == "" {
					ch = body.Channel
				}
				up := it.Uploader
				task := GlobalQueue.AddTaskWithGroup(u, body.Format, body.Cookies, body.RateLimit, it.Title, it.Thumbnail, body.PlaylistTitle, ch, up, gid, gTitle, gType, it.Duration)
				if it.InfoJSONPath != "" {
					task.InfoJSONPath = it.InfoJSONPath
				}
				if it.Referer != "" {
					task.Referer = it.Referer
				} else if body.Referer != "" {
					task.Referer = body.Referer
				}
				addedTasks = append(addedTasks, task)
			}
		}
	} else {
		urls := body.URLs
		if len(urls) == 0 && body.URL != "" {
			urls = []string{body.URL}
		}

		cleanUrls := make([]string, 0, len(urls))
		for _, u := range urls {
			u = strings.TrimSpace(u)
			if u != "" {
				cleanUrls = append(cleanUrls, u)
			}
		}

		batchGID := strings.TrimSpace(body.GroupID)
		batchTitle := strings.TrimSpace(body.GroupTitle)
		batchType := strings.TrimSpace(body.GroupType)
		if len(cleanUrls) > 1 && batchGID == "" {
			batchGID = "batch_" + uuid.New().String()[:8]
			if batchType == "" {
				batchType = "batch"
			}
			if batchTitle == "" {
				batchTitle = strings.TrimSpace(body.PlaylistTitle)
			}
			if batchTitle == "" {
				batchTitle = detectPlatformName(cleanUrls[0]) + " Batch (" + strconv.Itoa(len(cleanUrls)) + " links)"
			}
		}

		for _, u := range cleanUrls {
			if IsFacebookProfileURL(u) {
				if scan, _, err := ScrapeFacebookVideos(u, body.Cookies, 1, 100); err == nil && scan != nil && len(scan.Entries) > 0 {
					gid := "grp_" + GetMD5Hash(u, nil)[:10]
					gTitle := scan.Title
					if gTitle == "" {
						gTitle = scan.Channel
					}
					if gTitle == "" {
						gTitle = "Facebook Videos"
					}
					for _, entry := range scan.Entries {
						tTitle := entry.Title
						if tTitle == "" {
							tTitle = scan.Title
						}
						task := GlobalQueue.AddTaskWithGroup(entry.Url, body.Format, body.Cookies, body.RateLimit, tTitle, entry.Thumbnail, scan.Title, scan.Channel, "", gid, gTitle, "channel", formatDurationFloat(entry.Duration))
						addedTasks = append(addedTasks, task)
					}
					continue
				}
			}

			isPlaylist, plKind := isChannelOrPlaylistURL(u)
			if isPlaylist {
				if scan, _, err := ScanUrlWithPagination(u, body.Cookies, 1, 200); err == nil && scan != nil && len(scan.Entries) > 0 {
					plTitle := scan.Title
					if plTitle == "" {
						plTitle = body.PlaylistTitle
					}
					plChannel := scan.Channel
					if plChannel == "" {
						plChannel = scan.Uploader
					}
					if plChannel == "" {
						plChannel = body.Channel
					}
					gid := "grp_" + GetMD5Hash(u, nil)[:10]
					gTitle := plTitle
					if gTitle == "" {
						gTitle = plChannel
					}
					if gTitle == "" {
						gTitle = detectPlatformName(u) + " Collection"
					}
					for _, entry := range scan.Entries {
						eUrl := entry.Url
						if eUrl == "" && entry.Id != "" {
							if strings.Contains(u, "youtube.com") || strings.Contains(u, "youtu.be") {
								eUrl = "https://www.youtube.com/watch?v=" + entry.Id
							} else {
								eUrl = entry.Id
							}
						}
						if eUrl != "" {
							eThumb := entry.Thumbnail
							if eThumb == "" && len(entry.Thumbnails) > 0 {
								eThumb = entry.Thumbnails[len(entry.Thumbnails)-1].URL
							}
							task := GlobalQueue.AddTaskWithGroup(eUrl, body.Format, body.Cookies, body.RateLimit, entry.Title, eThumb, plTitle, plChannel, entry.Uploader, gid, gTitle, plKind, formatDurationFloat(entry.Duration))
							addedTasks = append(addedTasks, task)
						}
					}
					continue
				}
			}

			task := GlobalQueue.AddTaskWithGroup(u, body.Format, body.Cookies, body.RateLimit, "", "", body.PlaylistTitle, body.Channel, "", batchGID, batchTitle, batchType, "")
			if body.Referer != "" {
				task.Referer = body.Referer
			}
			addedTasks = append(addedTasks, task)
		}
	}

	if len(addedTasks) == 0 {
		http.Error(w, "No valid URLs provided", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"tasks":   addedTasks,
	})
}

func QueueGroupCancelHandler(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("groupId"))
	if groupID == "" {
		http.Error(w, "Missing groupId", http.StatusBadRequest)
		return
	}
	cancelled := GlobalQueue.CancelGroup(groupID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"cancelled": cancelled,
	})
}

func QueueGroupRetryHandler(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("groupId"))
	if groupID == "" {
		http.Error(w, "Missing groupId", http.StatusBadRequest)
		return
	}
	retried := GlobalQueue.RetryGroup(groupID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"retried": retried,
	})
}

func QueueGroupDeleteHandler(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("groupId"))
	if groupID == "" {
		http.Error(w, "Missing groupId", http.StatusBadRequest)
		return
	}
	deleteFiles := r.URL.Query().Get("deleteFiles") == "true"
	removed := GlobalQueue.DeleteGroup(groupID, deleteFiles)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"removed": removed,
	})
}

func QueueListHandler(w http.ResponseWriter, r *http.Request) {
	tasks := GlobalQueue.GetTasks()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks": tasks,
	})
}

func QueueCancelHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing task ID", http.StatusBadRequest)
		return
	}

	ok := GlobalQueue.Cancel(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": ok,
	})
}

func QueueRetryHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing task ID", http.StatusBadRequest)
		return
	}

	ok := GlobalQueue.Retry(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": ok,
	})
}

func QueueDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing task ID", http.StatusBadRequest)
		return
	}

	deleteFile := r.URL.Query().Get("deleteFile") == "true"
	ok := GlobalQueue.Delete(id, deleteFile)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": ok,
	})
}

func QueueClearCompletedHandler(w http.ResponseWriter, r *http.Request) {
	count := GlobalQueue.ClearCompleted()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"cleared": count,
	})
}

func QueueRetryFailedHandler(w http.ResponseWriter, r *http.Request) {
	count := GlobalQueue.RetryAllFailed()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"retried": count,
	})
}

func QueueGetWorkersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"maxWorkers":    GlobalQueue.GetMaxWorkers(),
		"activeWorkers": GlobalQueue.GetActiveWorkers(),
	})
}

func QueueSetWorkersHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Workers int `json:"workers"`
	}
	countParam := r.URL.Query().Get("count")
	if countParam == "" {
		countParam = r.URL.Query().Get("workers")
	}
	if countParam != "" {
		if n, err := strconv.Atoi(countParam); err == nil && n > 0 {
			body.Workers = n
		}
	} else if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if body.Workers < 1 {
		body.Workers = 1
	}
	if body.Workers > 16 {
		body.Workers = 16
	}

	GlobalQueue.SetMaxWorkers(body.Workers)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"maxWorkers":    GlobalQueue.GetMaxWorkers(),
		"activeWorkers": GlobalQueue.GetActiveWorkers(),
	})
}
