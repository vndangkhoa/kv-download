package main

import (
	"context"
	"errors"
	"kv-download/src/media"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func main() {
	// setup routes
	router := chi.NewRouter()
	// Setup Next.js Frontend Static Export or legacy fallback
	outDir, _ := filepath.Abs("frontend/out")
	hasFrontendOut := false
	if _, err := os.Stat(outDir); err == nil {
		hasFrontendOut = true
		fileServer(router, "/_next", filepath.Join(outDir, "_next"))
	}

	if !hasFrontendOut {
		router.Get("/", media.Index)
		router.Head("/", media.Index)
	}

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK\n"))
	})
	router.Get("/fetch", media.FetchMedia)
	router.Get("/api/download", media.FetchMediaApi)
	router.Get("/api/info", media.FetchMediaInfo)
	router.Get("/api/scan", media.ScanMediaApi)
	router.Get("/api/scan/stream", media.StreamScanMediaApi)
	router.Get("/api/youtube/search", media.YouTubeSearchHandler)
	router.Get("/api/ytdlp/version", media.YtDlpVersionHandler)
	router.Post("/api/ytdlp/update", media.YtDlpUpdateHandler)
	router.Get("/api/app/version", media.AppVersionHandler)
	router.Get("/api/events", media.EventsHandler)
	router.Get("/api/queue", media.QueueListHandler)
	router.Post("/api/queue/add", media.QueueAddHandler)
	router.Post("/api/queue/cancel", media.QueueCancelHandler)
	router.Post("/api/queue/retry", media.QueueRetryHandler)
	router.Post("/api/queue/clear-completed", media.QueueClearCompletedHandler)
	router.Post("/api/queue/retry-failed", media.QueueRetryFailedHandler)
	router.Post("/api/queue/group/cancel", media.QueueGroupCancelHandler)
	router.Post("/api/queue/group/retry", media.QueueGroupRetryHandler)
	router.Delete("/api/queue/group", media.QueueGroupDeleteHandler)
	router.Get("/api/queue/workers", media.QueueGetWorkersHandler)
	router.Post("/api/queue/workers", media.QueueSetWorkersHandler)
	router.Delete("/api/queue/item", media.QueueDeleteHandler)
	router.Get("/api/gallery", media.GalleryHandler)
	router.Post("/api/gallery/organize", media.GalleryOrganizeHandler)
	router.Delete("/api/gallery/file", media.GalleryDeleteHandler)
	router.Post("/api/gallery/delete-multiple", media.GalleryDeleteMultipleHandler)
	router.Get("/api/settings/storage", media.GetStorageSettingHandler)
	router.Post("/api/settings/storage", media.SetStorageSettingHandler)
	router.Get("/download/zip", media.DownloadAllZip)
	router.Get("/download", media.ServeMedia)
	router.Head("/download", media.ServeMedia)
	router.Get("/thumbnail", media.ServeThumbnailHandler)
	router.Head("/thumbnail", media.ServeThumbnailHandler)
	router.Get("/api/thumbnail", media.ServeThumbnailHandler)
	router.Head("/api/thumbnail", media.ServeThumbnailHandler)
	router.Get("/api/browser/proxy", media.BrowserProxyHandler)
	router.Get("/api/browser/sniff", media.BrowserSniffHandler)
	router.Get("/api/browser/resolve", media.BrowserResolveHandler)
	router.Get("/api/browser/stream", media.BrowserStreamHandler)
	router.Get("/api/browser/proxy-media", media.BrowserProxyMediaHandler)
	router.Head("/api/browser/stream", media.BrowserStreamHandler)
	router.Head("/api/browser/proxy-media", media.BrowserProxyMediaHandler)
	router.HandleFunc("/jsonrpc", media.Aria2JsonRpcHandler)
	router.HandleFunc("/rpc", media.Aria2JsonRpcHandler)
	fileServer(router, "/static", "static/")

	// Serve favicon.ico at root (browsers request this by default)
	router.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})

	if hasFrontendOut {
		spaHandler := func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				http.ServeFile(w, r, filepath.Join(outDir, "index.html"))
				return
			}
			fullPath := filepath.Join(outDir, path)
			if fi, err := os.Stat(fullPath); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, fullPath)
				return
			}
			if fi, err := os.Stat(fullPath + ".html"); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, fullPath+".html")
				return
			}
			if fi, err := os.Stat(filepath.Join(fullPath, "index.html")); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, filepath.Join(fullPath, "index.html"))
				return
			}
			// Fallback to index.html for client-side routing
			http.ServeFile(w, r, filepath.Join(outDir, "index.html"))
		}
		router.Get("/", spaHandler)
		router.Head("/", spaHandler)
		router.Get("/*", spaHandler)
		router.Head("/*", spaHandler)
	}

	// Print out all routes
	walkFunc := func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		log.Info().Msgf("%s %s", method, route)
		return nil
	}
	// Panic if there is an error
	if err := chi.Walk(router, walkFunc); err != nil {
		log.Panic().Msgf("%s\n", err.Error())
	}

	media.GetInstalledVersion()
	go startYtDlpUpdater()
	go startCookieRefresher()
	go media.StartInfoCachePruner()

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = strings.TrimSpace(os.Getenv("MR_PORT"))
	}
	if port == "" {
		port = "9292"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	log.Info().Msgf("KV Download web server listening on http://localhost%s", port)

	// The HTTP Server
	server := &http.Server{Addr: port, Handler: router}

	// Server run context
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	// Listen for syscall signals for process to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig

		// Shutdown signal with grace period of 30 seconds
		shutdownCtx, cancel := context.WithTimeout(serverCtx, 30*time.Second)
		defer cancel()

		go func() {
			<-shutdownCtx.Done()
			if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
				log.Fatal().Msg("graceful shutdown timed out.. forcing exit.")
			}
		}()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal().Err(err)
		}
		serverStopCtx()
	}()

	// Run the server
	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal().Err(err)
	}

	// Wait for server context to be stopped
	<-serverCtx.Done()
	log.Info().Msgf("Shutdown complete")
}

// startYtDlpUpdater will update the yt-dlp to the latest nightly version ever few hours
func startYtDlpUpdater() {
	log.Info().Msgf("yt-dlp version: %s", media.GetInstalledVersion())
	ticker := time.NewTicker(6 * time.Hour)

	// Do one update now
	_ = media.UpdateYtDlp()

	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				_ = media.UpdateYtDlp()
				log.Info().Msgf("yt-dlp version: %s", media.GetInstalledVersion())
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
}

// startCookieRefresher auto-generates/refreshes cookies.txt from the configured
// browser (MR_COOKIES_BROWSER) if it is missing or stale, then again every 24h.
func startCookieRefresher() {
	media.RefreshCookiesFromBrowser()

	ticker := time.NewTicker(24 * time.Hour)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				media.RefreshCookiesFromBrowser()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
}

func fileServer(r chi.Router, public string, static string) {
	if strings.ContainsAny(public, "{}*") {
		panic("FileServer does not permit URL parameters.")
	}

	root, _ := filepath.Abs(static)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		panic("Static Documents Directory Not Found")
	}

	fs := http.StripPrefix(public, http.FileServer(http.Dir(root)))

	if public != "/" && public[len(public)-1] != '/' {
		r.Get(public, http.RedirectHandler(public+"/", http.StatusMovedPermanently).ServeHTTP)
		public += "/"
	}

	staticHandler := func(w http.ResponseWriter, r *http.Request) {
		cleanPublic := strings.TrimSuffix(public, "/")
		filePath := strings.TrimPrefix(r.URL.Path, cleanPublic)
		fullPath := filepath.Join(root, filePath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	}

	r.Get(public+"*", staticHandler)
	r.Head(public+"*", staticHandler)
}
