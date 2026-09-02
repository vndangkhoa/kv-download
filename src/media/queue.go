package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	ID           string     `json:"id"`
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	Thumbnail    string     `json:"thumbnail"`
	Format       string     `json:"format"`
	Cookies      string     `json:"cookies,omitempty"`
	RateLimit    string     `json:"rateLimit,omitempty"`
	Status       TaskStatus `json:"status"`
	Percent      float64    `json:"percent"`
	Speed        string     `json:"speed"`
	ETA          string     `json:"eta"`
	Downloaded   int64      `json:"downloaded"`
	TotalBytes   int64      `json:"totalBytes"`
	HumanSize    string     `json:"humanSize"`
	ErrorMessage string     `json:"errorMessage"`
	MediaID      string     `json:"mediaId"`
	MediaName     string     `json:"mediaName"`
	PlaylistTitle string     `json:"playlistTitle,omitempty"`
	Channel       string     `json:"channel,omitempty"`
	Uploader      string     `json:"uploader,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`

	cancelFunc context.CancelFunc `json:"-"`
}

type QueueManager struct {
	mu           sync.RWMutex
	tasks        map[string]*DownloadTask
	queue        []string
	maxWorkers   int
	activeWorker int
	workChan     chan string
}

var GlobalQueue *QueueManager

func init() {
	GlobalQueue = NewQueueManager(3)
	GlobalQueue.LoadState()
	go GlobalQueue.workerLoop()
}

func NewQueueManager(maxWorkers int) *QueueManager {
	return &QueueManager{
		tasks:      make(map[string]*DownloadTask),
		queue:      make([]string, 0),
		maxWorkers: maxWorkers,
		workChan:   make(chan string, 100),
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

func (q *QueueManager) AddFullWithMeta(url string, format string, cookies string, rateLimit string, title string, thumbnail string, playlistTitle string, channel string, uploader string) *DownloadTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	if format == "" {
		format = "best"
	}

	initialThumb := thumbnail
	if initialThumb == "" {
		ytRegex := regexp.MustCompile(`(?:youtu\.be/|youtube\.com/(?:watch\?v=|shorts/|embed/))([a-zA-Z0-9_-]{11})`)
		if m := ytRegex.FindStringSubmatch(url); len(m) > 1 {
			initialThumb = "https://i.ytimg.com/vi/" + m[1] + "/hqdefault.jpg"
		}
	}

	taskTitle := title
	if taskTitle == "" {
		taskTitle = shortUrl(url)
	}

	task := &DownloadTask{
		ID:            uuid.New().String(),
		URL:           url,
		Title:         taskTitle,
		Thumbnail:     initialThumb,
		Format:        format,
		Cookies:       cookies,
		RateLimit:     rateLimit,
		PlaylistTitle: playlistTitle,
		Channel:       channel,
		Uploader:      uploader,
		Status:        StatusQueued,
		CreatedAt:     time.Now(),
	}

	q.tasks[task.ID] = task
	q.queue = append(q.queue, task.ID)

	log.Info().Msgf("Enqueued task %s for URL %s (title: %s, playlist: %s, channel: %s, format: %s, rateLimit: %s, hasCustomCookies: %t)", task.ID, url, taskTitle, playlistTitle, channel, format, rateLimit, cookies != "")

	q.saveStateLocked()
	q.broadcastTaskUpdate(task)

	select {
	case q.workChan <- task.ID:
	default:
	}

	return task
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

		select {
		case q.workChan <- id:
		default:
		}
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

	if deleteFile && task.MediaID != "" {
		filePath, err := getFileFromId(task.MediaID)
		if err == nil {
			_ = os.Remove(filePath)
			dirID, _, _ := strings.Cut(task.MediaID, "/")
			_ = os.RemoveAll(getMediaDirectory(dirID))
		}
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
			select {
			case q.workChan <- id:
			default:
			}
			count++
		}
	}
	if count > 0 {
		q.saveStateLocked()
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

func (q *QueueManager) workerLoop() {
	for taskID := range q.workChan {
		q.mu.Lock()
		task, exists := q.tasks[taskID]
		if !exists || task.Status != StatusQueued {
			q.mu.Unlock()
			continue
		}

		ctx, cancel := context.WithCancel(context.Background())
		task.cancelFunc = cancel
		task.Status = StatusDownloading
		q.saveStateLocked()
		q.mu.Unlock()

		q.broadcastTaskUpdate(task)
		q.processTask(ctx, task)
	}
}

func (q *QueueManager) processTask(ctx context.Context, task *DownloadTask) {
	defer func() {
		if task.cancelFunc != nil {
			task.cancelFunc()
			task.cancelFunc = nil
		}
	}()

	dirID := GetMD5Hash(task.URL, map[string]string{"format": task.Format})
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
				medias, _ := getAllFilesForId(dirID)
				if len(medias) > 0 {
					q.completeTask(task, medias[0], dirID)
					return
				}
			}
		}
	}

	outputTemplate := targetDir + "%(title,channel+id,uploader+id,id)s.%(ext)s"

	var lastErr string
	maxAttempts := 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() == context.Canceled {
			q.mu.Lock()
			task.Status = StatusCancelled
			now := time.Now()
			task.CompletedAt = &now
			q.saveStateLocked()
			q.mu.Unlock()
			q.broadcastTaskUpdate(task)
			return
		}

		args := []string{
			"--newline",
			"--progress-template", "%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s|%(progress.downloaded_bytes)s|%(progress.total_bytes)s",
			"--no-playlist",
			"--trim-filenames", "120",
			"--write-thumbnail",
			"--convert-thumbnails", "jpg",
			"--remux-video", "mp4",
			"--merge-output-format", "mp4",
			"--output", outputTemplate,
			"--no-check-certificates",
			"--extractor-args", "instagram:image_persist=1;threads:app_version=30.0.0",
		}

		// Attempt 1: try aria2c if available. Attempt 2+: use native yt-dlp downloader to avoid CDN 403s
		if attempt == 1 && anpan.HasAria2c() && task.Format != "audio_mp3" && task.Format != "audio_m4a" && !strings.Contains(task.URL, "tiktok.com") {
			if ariaArgs := anpan.BuildYtDlpAria2Args(16); ariaArgs != nil {
				args = append(args, ariaArgs...)
			}
		}

		// Apply Format Selection
		switch task.Format {
		case "1080p":
			args = append(args, "--format", "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best")
		case "720p":
			args = append(args, "--format", "bestvideo[height<=720]+bestaudio/best[height<=720]/best")
		case "480p":
			args = append(args, "--format", "bestvideo[height<=480]+bestaudio/best[height<=480]/best")
		case "audio_mp3":
			args = append(args, "--extract-audio", "--audio-format", "mp3")
		case "audio_m4a":
			args = append(args, "--extract-audio", "--audio-format", "m4a")
		default:
			args = append(args, "--format", "b/bv*+ba/best")
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

		args = append(args, task.URL)

		cmd := exec.CommandContext(ctx, "yt-dlp", args...)
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		if err := cmd.Start(); err != nil {
			lastErr = err.Error()
			time.Sleep(300 * time.Millisecond)
			continue
		}

		var stderrBuf bytes.Buffer
		go io.Copy(&stderrBuf, stderr)

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			q.parseProgressLine(task, line)
		}

		err := cmd.Wait()
		if ctx.Err() == context.Canceled {
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
			medias, _ := getAllFilesForId(dirID)
			if len(medias) > 0 {
				q.completeTask(task, medias[0], dirID)
				return
			}
		}

		errMsg := strings.TrimSpace(stderrBuf.String())
		if errMsg == "" && err != nil {
			errMsg = err.Error()
		}
		lastErr = errMsg
		log.Warn().Msgf("Download attempt %d/%d failed for task %s (%s): %s", attempt, maxAttempts, task.ID, task.URL, errMsg)
		time.Sleep(500 * time.Millisecond)
	}

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
	if task.Title == shortUrl(task.URL) {
		task.Title = item.Name
	}
	task.HumanSize = item.HumanSize
	task.TotalBytes = item.SizeInBytes

	// Find downloaded thumbnail or extract frame using ffmpeg
	targetDir := getMediaDirectory(dirID)
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
	if task.Format == "audio_mp3" || task.Format == "audio_m4a" {
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
				filePath := filepath.Join(getMediaDirectory(dirID), m.Name)
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

	for _, t := range tasksList {
		if t.Status == StatusDownloading || t.Status == StatusQueued {
			t.Status = StatusFailed
			t.ErrorMessage = "Interrupted by server restart"
		}

		// Ensure thumbnail is available for existing tasks
		if t.Thumbnail == "" && t.URL != "" {
			if m := ytRegex.FindStringSubmatch(t.URL); len(m) > 1 {
				t.Thumbnail = "https://i.ytimg.com/vi/" + m[1] + "/hqdefault.jpg"
			}
		}

		if t.Status == StatusCompleted && t.MediaID != "" {
			dirID, _, _ := strings.Cut(t.MediaID, "/")
			targetDir := getMediaDirectory(dirID)
			foundThumb := false
			if entries, err := os.ReadDir(targetDir); err == nil {
				for _, e := range entries {
					l := strings.ToLower(e.Name())
					if !e.IsDir() && (strings.HasSuffix(l, ".jpg") || strings.HasSuffix(l, ".jpeg") || strings.HasSuffix(l, ".png") || strings.HasSuffix(l, ".webp")) {
						t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID+"/"+e.Name())
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
						t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID+"/thumb.jpg")
					}
				} else if videoExt == ".mp3" || videoExt == ".m4a" || videoExt == ".flac" {
					if err := ExtractThumbnailFromAudio(videoPath, thumbPath); err == nil {
						t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID+"/thumb.jpg")
					}
				}
			}
			if t.Thumbnail == "" {
				t.Thumbnail = "/thumbnail?id=" + url.QueryEscape(dirID)
			}
		}

		q.tasks[t.ID] = t
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
		URL       string `json:"url"`
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail"`
		Uploader  string `json:"uploader"`
		Channel   string `json:"channel"`
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
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	addedTasks := make([]*DownloadTask, 0)
	if len(body.Items) > 0 {
		for _, it := range body.Items {
			u := strings.TrimSpace(it.URL)
			if u != "" {
				ch := it.Channel
				if ch == "" {
					ch = body.Channel
				}
				up := it.Uploader
				task := GlobalQueue.AddFullWithMeta(u, body.Format, body.Cookies, body.RateLimit, it.Title, it.Thumbnail, body.PlaylistTitle, ch, up)
				addedTasks = append(addedTasks, task)
			}
		}
	} else {
		urls := body.URLs
		if len(urls) == 0 && body.URL != "" {
			urls = []string{body.URL}
		}
		for _, u := range urls {
			u = strings.TrimSpace(u)
			if u != "" {
				task := GlobalQueue.AddFullWithMeta(u, body.Format, body.Cookies, body.RateLimit, "", "", body.PlaylistTitle, body.Channel, "")
				addedTasks = append(addedTasks, task)
			}
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
