package main

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
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
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Range"},
		ExposedHeaders:   []string{"Content-Length", "Content-Range"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	fs := http.FileServer(http.Dir("./outputs"))
	r.Handle("/videos/*", http.StripPrefix("/videos/", fs))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", app.health)
		r.Post("/upload", app.uploadVideo)
		r.Get("/ws", app.wsHandler)
		// r.Get("/videos/{id}", app.serveVideo)
		r.Get("/status", app.ssestatusHandler) // Changed from /status/{id} to /status?id=VIDEO_ID
	})
	return r
}

func (app *application) start(router http.Handler) error {

	srv := http.Server{
		Addr:    app.config.addr,
		Handler: router,
	}
	return srv.ListenAndServe()
}
