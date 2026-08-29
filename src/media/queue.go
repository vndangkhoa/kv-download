package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	Status       TaskStatus `json:"status"`
	Percent      float64    `json:"percent"`
	Speed        string     `json:"speed"`
	ETA          string     `json:"eta"`
	Downloaded   int64      `json:"downloaded"`
	TotalBytes   int64      `json:"totalBytes"`
	HumanSize    string     `json:"humanSize"`
	ErrorMessage string     `json:"errorMessage"`
	MediaID      string     `json:"mediaId"`
	MediaName    string     `json:"mediaName"`
	CreatedAt    time.Time  `json:"createdAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`

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
	q.mu.Lock()
	defer q.mu.Unlock()

	if format == "" {
		format = "best"
	}

	task := &DownloadTask{
		ID:        uuid.New().String(),
		URL:       url,
		Title:     shortUrl(url),
		Format:    format,
		Status:    StatusQueued,
		CreatedAt: time.Now(),
	}

	q.tasks[task.ID] = task
	q.queue = append(q.queue, task.ID)

	log.Info().Msgf("Enqueued task %s for URL %s (format: %s)", task.ID, url, format)

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
	outputTemplate := getMediaDirectory(dirID) + "%(title,channel+id,uploader+id,id)s.%(ext)s"

	args := []string{
		"--newline",
		"--progress-template", "%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s|%(progress.downloaded_bytes)s|%(progress.total_bytes)s",
		"--no-playlist",
		"--trim-filenames", "120",
		"--remux-video", "mp4",
		"--merge-output-format", "mp4",
		"--write-info-json",
		"--output", outputTemplate,
		"--no-check-certificates",
		"--extractor-args", "instagram:image_persist=1;tiktok:app_version=30.0.0",
		"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
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
	} else {
		args = append(args, "--impersonate", "chrome")
	}

	cookiesPath := getCookiesPath()
	if workingCookies := getWorkingCookiesPath(cookiesPath); workingCookies != "" {
		args = append(args, "--cookies", workingCookies)
	}

	for arg, value := range getEnvVars() {
		args = append(args, arg, value)
	}

	args = append(args, task.URL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		q.failTask(task, err.Error())
		return
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

	if err != nil {
		errMsg := strings.TrimSpace(stderrBuf.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		q.failTask(task, errMsg)
		return
	}

	moveJsonFiles(dirID)
	medias, _ := getAllFilesForId(dirID)
	if len(medias) > 0 {
		q.mu.Lock()
		task.Status = StatusCompleted
		task.Percent = 100.0
		task.Speed = ""
		task.ETA = ""
		task.MediaID = medias[0].Id
		task.MediaName = medias[0].Name
		if task.Title == shortUrl(task.URL) {
			task.Title = medias[0].Name
		}
		task.HumanSize = medias[0].HumanSize
		task.TotalBytes = medias[0].SizeInBytes
		now := time.Now()
		task.CompletedAt = &now
		q.saveStateLocked()
		q.mu.Unlock()
		q.broadcastTaskUpdate(task)
	} else {
		q.failTask(task, "No media file generated")
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
	for _, qid := range q.queue {
		if qid != id {
			newQueue = append(newQueue, qid)
		}
	}
	q.queue = newQueue
}

func (q *QueueManager) stateFilePath() string {
	return filepath.Join(getDownloadDir(), "queue_state.json")
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

	for _, t := range tasksList {
		if t.Status == StatusDownloading || t.Status == StatusQueued {
			t.Status = StatusFailed
			t.ErrorMessage = "Interrupted by server restart"
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
	var body struct {
		URLs   []string `json:"urls"`
		URL    string   `json:"url"`
		Format string   `json:"format"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	urls := body.URLs
	if len(urls) == 0 && body.URL != "" {
		urls = []string{body.URL}
	}

	if len(urls) == 0 {
		http.Error(w, "No URLs provided", http.StatusBadRequest)
		return
	}

	addedTasks := make([]*DownloadTask, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" {
			task := GlobalQueue.Add(u, body.Format)
			addedTasks = append(addedTasks, task)
		}
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
