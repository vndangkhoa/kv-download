package media

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"kv-download/src/anpan"

	"github.com/rs/zerolog/log"
)

const (
	defaultMaxWorkers         = 4
	defaultConnectionsPerTask = 24
	minConnectionsPerTask     = 4
	maxConnectionsPerTask     = 48
)

// PerformanceSettings controls download concurrency, connection fan-out, and bandwidth limits.
type PerformanceSettings struct {
	MaxWorkers         int    `json:"maxWorkers"`
	ConnectionsPerTask int    `json:"connectionsPerTask"`
	RateLimit          string `json:"rateLimit"`
	NativeHLS          bool   `json:"nativeHls"`
}

// PerformanceSettingsPatch holds optional fields for partial performance settings updates.
type PerformanceSettingsPatch struct {
	MaxWorkers         *int    `json:"maxWorkers"`
	ConnectionsPerTask *int    `json:"connectionsPerTask"`
	RateLimit          *string `json:"rateLimit"`
	NativeHLS          *bool   `json:"nativeHls"`
}

// PerformanceSettingsResponse includes persisted performance settings plus host engine capabilities.
type PerformanceSettingsResponse struct {
	Success            bool   `json:"success,omitempty"`
	MaxWorkers         int    `json:"maxWorkers"`
	ConnectionsPerTask int    `json:"connectionsPerTask"`
	RateLimit          string `json:"rateLimit"`
	NativeHLS          bool   `json:"nativeHls"`
	Aria2cAvailable    bool   `json:"aria2cAvailable"`
	FFmpegAvailable    bool   `json:"ffmpegAvailable"`
}

var (
	perfMu        sync.RWMutex
	perfSettings  PerformanceSettings
	perfLoaded    bool
	perfLoadedDir string
)

func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func defaultPerformanceSettingsFromEnv() PerformanceSettings {
	workers := defaultMaxWorkers
	if env := strings.TrimSpace(os.Getenv("MR_MAX_WORKERS")); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			workers = n
		}
	}

	conns := defaultConnectionsPerTask
	if env := strings.TrimSpace(os.Getenv("MR_CONNECTIONS_PER_TASK")); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			conns = n
		}
	}

	rateLimit := normalizeRateLimit(os.Getenv("MR_RATE_LIMIT"))

	nativeHLS := true
	if env := strings.TrimSpace(os.Getenv("MR_NATIVE_HLS")); env != "" {
		lower := strings.ToLower(env)
		if lower == "0" || lower == "false" || lower == "no" || lower == "off" {
			nativeHLS = false
		}
	}

	return PerformanceSettings{
		MaxWorkers:         clampInt(workers, 1, 16),
		ConnectionsPerTask: clampInt(conns, minConnectionsPerTask, maxConnectionsPerTask),
		RateLimit:          rateLimit,
		NativeHLS:          nativeHLS,
	}
}

func performanceSettingsFilePath() string {
	return filepath.Join(getDownloadDir(), "performance_settings.json")
}

func normalizePerformanceSettings(s PerformanceSettings) PerformanceSettings {
	if s.MaxWorkers <= 0 {
		s.MaxWorkers = defaultMaxWorkers
	}
	s.MaxWorkers = clampInt(s.MaxWorkers, 1, 16)

	if s.ConnectionsPerTask <= 0 {
		s.ConnectionsPerTask = defaultConnectionsPerTask
	}
	s.ConnectionsPerTask = clampInt(s.ConnectionsPerTask, minConnectionsPerTask, maxConnectionsPerTask)
	s.RateLimit = normalizeRateLimit(s.RateLimit)
	return s
}

func loadPerformanceSettingsLocked() {
	dir := getDownloadDir()
	if perfLoaded && perfLoadedDir == dir {
		return
	}

	cfg := defaultPerformanceSettingsFromEnv()
	path := filepath.Join(dir, "performance_settings.json")
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		// Decode over defaults so newly added fields (like nativeHls) retain sensible defaults if omitted.
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Warn().Err(err).Msgf("Failed parsing %s, using defaults", path)
			cfg = defaultPerformanceSettingsFromEnv()
		}
	}

	perfSettings = normalizePerformanceSettings(cfg)
	perfLoaded = true
	perfLoadedDir = dir
}

func writePerformanceSettingsAtomic(dir string, s PerformanceSettings) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	targetPath := filepath.Join(dir, "performance_settings.json")
	tmpPath := fmt.Sprintf("%s.%d.tmp", targetPath, time.Now().UnixNano())
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// GetPerformanceSettings returns the active performance settings, loading lazily from disk on first access.
func GetPerformanceSettings() PerformanceSettings {
	dir := getDownloadDir()
	perfMu.RLock()
	if perfLoaded && perfLoadedDir == dir {
		s := perfSettings
		perfMu.RUnlock()
		return s
	}
	perfMu.RUnlock()

	perfMu.Lock()
	defer perfMu.Unlock()
	loadPerformanceSettingsLocked()
	return perfSettings
}

// UpdatePerformanceSettings applies non-nil patch fields, persists them atomically, and updates queue workers.
func UpdatePerformanceSettings(patch PerformanceSettingsPatch) (PerformanceSettings, error) {
	perfMu.Lock()
	loadPerformanceSettingsLocked()
	next := perfSettings
	prevWorkers := next.MaxWorkers

	if patch.MaxWorkers != nil {
		next.MaxWorkers = *patch.MaxWorkers
	}
	if patch.ConnectionsPerTask != nil {
		next.ConnectionsPerTask = *patch.ConnectionsPerTask
	}
	if patch.RateLimit != nil {
		next.RateLimit = *patch.RateLimit
	}
	if patch.NativeHLS != nil {
		next.NativeHLS = *patch.NativeHLS
	}

	next = normalizePerformanceSettings(next)
	dir := getDownloadDir()
	if err := writePerformanceSettingsAtomic(dir, next); err != nil {
		perfMu.Unlock()
		return perfSettings, err
	}

	perfSettings = next
	perfLoaded = true
	perfLoadedDir = dir
	perfMu.Unlock()

	if GlobalQueue != nil && next.MaxWorkers != prevWorkers {
		GlobalQueue.setMaxWorkersInternal(next.MaxWorkers, false)
	}
	return next, nil
}

func resetPerformanceSettingsCacheForTest() {
	perfMu.Lock()
	defer perfMu.Unlock()
	perfLoaded = false
	perfLoadedDir = ""
	perfSettings = PerformanceSettings{}
}

func isUnlimitedRateLiteral(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return lower == "" || lower == "0" || lower == "unlimited" || lower == "none" || lower == "max"
}

func normalizeRateLimit(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if isUnlimitedRateLiteral(trimmed) {
		return ""
	}
	return trimmed
}

func resolveEffectiveRateLimit(taskRateLimit string) string {
	trimmedTask := strings.TrimSpace(taskRateLimit)
	if trimmedTask != "" {
		// Explicit per-task override such as "unlimited" or "0" clears global caps for that task.
		if isUnlimitedRateLiteral(trimmedTask) {
			return ""
		}
		return trimmedTask
	}
	if s := normalizeRateLimit(GetPerformanceSettings().RateLimit); s != "" {
		return s
	}
	return normalizeRateLimit(os.Getenv("MR_RATE_LIMIT"))
}

// parseRateLimitBytesPerSec parses strings like "10M", "10MB", "10MiB", "500K", "1G" into bytes/sec.
// Returns 0 for unlimited/empty or unparseable values.
func parseRateLimitBytesPerSec(raw string) int64 {
	s := normalizeRateLimit(raw)
	if s == "" {
		return 0
	}

	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	lower = strings.TrimSuffix(lower, "/s")
	lower = strings.TrimSuffix(lower, "ps")
	lower = strings.TrimSpace(lower)

	var mult float64 = 1
	numStr := lower

	switch {
	case strings.HasSuffix(lower, "gib"):
		mult = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(lower, "gib")
	case strings.HasSuffix(lower, "gb"):
		mult = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(lower, "gb")
	case strings.HasSuffix(lower, "gi"):
		mult = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(lower, "gi")
	case strings.HasSuffix(lower, "g"):
		mult = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(lower, "g")
	case strings.HasSuffix(lower, "mib"):
		mult = 1024 * 1024
		numStr = strings.TrimSuffix(lower, "mib")
	case strings.HasSuffix(lower, "mb"):
		mult = 1024 * 1024
		numStr = strings.TrimSuffix(lower, "mb")
	case strings.HasSuffix(lower, "mi"):
		mult = 1024 * 1024
		numStr = strings.TrimSuffix(lower, "mi")
	case strings.HasSuffix(lower, "m"):
		mult = 1024 * 1024
		numStr = strings.TrimSuffix(lower, "m")
	case strings.HasSuffix(lower, "kib"):
		mult = 1024
		numStr = strings.TrimSuffix(lower, "kib")
	case strings.HasSuffix(lower, "kb"):
		mult = 1024
		numStr = strings.TrimSuffix(lower, "kb")
	case strings.HasSuffix(lower, "ki"):
		mult = 1024
		numStr = strings.TrimSuffix(lower, "ki")
	case strings.HasSuffix(lower, "k"):
		mult = 1024
		numStr = strings.TrimSuffix(lower, "k")
	case strings.HasSuffix(lower, "b"):
		mult = 1
		numStr = strings.TrimSuffix(lower, "b")
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil || val <= 0 || math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}
	res := int64(val * mult)
	if res <= 0 {
		return 0
	}
	return res
}

type byteRateLimiter struct {
	mu          sync.Mutex
	bytesPerSec float64
	burst       float64
	tokens      float64
	lastTime    time.Time
}

func newByteRateLimiter(bytesPerSec int64) *byteRateLimiter {
	if bytesPerSec <= 0 {
		return nil
	}
	burst := float64(bytesPerSec) * 0.25
	if burst < 256*1024 {
		burst = 256 * 1024
	}
	if burst < float64(bytesPerSec) {
		// Allow at least 1s worth if rate is very small
		if bytesPerSec < 256*1024 {
			burst = float64(bytesPerSec)
		}
	}
	return &byteRateLimiter{
		bytesPerSec: float64(bytesPerSec),
		burst:       burst,
		tokens:      burst,
		lastTime:    time.Now(),
	}
}

func (l *byteRateLimiter) Wait(ctx context.Context, n int) error {
	if l == nil || n <= 0 {
		return nil
	}
	need := float64(n)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(l.lastTime).Seconds()
		if elapsed > 0 {
			l.tokens += elapsed * l.bytesPerSec
			if l.tokens > l.burst {
				l.tokens = l.burst
			}
			l.lastTime = now
		}

		if l.tokens >= need || need > l.burst && l.tokens > 0 {
			l.tokens -= need
			if l.tokens >= 0 {
				l.mu.Unlock()
				return nil
			}
			deficit := -l.tokens
			waitDur := time.Duration((deficit / l.bytesPerSec) * float64(time.Second))
			l.mu.Unlock()
			if waitDur <= 0 {
				return nil
			}
			timer := time.NewTimer(waitDur)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}

		deficit := need - l.tokens
		waitDur := time.Duration((deficit / l.bytesPerSec) * float64(time.Second))
		l.mu.Unlock()
		if waitDur < time.Millisecond {
			waitDur = time.Millisecond
		}
		timer := time.NewTimer(waitDur)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type speedSample struct {
	at    time.Time
	bytes int64
}

type throughputMeter struct {
	mu      sync.Mutex
	window  time.Duration
	started time.Time
	samples []speedSample
	total   int64
}

func newThroughputMeter() *throughputMeter {
	return &throughputMeter{
		window:  2500 * time.Millisecond,
		started: time.Now(),
		samples: make([]speedSample, 0, 128),
	}
}

func (m *throughputMeter) Add(delta int64) {
	if m == nil || delta <= 0 {
		return
	}
	now := time.Now()
	m.mu.Lock()
	m.samples = append(m.samples, speedSample{at: now, bytes: delta})
	m.total += delta
	m.pruneLocked(now)
	m.mu.Unlock()
}

func (m *throughputMeter) pruneLocked(now time.Time) {
	cutoff := now.Add(-m.window)
	cutIdx := 0
	for cutIdx < len(m.samples) && m.samples[cutIdx].at.Before(cutoff) {
		m.total -= m.samples[cutIdx].bytes
		cutIdx++
	}
	if cutIdx > 0 {
		copy(m.samples, m.samples[cutIdx:])
		m.samples = m.samples[:len(m.samples)-cutIdx]
	}
}

func (m *throughputMeter) SpeedBytesPerSec() float64 {
	if m == nil {
		return 0
	}
	now := time.Now()
	m.mu.Lock()
	m.pruneLocked(now)
	total := m.total
	var span time.Duration
	if len(m.samples) > 0 {
		span = now.Sub(m.samples[0].at)
		if sinceStart := now.Sub(m.started); sinceStart < m.window && sinceStart > span {
			span = sinceStart
		}
	}
	m.mu.Unlock()

	if total <= 0 {
		return 0
	}
	secs := span.Seconds()
	if secs < 0.25 {
		secs = 0.25
	}
	if secs > m.window.Seconds() {
		secs = m.window.Seconds()
	}
	return float64(total) / secs
}

func formatSpeedMiB(bps float64) string {
	if bps <= 0 {
		return ""
	}
	const (
		kib = 1024.0
		mib = 1024.0 * 1024.0
		gib = 1024.0 * 1024.0 * 1024.0
	)
	switch {
	case bps >= gib:
		return fmt.Sprintf("%.2fGiB/s", bps/gib)
	case bps >= mib:
		return fmt.Sprintf("%.2fMiB/s", bps/mib)
	case bps >= kib:
		return fmt.Sprintf("%.1fKiB/s", bps/kib)
	default:
		return fmt.Sprintf("%.0fB/s", bps)
	}
}

func formatETASeconds(sec float64) string {
	if sec <= 0 || math.IsNaN(sec) || math.IsInf(sec, 0) {
		return ""
	}
	total := int(math.Round(sec))
	if total < 0 {
		total = 0
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func buildPerformanceSettingsResponse(s PerformanceSettings, includeSuccess bool) PerformanceSettingsResponse {
	_, ffErr := exec.LookPath("ffmpeg")
	return PerformanceSettingsResponse{
		Success:            includeSuccess,
		MaxWorkers:         s.MaxWorkers,
		ConnectionsPerTask: s.ConnectionsPerTask,
		RateLimit:          s.RateLimit,
		NativeHLS:          s.NativeHLS,
		Aria2cAvailable:    anpan.HasAria2c(),
		FFmpegAvailable:    ffErr == nil,
	}
}

// GetPerformanceSettingsHandler serves GET /api/settings/performance.
func GetPerformanceSettingsHandler(w http.ResponseWriter, r *http.Request) {
	s := GetPerformanceSettings()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildPerformanceSettingsResponse(s, false))
}

// SetPerformanceSettingsHandler handles POST /api/settings/performance.
func SetPerformanceSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var patch PerformanceSettingsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	updated, err := UpdatePerformanceSettings(patch)
	if err != nil {
		http.Error(w, "Failed to save performance settings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildPerformanceSettingsResponse(updated, true))
}
