package main

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
)

const (
	UploadsBucket   = "uploads"
	StreamingBucket = "streaming"
)

type config struct {
	addr      string
	redisAddr string
}

type application struct {
	config   config
	logger   *slog.Logger
	queueMgr *queue.QueueManager
	s3       *storage.S3Client
}

// mount composes the application's routing tree. It binds required middleware
// such as CORS, Request IDs, and panic recovery, and registers the API v1 endpoints.
func (app *application) mount() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Range"},
		ExposedHeaders:   []string{"Content-Length", "Content-Range"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", app.health)
		r.Get("/upload/presign", app.presignUpload)
		r.Post("/upload", app.uploadComplete)
		r.Get("/status", app.ssestatusHandler)
	})
	return r
}

// start spins up an HTTP server with the provided router.
func (app *application) start(router http.Handler) error {
	srv := http.Server{
		Addr:    app.config.addr,
		Handler: router,
	}
	return srv.ListenAndServe()
}
