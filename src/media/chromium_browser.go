package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	defaultChromiumWidth     = 1280
	defaultChromiumHeight    = 800
	minChromiumWidth         = 360
	maxChromiumWidth         = 1920
	minChromiumHeight        = 320
	maxChromiumHeight        = 1200
	screencastMinInterval    = 140 * time.Millisecond
	chromiumIdleNoClients    = 45 * time.Second
	chromiumIdleWithClients  = 3 * time.Minute
	chromiumWatchdogInterval = 15 * time.Second
	defaultChromiumStartPage = "https://www.google.com"
	chromiumDesktopUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
)

type ChromiumAction struct {
	Type       string  `json:"type"`
	URL        string  `json:"url,omitempty"`
	X          float64 `json:"x,omitempty"`
	Y          float64 `json:"y,omitempty"`
	Button     string  `json:"button,omitempty"`
	ClickCount int     `json:"clickCount,omitempty"`
	DeltaX     float64 `json:"deltaX,omitempty"`
	DeltaY     float64 `json:"deltaY,omitempty"`
	Key        string  `json:"key,omitempty"`
	Code       string  `json:"code,omitempty"`
	KeyCode    int     `json:"keyCode,omitempty"`
	Text       string  `json:"text,omitempty"`
	Modifiers  int     `json:"modifiers,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
}

type ChromiumStatus struct {
	Available    bool   `json:"available"`
	BinaryPath   string `json:"binaryPath,omitempty"`
	Running      bool   `json:"running"`
	CurrentURL   string `json:"currentUrl,omitempty"`
	PageTitle    string `json:"pageTitle,omitempty"`
	CanGoBack    bool   `json:"canGoBack"`
	CanGoForward bool   `json:"canGoForward"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	ClientCount  int    `json:"clientCount"`
	MediaCount   int    `json:"mediaCount"`
}

type chromiumEvent struct {
	Type       string      `json:"type"`
	Data       string      `json:"data,omitempty"`
	Width      int         `json:"width,omitempty"`
	Height     int         `json:"height,omitempty"`
	URL        string      `json:"url,omitempty"`
	Title      string      `json:"title,omitempty"`
	CanGoBack  bool        `json:"canGoBack,omitempty"`
	CanForward bool        `json:"canGoForward,omitempty"`
	Running    *bool       `json:"running,omitempty"`
	Available  *bool       `json:"available,omitempty"`
	Item       interface{} `json:"item,omitempty"`
	Items      interface{} `json:"items,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type cdpRequest struct {
	ID     int64                  `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type cdpRPCResult struct {
	Result json.RawMessage
	Err    error
}

type chromiumManager struct {
	startMu    sync.Mutex
	stateMu    sync.RWMutex
	cdpWriteMu sync.Mutex
	pendingMu  sync.Mutex
	subsMu     sync.Mutex
	ackMu      sync.Mutex

	running           bool
	cmd               *exec.Cmd
	pgid              int
	profileDir        string
	debugPort         int
	cdpConn           *websocket.Conn
	stopCh            chan struct{}
	currentURL        string
	pageTitle         string
	canGoBack         bool
	canGoForward      bool
	width             int
	height            int
	screencastPaused  bool
	lastScreencastAck time.Time
	lastFrame         string
	lastActivity      time.Time
	watchdogOnce      sync.Once

	msgSeq  atomic.Int64
	pending map[int64]chan cdpRPCResult

	subSeq      atomic.Int64
	subscribers map[int64]chan chromiumEvent

	mediaMap  map[string]SniffedMediaItem
	mediaList []SniffedMediaItem
}

var (
	globalChromium = &chromiumManager{
		width:       defaultChromiumWidth,
		height:      defaultChromiumHeight,
		pending:     make(map[int64]chan cdpRPCResult),
		subscribers: make(map[int64]chan chromiumEvent),
		mediaMap:    make(map[string]SniffedMediaItem),
	}
	chromiumWSUpgrader = websocket.Upgrader{
		ReadBufferSize:  8192,
		WriteBufferSize: 65536,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
)

// findChromiumBinary locates an installed Chromium / Google Chrome binary without starting it.
func findChromiumBinary() string {
	if envBin := strings.TrimSpace(os.Getenv("CHROMIUM_BIN")); envBin != "" {
		if fi, err := os.Stat(envBin); err == nil && !fi.IsDir() {
			return envBin
		}
	}
	for _, candidate := range []string{
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"chromium-browser",
		"chromium",
		"google-chrome",
		"google-chrome-stable",
	} {
		if path, err := exec.LookPath(candidate); err == nil && path != "" {
			return path
		}
	}
	return ""
}

// normalizeOmniboxURL converts typed URLs or search keywords into an http/https URL.
func normalizeOmniboxURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultChromiumStartPage
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if isSupportedHTTPURL(trimmed) {
			return trimmed
		}
		return "https://www.google.com/search?q=" + url.QueryEscape(trimmed)
	}
	if strings.Contains(trimmed, " ") || (!strings.Contains(trimmed, ".") && !strings.HasPrefix(lower, "localhost")) {
		return "https://www.google.com/search?q=" + url.QueryEscape(trimmed)
	}
	candidate := "https://" + trimmed
	if isSupportedHTTPURL(candidate) {
		return candidate
	}
	return "https://www.google.com/search?q=" + url.QueryEscape(trimmed)
}

func (m *chromiumManager) getStatus() ChromiumStatus {
	bin := findChromiumBinary()
	m.stateMu.RLock()
	st := ChromiumStatus{
		Available:    bin != "",
		BinaryPath:   bin,
		Running:      m.running,
		CurrentURL:   m.currentURL,
		PageTitle:    m.pageTitle,
		CanGoBack:    m.canGoBack,
		CanGoForward: m.canGoForward,
		Width:        m.width,
		Height:       m.height,
		MediaCount:   len(m.mediaList),
	}
	m.stateMu.RUnlock()

	m.subsMu.Lock()
	st.ClientCount = len(m.subscribers)
	m.subsMu.Unlock()
	return st
}

func (m *chromiumManager) touchActivity() {
	m.stateMu.Lock()
	m.lastActivity = time.Now()
	m.stateMu.Unlock()
}

func (m *chromiumManager) subscribe() (int64, chan chromiumEvent) {
	id := m.subSeq.Add(1)
	ch := make(chan chromiumEvent, 32)
	m.subsMu.Lock()
	m.subscribers[id] = ch
	count := len(m.subscribers)
	m.subsMu.Unlock()
	m.touchActivity()
	if count == 1 {
		go m.resumeScreencastIfPaused()
	}
	return id, ch
}

func (m *chromiumManager) unsubscribe(id int64) {
	m.subsMu.Lock()
	remaining := len(m.subscribers)
	if ch, ok := m.subscribers[id]; ok {
		delete(m.subscribers, id)
		close(ch)
		remaining = len(m.subscribers)
	}
	m.subsMu.Unlock()
	if remaining == 0 {
		go m.pauseScreencast()
	}
}

func (m *chromiumManager) broadcast(ev chromiumEvent) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	for _, ch := range m.subscribers {
		select {
		case ch <- ev:
		default:
			// Drop oldest queued message (typically a video frame) so slow readers never stall CDP.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

func allocateLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected TCP listener address")
	}
	return addr.Port, nil
}

func (m *chromiumManager) ensureWatchdog() {
	m.watchdogOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(chromiumWatchdogInterval)
			defer ticker.Stop()
			for range ticker.C {
				m.stateMu.RLock()
				running := m.running
				lastAct := m.lastActivity
				m.stateMu.RUnlock()
				if !running {
					continue
				}

				m.subsMu.Lock()
				clients := len(m.subscribers)
				m.subsMu.Unlock()

				idle := time.Since(lastAct)
				if (clients == 0 && idle > chromiumIdleNoClients) || (clients > 0 && idle > chromiumIdleWithClients) {
					log.Info().Msgf("[Chromium] Stopping idle Chromium instance (clients=%d, idle=%s)", clients, idle.Round(time.Second))
					m.stop()
				}
			}
		}()
	})
}

func clampChromiumViewport(w, h int) (int, int) {
	if w <= 0 {
		w = defaultChromiumWidth
	} else if w < minChromiumWidth {
		w = minChromiumWidth
	} else if w > maxChromiumWidth {
		w = maxChromiumWidth
	}
	if h <= 0 {
		h = defaultChromiumHeight
	} else if h < minChromiumHeight {
		h = minChromiumHeight
	} else if h > maxChromiumHeight {
		h = maxChromiumHeight
	}
	return w, h
}

func nextScreencastAckDelay(now, lastAck time.Time, minInterval time.Duration) time.Duration {
	if minInterval <= 0 || lastAck.IsZero() {
		return 0
	}
	nextAllowed := lastAck.Add(minInterval)
	if !now.Before(nextAllowed) {
		return 0
	}
	delay := nextAllowed.Sub(now)
	if delay > minInterval*2 {
		return minInterval
	}
	return delay
}

func parseQueryViewport(r *http.Request) (int, int) {
	if r == nil {
		return 0, 0
	}
	q := r.URL.Query()
	w, wErr := strconv.Atoi(strings.TrimSpace(q.Get("width")))
	h, hErr := strconv.Atoi(strings.TrimSpace(q.Get("height")))
	if wErr != nil || hErr != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return clampChromiumViewport(w, h)
}

func buildChromiumArgs(width, height, port int, profileDir, target string) []string {
	w, h := clampChromiumViewport(width, height)
	return []string{
		"--headless=new",
		"--no-sandbox",
		"--disable-setuid-sandbox",
		"--disable-gpu",
		"--disable-software-rasterizer",
		"--disable-webgl",
		"--disable-webgl2",
		"--disable-3d-apis",
		"--disable-dev-shm-usage",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-domain-reliability",
		"--disable-crash-reporter",
		"--disable-breakpad",
		"--disable-extensions",
		"--disable-sync",
		"--disable-translate",
		"--no-first-run",
		"--no-default-browser-check",
		"--mute-audio",
		"--autoplay-policy=document-user-activation-required",
		"--renderer-process-limit=1",
		"--num-raster-threads=1",
		"--js-flags=--max-old-space-size=160",
		"--disk-cache-size=33554432",
		"--disable-features=AudioServiceOutOfProcess,IsolateOrigins,site-per-process,Translate,BackForwardCache,MediaRouter,OptimizationHints,PaintHolding,InterestFeedContentSuggestions",
		fmt.Sprintf("--window-size=%d,%d", w, h),
		"--remote-debugging-address=127.0.0.1",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-allow-origins=*",
		"--user-data-dir=" + profileDir,
		"--disable-blink-features=AutomationControlled",
		"--user-agent=" + chromiumDesktopUserAgent,
		target,
	}
}

func (m *chromiumManager) ensureStarted(initialURL string) error {
	m.ensureWatchdog()
	m.startMu.Lock()
	defer m.startMu.Unlock()

	target := ""
	if strings.TrimSpace(initialURL) != "" {
		target = normalizeOmniboxURL(initialURL)
	}

	m.stateMu.RLock()
	alreadyRunning := m.running && m.cdpConn != nil
	currURL := m.currentURL
	width := m.width
	height := m.height
	m.stateMu.RUnlock()

	if alreadyRunning {
		m.touchActivity()
		if target != "" && target != currURL {
			return m.navigate(target)
		}
		return nil
	}

	bin := findChromiumBinary()
	if bin == "" {
		return errors.New("chromium binary is not installed in this environment")
	}

	port, err := allocateLocalPort()
	if err != nil {
		return fmt.Errorf("failed to allocate CDP debug port: %w", err)
	}
	profileDir, err := os.MkdirTemp("", "kv-chromium-*")
	if err != nil {
		return fmt.Errorf("failed to create chromium profile dir: %w", err)
	}

	if target == "" {
		target = defaultChromiumStartPage
	}
	width, height = clampChromiumViewport(width, height)

	chromeArgs := buildChromiumArgs(width, height, port, profileDir, target)

	var stderrBuf bytes.Buffer
	var cmd *exec.Cmd
	if nicePath, nErr := exec.LookPath("nice"); nErr == nil && nicePath != "" {
		cmd = exec.Command(nicePath, append([]string{"-n", "15", bin}, chromeArgs...)...)
	} else {
		cmd = exec.Command(bin, chromeArgs...)
	}
	cmd.Stderr = &stderrBuf
	cmd.Stdout = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	log.Info().Msgf("[Chromium] Launching %s on CDP port %d (%s)", filepath.Base(bin), port, target)
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("failed to launch chromium: %w", err)
	}

	pgid := 0
	if cmd.Process != nil {
		pgid = cmd.Process.Pid
	}

	// Reap exited child process cleanly in the background.
	procDone := make(chan error, 1)
	go func() {
		procDone <- cmd.Wait()
	}()

	// Poll CDP /json/list until a page target exposes webSocketDebuggerUrl.
	wsURL := ""
	pollClient := &http.Client{Timeout: 1200 * time.Millisecond}
	deadline := time.Now().Add(14 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case waitErr := <-procDone:
			_ = os.RemoveAll(profileDir)
			return fmt.Errorf("chromium exited prematurely (%v): %s", waitErr, strings.TrimSpace(stderrBuf.String()))
		default:
		}

		resp, rErr := pollClient.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
		if rErr == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var targets []struct {
				Type                 string `json:"type"`
				URL                  string `json:"url"`
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			if json.Unmarshal(body, &targets) == nil {
				for _, t := range targets {
					if t.Type == "page" && t.WebSocketDebuggerURL != "" {
						wsURL = t.WebSocketDebuggerURL
						break
					}
				}
				if wsURL != "" {
					break
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}

	if wsURL == "" {
		if pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("timed out waiting for Chromium CDP endpoint (%s)", strings.TrimSpace(stderrBuf.String()))
	}

	cdpConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("failed to dial Chromium CDP WebSocket: %w", err)
	}

	stopCh := make(chan struct{})
	m.stateMu.Lock()
	m.running = true
	m.cmd = cmd
	m.pgid = pgid
	m.profileDir = profileDir
	m.debugPort = port
	m.cdpConn = cdpConn
	m.stopCh = stopCh
	m.currentURL = target
	m.pageTitle = ""
	m.canGoBack = false
	m.canGoForward = false
	m.width = width
	m.height = height
	m.screencastPaused = false
	m.lastFrame = ""
	m.lastActivity = time.Now()
	m.mediaMap = make(map[string]SniffedMediaItem)
	m.mediaList = nil
	m.stateMu.Unlock()

	m.ackMu.Lock()
	m.lastScreencastAck = time.Time{}
	m.ackMu.Unlock()

	go m.cdpReadLoop(cdpConn, stopCh)

	// Configure stealth, DOM hooks, screencast, and network listeners.
	_, _ = m.callCDP("Page.enable", nil)
	_, _ = m.callCDP("Network.enable", map[string]interface{}{
		"maxTotalBufferSize":    0,
		"maxResourceBufferSize": 0,
	})
	_, _ = m.callCDP("Runtime.enable", nil)
	_, _ = m.callCDP("Emulation.setDeviceMetricsOverride", map[string]interface{}{
		"width":             width,
		"height":            height,
		"deviceScaleFactor": 1,
		"mobile":            false,
	})
	_, _ = m.callCDP("Page.addScriptToEvaluateOnNewDocument", map[string]interface{}{
		"source": `
			try {
				Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
				Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en', 'vi'] });
				if (!window.chrome) { window.chrome = { runtime: {} }; }
				const origOpen = window.open;
				window.open = function(u) {
					if (u && typeof u === 'string') { location.href = u; return window; }
					return origOpen ? origOpen.apply(this, arguments) : null;
				};
			} catch (e) {}
		`,
	})
	_ = m.startScreencast(width, height)

	m.detectPlayablePageMedia(target, "")
	runningFlag := true
	availFlag := true
	m.broadcast(chromiumEvent{
		Type:      "status",
		Running:   &runningFlag,
		Available: &availFlag,
		URL:       target,
		Width:     width,
		Height:    height,
	})
	return nil
}

func (m *chromiumManager) startScreencast(w, h int) error {
	w, h = clampChromiumViewport(w, h)
	m.stateMu.Lock()
	m.screencastPaused = false
	m.stateMu.Unlock()
	_, err := m.callCDP("Page.startScreencast", map[string]interface{}{
		"format":        "jpeg",
		"quality":       60,
		"maxWidth":      w,
		"maxHeight":     h,
		"everyNthFrame": 5,
	})
	return err
}

func (m *chromiumManager) pauseScreencast() {
	m.stateMu.Lock()
	if !m.running || m.screencastPaused {
		m.stateMu.Unlock()
		return
	}
	m.screencastPaused = true
	m.stateMu.Unlock()
	m.sendCDPAsync("Page.stopScreencast", nil)
}

func (m *chromiumManager) resumeScreencastIfPaused() {
	m.stateMu.RLock()
	running := m.running
	paused := m.screencastPaused
	w, h := m.width, m.height
	m.stateMu.RUnlock()
	if !running || !paused {
		return
	}
	_ = m.startScreencast(w, h)
}

func (m *chromiumManager) stop() {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	m.stateMu.Lock()
	if !m.running && m.cmd == nil && m.cdpConn == nil {
		m.stateMu.Unlock()
		return
	}
	conn := m.cdpConn
	stopCh := m.stopCh
	cmd := m.cmd
	pgid := m.pgid
	profileDir := m.profileDir

	m.running = false
	m.cdpConn = nil
	m.stopCh = nil
	m.cmd = nil
	m.pgid = 0
	m.profileDir = ""
	m.lastFrame = ""
	m.stateMu.Unlock()

	if stopCh != nil {
		select {
		case <-stopCh:
		default:
			close(stopCh)
		}
	}
	if conn != nil {
		_ = conn.Close()
	}
	if pgid > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if profileDir != "" {
		_ = os.RemoveAll(profileDir)
	}

	m.pendingMu.Lock()
	for id, ch := range m.pending {
		delete(m.pending, id)
		select {
		case ch <- cdpRPCResult{Err: errors.New("chromium stopped")}:
		default:
		}
	}
	m.pendingMu.Unlock()

	runningFlag := false
	availFlag := findChromiumBinary() != ""
	m.broadcast(chromiumEvent{
		Type:      "status",
		Running:   &runningFlag,
		Available: &availFlag,
	})
}

// ShutdownChromiumBrowser terminates the idle/active headless Chromium process on server shutdown.
func ShutdownChromiumBrowser() {
	globalChromium.stop()
}

func (m *chromiumManager) callCDP(method string, params map[string]interface{}) (json.RawMessage, error) {
	m.stateMu.RLock()
	conn := m.cdpConn
	running := m.running
	m.stateMu.RUnlock()
	if !running || conn == nil {
		return nil, errors.New("chromium session is not running")
	}

	id := m.msgSeq.Add(1)
	respCh := make(chan cdpRPCResult, 1)
	m.pendingMu.Lock()
	m.pending[id] = respCh
	m.pendingMu.Unlock()

	cleanup := func() {
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
	}

	req := cdpRequest{
		ID:     id,
		Method: method,
		Params: params,
	}

	m.cdpWriteMu.Lock()
	err := conn.WriteJSON(req)
	m.cdpWriteMu.Unlock()
	if err != nil {
		cleanup()
		return nil, err
	}

	select {
	case res := <-respCh:
		return res.Result, res.Err
	case <-time.After(8 * time.Second):
		cleanup()
		return nil, fmt.Errorf("CDP timeout calling %s", method)
	}
}

func (m *chromiumManager) sendCDPAsync(method string, params map[string]interface{}) {
	m.stateMu.RLock()
	conn := m.cdpConn
	running := m.running
	m.stateMu.RUnlock()
	if !running || conn == nil {
		return
	}
	id := m.msgSeq.Add(1)
	m.cdpWriteMu.Lock()
	_ = conn.WriteJSON(cdpRequest{ID: id, Method: method, Params: params})
	m.cdpWriteMu.Unlock()
}

func (m *chromiumManager) cdpReadLoop(conn *websocket.Conn, stopCh chan struct{}) {
	defer func() {
		select {
		case <-stopCh:
		default:
			// Unexpected CDP socket closure: tear down Chromium cleanly so it can restart on demand.
			go m.stop()
		}
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg cdpResponse
		if json.Unmarshal(msgBytes, &msg) != nil {
			continue
		}

		// Dispatch RPC reply
		if msg.ID != 0 {
			m.pendingMu.Lock()
			ch, ok := m.pending[msg.ID]
			if ok {
				delete(m.pending, msg.ID)
			}
			m.pendingMu.Unlock()
			if ok {
				var rpcErr error
				if msg.Error != nil {
					rpcErr = fmt.Errorf("CDP error %d: %s", msg.Error.Code, msg.Error.Message)
				}
				select {
				case ch <- cdpRPCResult{Result: msg.Result, Err: rpcErr}:
				default:
				}
			}
			continue
		}

		// Dispatch CDP events asynchronously where needed
		switch msg.Method {
		case "Page.screencastFrame":
			var ev struct {
				Data      string `json:"data"`
				SessionID int    `json:"sessionId"`
				Metadata  struct {
					DeviceWidth  int `json:"deviceWidth"`
					DeviceHeight int `json:"deviceHeight"`
				} `json:"metadata"`
			}
			if json.Unmarshal(msg.Params, &ev) == nil && ev.Data != "" {
				now := time.Now()
				m.ackMu.Lock()
				delay := nextScreencastAckDelay(now, m.lastScreencastAck, screencastMinInterval)
				m.lastScreencastAck = now.Add(delay)
				m.ackMu.Unlock()

				ackParams := map[string]interface{}{
					"sessionId": ev.SessionID,
				}
				if delay <= 0 {
					m.sendCDPAsync("Page.screencastFrameAck", ackParams)
				} else {
					go func(d time.Duration, stop <-chan struct{}, params map[string]interface{}) {
						timer := time.NewTimer(d)
						defer timer.Stop()
						select {
						case <-timer.C:
							m.sendCDPAsync("Page.screencastFrameAck", params)
						case <-stop:
						}
					}(delay, stopCh, ackParams)
				}

				w := ev.Metadata.DeviceWidth
				h := ev.Metadata.DeviceHeight
				if w <= 0 || h <= 0 {
					m.stateMu.RLock()
					w, h = m.width, m.height
					m.stateMu.RUnlock()
				}
				m.stateMu.Lock()
				m.lastFrame = ev.Data
				m.stateMu.Unlock()
				m.broadcast(chromiumEvent{
					Type:   "frame",
					Data:   ev.Data,
					Width:  w,
					Height: h,
				})
			}

		case "Page.frameNavigated":
			var ev struct {
				Frame struct {
					ParentID string `json:"parentId"`
					URL      string `json:"url"`
				} `json:"frame"`
			}
			if json.Unmarshal(msg.Params, &ev) == nil && ev.Frame.ParentID == "" && ev.Frame.URL != "" {
				go m.handleNavUpdate(ev.Frame.URL)
			}

		case "Page.navigatedWithinDocument":
			var ev struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(msg.Params, &ev) == nil && ev.URL != "" {
				go m.handleNavUpdate(ev.URL)
			}

		case "Page.loadEventFired":
			go func() {
				m.syncNavMetadata()
				m.extractDomMedia()
			}()

		case "Page.windowOpen":
			var ev struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(msg.Params, &ev) == nil && isSupportedHTTPURL(ev.URL) {
				go func() { _ = m.navigate(ev.URL) }()
			}

		case "Network.responseReceived":
			var ev struct {
				Response struct {
					URL      string `json:"url"`
					MimeType string `json:"mimeType"`
					Status   int    `json:"status"`
				} `json:"response"`
			}
			if json.Unmarshal(msg.Params, &ev) == nil {
				m.inspectNetworkMedia(ev.Response.URL, ev.Response.MimeType)
			}

		case "Network.requestWillBeSent":
			var ev struct {
				Request struct {
					URL string `json:"url"`
				} `json:"request"`
			}
			if json.Unmarshal(msg.Params, &ev) == nil {
				m.inspectNetworkMedia(ev.Request.URL, "")
			}
		}
	}
}

func (m *chromiumManager) syncNavMetadata() {
	m.stateMu.RLock()
	currURL := m.currentURL
	m.stateMu.RUnlock()

	title := ""
	if raw, err := m.callCDP("Runtime.evaluate", map[string]interface{}{
		"expression":    "document.title || ''",
		"returnByValue": true,
	}); err == nil {
		var evalRes struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &evalRes) == nil {
			title = strings.TrimSpace(evalRes.Result.Value)
		}
	}

	canBack := false
	canForward := false
	if raw, err := m.callCDP("Page.getNavigationHistory", nil); err == nil {
		var hist struct {
			CurrentIndex int `json:"currentIndex"`
			Entries      []struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"entries"`
		}
		if json.Unmarshal(raw, &hist) == nil && len(hist.Entries) > 0 {
			canBack = hist.CurrentIndex > 0
			canForward = hist.CurrentIndex >= 0 && hist.CurrentIndex < len(hist.Entries)-1
			if hist.CurrentIndex >= 0 && hist.CurrentIndex < len(hist.Entries) {
				if entryURL := strings.TrimSpace(hist.Entries[hist.CurrentIndex].URL); entryURL != "" {
					currURL = entryURL
				}
				if title == "" {
					title = strings.TrimSpace(hist.Entries[hist.CurrentIndex].Title)
				}
			}
		}
	}

	m.stateMu.Lock()
	if currURL != "" {
		m.currentURL = currURL
	}
	if title != "" {
		m.pageTitle = title
	}
	m.canGoBack = canBack
	m.canGoForward = canForward
	m.stateMu.Unlock()

	m.broadcast(chromiumEvent{
		Type:       "nav",
		URL:        currURL,
		Title:      title,
		CanGoBack:  canBack,
		CanForward: canForward,
	})
}

func (m *chromiumManager) handleNavUpdate(newURL string) {
	if !strings.HasPrefix(newURL, "http://") && !strings.HasPrefix(newURL, "https://") {
		return
	}
	m.stateMu.Lock()
	if m.currentURL != newURL {
		m.currentURL = newURL
		// Reset sniffed media list when navigating to a genuinely different page URL
		m.mediaMap = make(map[string]SniffedMediaItem)
		m.mediaList = nil
	}
	m.stateMu.Unlock()

	m.syncNavMetadata()

	m.stateMu.RLock()
	title := m.pageTitle
	m.stateMu.RUnlock()
	m.detectPlayablePageMedia(newURL, title)
}

var (
	ytWatchIDRegex  = regexp.MustCompile(`(?:youtube\.com/watch\?[^#]*v=|youtu\.be/|youtube\.com/shorts/)([a-zA-Z0-9_-]{11})`)
	tiktokVideoRe   = regexp.MustCompile(`tiktok\.com/(?:@[^/?#]+/video/\d+|t/[a-zA-Z0-9]+)`)
)

// detectPlayablePageMedia emits direct downloadable entries for canonical video pages
// (YouTube, TikTok, AV01, Vimeo, Facebook Reels/Watch, Instagram, SoundCloud).
func detectPlayablePageMedia(rawURL, pageTitle string) []SniffedMediaItem {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil
	}
	lower := strings.ToLower(trimmed)
	title := strings.TrimSpace(pageTitle)
	var out []SniffedMediaItem

	if m := ytWatchIDRegex.FindStringSubmatch(trimmed); len(m) == 2 {
		vid := m[1]
		if title == "" || strings.EqualFold(title, "YouTube") {
			title = "YouTube Video " + vid
		}
		out = append(out, SniffedMediaItem{
			ID:        "yt_" + vid,
			Type:      "video",
			Format:    "YOUTUBE",
			Title:     title,
			URL:       "https://www.youtube.com/watch?v=" + vid,
			Thumbnail: fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", vid),
			Channel:   "YouTube",
			Source:    "chromium_page",
		})
	} else if tiktokVideoRe.MatchString(lower) {
		if title == "" {
			title = "TikTok Video"
		}
		out = append(out, SniffedMediaItem{
			ID:      "tt_" + fmt.Sprintf("%x", len(trimmed)),
			Type:    "video",
			Format:  "TIKTOK",
			Title:   title,
			URL:     trimmed,
			Channel: "TikTok",
			Source:  "chromium_page",
		})
	} else if _, _, ok := ParseAV01VideoURL(trimmed); ok {
		if title == "" {
			title = "AV01 Video Stream"
		}
		out = append(out, SniffedMediaItem{
			ID:      "av01_page",
			Type:    "video",
			Format:  "M3U8 HLS",
			Title:   title,
			URL:     trimmed,
			Channel: "AV01",
			Source:  "chromium_page",
		})
	} else if strings.Contains(lower, "vimeo.com/") && regexp.MustCompile(`vimeo\.com/\d+`).MatchString(lower) {
		out = append(out, SniffedMediaItem{
			Type:    "video",
			Format:  "VIMEO",
			Title:   firstNonEmptyString(title, "Vimeo Video"),
			URL:     trimmed,
			Channel: "Vimeo",
			Source:  "chromium_page",
		})
	} else if IsFacebookVideoURL(trimmed) {
		out = append(out, SniffedMediaItem{
			Type:    "video",
			Format:  "FACEBOOK",
			Title:   firstNonEmptyString(title, "Facebook Video"),
			URL:     trimmed,
			Channel: "Facebook",
			Source:  "chromium_page",
		})
	} else if strings.Contains(lower, "instagram.com/reel/") || strings.Contains(lower, "instagram.com/p/") {
		out = append(out, SniffedMediaItem{
			Type:    "video",
			Format:  "INSTAGRAM",
			Title:   firstNonEmptyString(title, "Instagram Media"),
			URL:     trimmed,
			Channel: "Instagram",
			Source:  "chromium_page",
		})
	}
	return out
}

func (m *chromiumManager) detectPlayablePageMedia(rawURL, pageTitle string) {
	items := detectPlayablePageMedia(rawURL, pageTitle)
	for _, it := range items {
		m.addMediaItem(it)
	}
	if _, _, ok := ParseAV01VideoURL(rawURL); ok {
		go func(u string) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			if av, err := ResolveAV01Video(ctx, u); err == nil && av != nil {
				for _, v := range av.Variants {
					m.addMediaItem(SniffedMediaItem{
						ID:         "av01_" + av.VideoID + "_" + v.Label,
						Type:       "video",
						Format:     v.Label + " M3U8 HLS",
						Title:      fmt.Sprintf("%s (%s)", av.Title, v.Label),
						URL:        v.URL,
						Thumbnail:  av.Thumbnail,
						Resolution: v.Label,
						Channel:    "AV01",
						IsStream:   true,
						Source:     "av01_extractor",
					})
				}
			}
		}(rawURL)
	}
}

// isCapturableNetworkMedia determines whether a Chromium network URL/MIME represents a downloadable media stream.
func isCapturableNetworkMedia(rawURL, mimeType string) (mediaType string, formatLabel string, isStream bool, ok bool) {
	uClean := strings.TrimSpace(rawURL)
	if uClean == "" || strings.HasPrefix(uClean, "data:") || strings.HasPrefix(uClean, "blob:") {
		return "", "", false, false
	}
	lower := strings.ToLower(uClean)
	pathOnly := strings.Split(strings.Split(lower, "?")[0], "#")[0]
	mimeLower := strings.ToLower(strings.TrimSpace(mimeType))

	// Ignore noisy HLS/DASH segments, tracking pixels, and temporary YouTube videoplayback range chunks
	// (YouTube canonical watch?v= URL is emitted by detectPlayablePageMedia so yt-dlp muxes audio+video cleanly).
	if strings.HasSuffix(pathOnly, ".ts") ||
		strings.HasSuffix(pathOnly, ".m4s") ||
		strings.HasSuffix(pathOnly, ".vtt") ||
		strings.HasSuffix(pathOnly, ".srt") ||
		strings.Contains(lower, "googlevideo.com/videoplayback") {
		return "", "", false, false
	}

	switch {
	case strings.HasSuffix(pathOnly, ".m3u8") ||
		strings.Contains(mimeLower, "application/vnd.apple.mpegurl") ||
		strings.Contains(mimeLower, "application/x-mpegurl"):
		return "video", "M3U8 HLS", true, true
	case strings.HasSuffix(pathOnly, ".mpd") || strings.Contains(mimeLower, "application/dash+xml"):
		return "video", "DASH MPD", true, true
	case strings.HasSuffix(pathOnly, ".mp4") ||
		strings.HasSuffix(pathOnly, ".m4v") ||
		strings.HasSuffix(pathOnly, ".webm") ||
		strings.Contains(mimeLower, "video/mp4") ||
		strings.Contains(mimeLower, "video/webm") ||
		(strings.Contains(lower, "tiktokcdn") && strings.Contains(lower, "mime_type=video_mp4")):
		return "video", "MP4 VIDEO", false, true
	case strings.HasSuffix(pathOnly, ".mp3") ||
		strings.HasSuffix(pathOnly, ".m4a") ||
		strings.HasSuffix(pathOnly, ".aac") ||
		strings.HasSuffix(pathOnly, ".flac") ||
		strings.Contains(mimeLower, "audio/mpeg") ||
		strings.Contains(mimeLower, "audio/mp4") ||
		strings.Contains(mimeLower, "audio/aac"):
		return "audio", "AUDIO", false, true
	}
	return "", "", false, false
}

func (m *chromiumManager) inspectNetworkMedia(rawURL, mimeType string) {
	mType, fmtLabel, isStream, ok := isCapturableNetworkMedia(rawURL, mimeType)
	if !ok {
		return
	}
	m.stateMu.RLock()
	title := m.pageTitle
	currURL := m.currentURL
	m.stateMu.RUnlock()

	if title == "" {
		base := filepath.Base(strings.Split(rawURL, "?")[0])
		if base == "" || base == "." || base == "/" {
			base = "Captured Stream"
		}
		title = base
	}
	channel := "Browser"
	if parsed, pErr := url.Parse(currURL); pErr == nil && parsed.Hostname() != "" {
		channel = strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	}
	m.addMediaItem(SniffedMediaItem{
		ID:       fmt.Sprintf("cdp_%x", len(rawURL)*31+int(time.Now().Unix()%10000)),
		Type:     mType,
		Format:   fmtLabel,
		Title:    title,
		URL:      sanitizeMediaUrl(rawURL),
		Channel:  channel,
		IsStream: isStream,
		Source:   "chromium_net",
	})
}

func (m *chromiumManager) extractDomMedia() {
	raw, err := m.callCDP("Runtime.evaluate", map[string]interface{}{
		"expression": `(function() {
			var items = [];
			var title = document.title || '';
			document.querySelectorAll('video, audio').forEach(function(el) {
				var kind = el.tagName === 'AUDIO' ? 'audio' : 'video';
				var poster = el.getAttribute('poster') || '';
				if (el.currentSrc && el.currentSrc.indexOf('http') === 0) {
					items.push({type: kind, url: el.currentSrc, thumbnail: poster, title: title});
				}
				if (el.src && el.src.indexOf('http') === 0) {
					items.push({type: kind, url: el.src, thumbnail: poster, title: title});
				}
				el.querySelectorAll('source').forEach(function(s) {
					if (s.src && s.src.indexOf('http') === 0) {
						items.push({type: kind, url: s.src, thumbnail: poster, title: title});
					}
				});
			});
			return JSON.stringify(items.slice(0, 20));
		})()`,
		"returnByValue": true,
	})
	if err != nil {
		return
	}
	var evalRes struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &evalRes) != nil || evalRes.Result.Value == "" {
		return
	}
	var found []struct {
		Type      string `json:"type"`
		URL       string `json:"url"`
		Thumbnail string `json:"thumbnail"`
		Title     string `json:"title"`
	}
	if json.Unmarshal([]byte(evalRes.Result.Value), &found) != nil {
		return
	}
	for _, f := range found {
		if f.URL == "" || strings.Contains(strings.ToLower(f.URL), "googlevideo.com/videoplayback") {
			continue
		}
		m.addMediaItem(SniffedMediaItem{
			ID:        fmt.Sprintf("dom_%x", len(f.URL)*17),
			Type:      firstNonEmptyString(f.Type, "video"),
			Format:    "DOM STREAM",
			Title:     firstNonEmptyString(f.Title, "Detected Video"),
			URL:       f.URL,
			Thumbnail: f.Thumbnail,
			IsStream:  strings.Contains(strings.ToLower(f.URL), ".m3u8") || strings.Contains(strings.ToLower(f.URL), ".mpd"),
			Source:    "chromium_dom",
		})
	}
}

func (m *chromiumManager) addMediaItem(item SniffedMediaItem) {
	norm := normalizeSniffUrl(item.URL)
	if norm == "" {
		return
	}
	item.URL = sanitizeMediaUrl(item.URL)
	if item.ID == "" {
		item.ID = fmt.Sprintf("chr_%d", len(norm))
	}

	m.stateMu.Lock()
	if existing, exists := m.mediaMap[norm]; exists {
		// Upgrade metadata if title or thumbnail improved.
		changed := false
		if item.Title != "" && (existing.Title == "" || strings.HasPrefix(existing.Title, "Captured") || len(item.Title) > len(existing.Title)) {
			existing.Title = item.Title
			changed = true
		}
		if item.Thumbnail != "" && existing.Thumbnail == "" {
			existing.Thumbnail = item.Thumbnail
			changed = true
		}
		if !changed {
			m.stateMu.Unlock()
			return
		}
		m.mediaMap[norm] = existing
		for i := range m.mediaList {
			if normalizeSniffUrl(m.mediaList[i].URL) == norm {
				m.mediaList[i] = existing
				break
			}
		}
		m.stateMu.Unlock()
		m.broadcast(chromiumEvent{Type: "media", Item: existing})
		return
	}

	m.mediaMap[norm] = item
	m.mediaList = append([]SniffedMediaItem{item}, m.mediaList...)
	if len(m.mediaList) > 60 {
		m.mediaList = m.mediaList[:60]
	}
	m.stateMu.Unlock()

	m.broadcast(chromiumEvent{
		Type: "media",
		Item: item,
	})
}

func (m *chromiumManager) navigate(rawURL string) error {
	target := normalizeOmniboxURL(rawURL)
	m.touchActivity()
	m.stateMu.Lock()
	m.mediaMap = make(map[string]SniffedMediaItem)
	m.mediaList = nil
	m.currentURL = target
	title := m.pageTitle
	m.stateMu.Unlock()

	_, err := m.callCDP("Page.navigate", map[string]interface{}{
		"url": target,
	})
	if err == nil {
		m.detectPlayablePageMedia(target, "")
		m.broadcast(chromiumEvent{Type: "nav", URL: target, Title: title})
	}
	return err
}

func (m *chromiumManager) goHistory(delta int) error {
	raw, err := m.callCDP("Page.getNavigationHistory", nil)
	if err != nil {
		return err
	}
	var hist struct {
		CurrentIndex int `json:"currentIndex"`
		Entries      []struct {
			ID  int    `json:"id"`
			URL string `json:"url"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &hist); err != nil {
		return err
	}
	nextIdx := hist.CurrentIndex + delta
	if nextIdx < 0 || nextIdx >= len(hist.Entries) {
		return nil
	}
	_, err = m.callCDP("Page.navigateToHistoryEntry", map[string]interface{}{
		"entryId": hist.Entries[nextIdx].ID,
	})
	return err
}

func (m *chromiumManager) handleAction(act ChromiumAction) error {
	m.touchActivity()

	switch act.Type {
	case "navigate":
		m.stateMu.RLock()
		isUp := m.running && m.cdpConn != nil
		m.stateMu.RUnlock()
		if !isUp {
			return m.ensureStarted(act.URL)
		}
		return m.navigate(act.URL)
	case "stop":
		m.stop()
		return nil
	}

	m.stateMu.RLock()
	running := m.running
	m.stateMu.RUnlock()
	if !running {
		if err := m.ensureStarted(act.URL); err != nil {
			return err
		}
	}

	switch act.Type {
	case "back":
		return m.goHistory(-1)
	case "forward":
		return m.goHistory(1)
	case "reload":
		_, err := m.callCDP("Page.reload", map[string]interface{}{"ignoreCache": false})
		return err
	case "click":
		btn := firstNonEmptyString(act.Button, "left")
		cc := act.ClickCount
		if cc <= 0 {
			cc = 1
		}
		m.sendCDPAsync("Input.dispatchMouseEvent", map[string]interface{}{
			"type":       "mouseMoved",
			"x":          act.X,
			"y":          act.Y,
			"clickCount": 0,
		})
		m.sendCDPAsync("Input.dispatchMouseEvent", map[string]interface{}{
			"type":       "mousePressed",
			"x":          act.X,
			"y":          act.Y,
			"button":     btn,
			"clickCount": cc,
			"modifiers":  act.Modifiers,
		})
		m.sendCDPAsync("Input.dispatchMouseEvent", map[string]interface{}{
			"type":       "mouseReleased",
			"x":          act.X,
			"y":          act.Y,
			"button":     btn,
			"clickCount": cc,
			"modifiers":  act.Modifiers,
		})
		return nil
	case "mousedown":
		btn := firstNonEmptyString(act.Button, "left")
		cc := act.ClickCount
		if cc <= 0 {
			cc = 1
		}
		m.sendCDPAsync("Input.dispatchMouseEvent", map[string]interface{}{
			"type":       "mousePressed",
			"x":          act.X,
			"y":          act.Y,
			"button":     btn,
			"clickCount": cc,
			"modifiers":  act.Modifiers,
		})
		return nil
	case "mouseup":
		btn := firstNonEmptyString(act.Button, "left")
		cc := act.ClickCount
		if cc <= 0 {
			cc = 1
		}
		m.sendCDPAsync("Input.dispatchMouseEvent", map[string]interface{}{
			"type":       "mouseReleased",
			"x":          act.X,
			"y":          act.Y,
			"button":     btn,
			"clickCount": cc,
			"modifiers":  act.Modifiers,
		})
		return nil
	case "mousemove":
		m.sendCDPAsync("Input.dispatchMouseEvent", map[string]interface{}{
			"type": "mouseMoved",
			"x":    act.X,
			"y":    act.Y,
		})
		return nil
	case "wheel":
		m.sendCDPAsync("Input.dispatchMouseEvent", map[string]interface{}{
			"type":   "mouseWheel",
			"x":      act.X,
			"y":      act.Y,
			"deltaX": act.DeltaX,
			"deltaY": act.DeltaY,
		})
		return nil
	case "text":
		if act.Text != "" {
			m.sendCDPAsync("Input.insertText", map[string]interface{}{
				"text": act.Text,
			})
		}
		return nil
	case "keydown":
		params := map[string]interface{}{
			"type":      "keyDown",
			"modifiers": act.Modifiers,
		}
		if act.Key != "" {
			params["key"] = act.Key
		}
		if act.Code != "" {
			params["code"] = act.Code
		}
		if act.KeyCode > 0 {
			params["windowsVirtualKeyCode"] = act.KeyCode
			params["nativeVirtualKeyCode"] = act.KeyCode
		}
		switch act.Key {
		case "Enter":
			params["type"] = "rawKeyDown"
			params["text"] = "\r"
			params["unmodifiedText"] = "\r"
			params["windowsVirtualKeyCode"] = 13
		case "Backspace", "Tab", "Escape", "Delete", "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "Home", "End", "PageUp", "PageDown":
			params["type"] = "rawKeyDown"
		default:
			if act.Text != "" {
				params["text"] = act.Text
				params["unmodifiedText"] = act.Text
			} else if len([]rune(act.Key)) == 1 && (act.Modifiers&^8) == 0 {
				params["text"] = act.Key
				params["unmodifiedText"] = act.Key
			}
		}
		m.sendCDPAsync("Input.dispatchKeyEvent", params)
		return nil
	case "keyup":
		params := map[string]interface{}{
			"type":      "keyUp",
			"modifiers": act.Modifiers,
		}
		if act.Key != "" {
			params["key"] = act.Key
		}
		if act.Code != "" {
			params["code"] = act.Code
		}
		if act.KeyCode > 0 {
			params["windowsVirtualKeyCode"] = act.KeyCode
			params["nativeVirtualKeyCode"] = act.KeyCode
		}
		m.sendCDPAsync("Input.dispatchKeyEvent", params)
		return nil
	case "resize":
		return m.resizeViewport(act.Width, act.Height)
	case "extract":
		go func() {
			m.syncNavMetadata()
			m.extractDomMedia()
			m.stateMu.RLock()
			u, t := m.currentURL, m.pageTitle
			m.stateMu.RUnlock()
			m.detectPlayablePageMedia(u, t)
		}()
		return nil
	}
	return fmt.Errorf("unknown chromium action: %s", act.Type)
}

type cdpCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
}

func formatNetscapeCookies(cookies []cdpCookie) string {
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	for _, c := range cookies {
		if c.Name == "" || c.Domain == "" {
			continue
		}
		includeSub := "FALSE"
		if strings.HasPrefix(c.Domain, ".") {
			includeSub = "TRUE"
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		secureFlag := "FALSE"
		if c.Secure {
			secureFlag = "TRUE"
		}
		exp := int64(c.Expires)
		if exp < 0 {
			exp = 0
		}
		b.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%s\t%s\n", c.Domain, includeSub, path, secureFlag, exp, c.Name, c.Value))
	}
	return b.String()
}

func (m *chromiumManager) exportNetscapeCookies() (string, error) {
	raw, err := m.callCDP("Network.getCookies", nil)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	return formatNetscapeCookies(parsed.Cookies), nil
}

func (m *chromiumManager) resizeViewport(rawW, rawH int) error {
	w, h := clampChromiumViewport(rawW, rawH)
	m.stateMu.Lock()
	diffW := m.width - w
	if diffW < 0 {
		diffW = -diffW
	}
	diffH := m.height - h
	if diffH < 0 {
		diffH = -diffH
	}
	if diffW < 4 && diffH < 4 {
		m.stateMu.Unlock()
		return nil
	}
	m.width = w
	m.height = h
	running := m.running
	m.stateMu.Unlock()

	if !running {
		return nil
	}
	_, _ = m.callCDP("Emulation.setDeviceMetricsOverride", map[string]interface{}{
		"width":             w,
		"height":            h,
		"deviceScaleFactor": 1,
		"mobile":            false,
	})
	_ = m.startScreencast(w, h)
	return nil
}

func applyInitialViewport(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	cw, ch := clampChromiumViewport(w, h)
	globalChromium.stateMu.Lock()
	if !globalChromium.running {
		globalChromium.width = cw
		globalChromium.height = ch
	}
	globalChromium.stateMu.Unlock()
}

// --- HTTP & WebSocket Handlers ---

func ChromiumStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(globalChromium.getStatus())
}

func ChromiumSessionStartHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&body)
	if body.URL == "" {
		body.URL = strings.TrimSpace(r.URL.Query().Get("url"))
	}
	applyInitialViewport(body.Width, body.Height)

	w.Header().Set("Content-Type", "application/json")
	if err := globalChromium.ensureStarted(body.URL); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     false,
			"error":  err.Error(),
			"status": globalChromium.getStatus(),
		})
		return
	}
	if body.Width > 0 && body.Height > 0 {
		_ = globalChromium.resizeViewport(body.Width, body.Height)
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"status": globalChromium.getStatus(),
	})
}

func ChromiumSessionStopHandler(w http.ResponseWriter, r *http.Request) {
	globalChromium.stop()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"status": globalChromium.getStatus(),
	})
}

func ChromiumCookiesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cookies, err := globalChromium.exportNetscapeCookies()
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"cookies": "",
			"error":   err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"cookies": cookies,
	})
}

func ChromiumActionHandler(w http.ResponseWriter, r *http.Request) {
	var act ChromiumAction
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&act); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := globalChromium.handleAction(act); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"status": globalChromium.getStatus(),
	})
}

func (m *chromiumManager) initialSnapshotEvents() []chromiumEvent {
	m.stateMu.RLock()
	running := m.running
	u := m.currentURL
	t := m.pageTitle
	back := m.canGoBack
	fwd := m.canGoForward
	w := m.width
	h := m.height
	frame := m.lastFrame
	itemsCopy := append([]SniffedMediaItem(nil), m.mediaList...)
	m.stateMu.RUnlock()

	avail := findChromiumBinary() != ""
	out := []chromiumEvent{
		{
			Type:      "status",
			Running:   &running,
			Available: &avail,
			URL:       u,
			Title:     t,
			Width:     w,
			Height:    h,
		},
		{
			Type:       "nav",
			URL:        u,
			Title:      t,
			CanGoBack:  back,
			CanForward: fwd,
		},
	}
	if len(itemsCopy) > 0 {
		out = append(out, chromiumEvent{
			Type:  "media_list",
			Items: itemsCopy,
		})
	}
	if frame != "" {
		out = append(out, chromiumEvent{
			Type:   "frame",
			Data:   frame,
			Width:  w,
			Height: h,
		})
	}
	return out
}

func ChromiumEventsHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	qw, qh := parseQueryViewport(r)
	applyInitialViewport(qw, qh)
	if autoURL := strings.TrimSpace(r.URL.Query().Get("url")); autoURL != "" {
		if err := globalChromium.ensureStarted(autoURL); err != nil {
			b, _ := json.Marshal(chromiumEvent{Type: "error", Error: err.Error()})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
	if qw > 0 && qh > 0 {
		_ = globalChromium.resizeViewport(qw, qh)
	}

	subID, ch := globalChromium.subscribe()
	defer globalChromium.unsubscribe(subID)

	for _, ev := range globalChromium.initialSnapshotEvents() {
		b, _ := json.Marshal(ev)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	}
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, open := <-ch:
			if !open {
				return
			}
			b, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func ChromiumWSHandler(w http.ResponseWriter, r *http.Request) {
	wsConn, err := chromiumWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wsConn.Close()

	qw, qh := parseQueryViewport(r)
	applyInitialViewport(qw, qh)
	if autoURL := strings.TrimSpace(r.URL.Query().Get("url")); autoURL != "" {
		if startErr := globalChromium.ensureStarted(autoURL); startErr != nil {
			_ = wsConn.WriteJSON(chromiumEvent{Type: "error", Error: startErr.Error()})
		}
	}
	if qw > 0 && qh > 0 {
		_ = globalChromium.resizeViewport(qw, qh)
	}

	subID, ch := globalChromium.subscribe()
	defer globalChromium.unsubscribe(subID)

	for _, ev := range globalChromium.initialSnapshotEvents() {
		if wErr := wsConn.WriteJSON(ev); wErr != nil {
			return
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, rawMsg, rErr := wsConn.ReadMessage()
			if rErr != nil {
				return
			}
			var act ChromiumAction
			if json.Unmarshal(rawMsg, &act) == nil && act.Type != "" {
				if hErr := globalChromium.handleAction(act); hErr != nil {
					select {
					case ch <- chromiumEvent{Type: "error", Error: hErr.Error()}:
					default:
					}
				}
			}
		}
	}()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-done:
			return
		case <-heartbeat.C:
			_ = wsConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := wsConn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
			_ = wsConn.SetWriteDeadline(time.Time{})
		case ev, open := <-ch:
			if !open {
				return
			}
			if wErr := wsConn.WriteJSON(ev); wErr != nil {
				return
			}
		}
	}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
