package main

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
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
	asynqCfg  asynqConfig
	redisCfg  redisConfig
}

type application struct {
	config   config
	logger   *slog.Logger
	queueMgr *queue.QueueManager
	rdb      *redis.Client
	s3       *storage.S3Client
}

type redisConfig struct {
	addr     string
	password string
	db       int
}

type asynqConfig struct {
	Concurrency int
	Queues      map[string]int
}

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

	// Proxy HLS content from MinIO instead of serving from local filesystem
	r.Get("/videos/*", app.serveFromS3)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", app.health)
		r.Get("/upload/presign", app.presignUpload)
		r.Post("/upload", app.uploadComplete)
		r.Get("/status", app.ssestatusHandler)
	})
	return r
}

func (app *application) serveFromS3(w http.ResponseWriter, r *http.Request) {
	// Strip "/videos/" prefix to get the S3 object key
	objectKey := strings.TrimPrefix(r.URL.Path, "/videos/")
	if objectKey == "" {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// Get object info for headers from the streaming bucket
	info, err := app.s3.StatObject(ctx, StreamingBucket, objectKey)
	if err != nil {
		app.logger.Error("s3 stat failed", "bucket", StreamingBucket, "key", objectKey, "err", err)
		http.NotFound(w, r)
		return
	}

	// Get the object
	obj, err := app.s3.GetObject(ctx, StreamingBucket, objectKey)
	if err != nil {
		app.logger.Error("s3 get failed", "bucket", StreamingBucket, "key", objectKey, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer obj.Close()

	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	io.Copy(w, obj)
}

func (app *application) start(router http.Handler) error {
	srv := http.Server{
		Addr:    app.config.addr,
		Handler: router,
	}
	return srv.ListenAndServe()
}
