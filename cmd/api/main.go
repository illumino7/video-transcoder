package main

import (
	"log/slog"
	"os"

	"github.com/theluminousartemis/video-transcoder/internal/db"
	"github.com/theluminousartemis/video-transcoder/internal/env"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
	"github.com/valkey-io/valkey-go"
)

func main() {
	cfg := config{
		addr:      env.GetString("ADDR", ":3030"),
		redisAddr: env.GetString("REDIS_ADDR", "localhost:6379"),
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("connecting valkey client", "addr", cfg.redisAddr)
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{cfg.redisAddr},
	})
	if err != nil {
		logger.Error("init valkey client", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	queueMgr := &queue.QueueManager{
		ValkeyClient: client,
	}

	s3Client, err := storage.NewS3Client(storage.S3Config{
		Endpoint:  env.GetString("MINIO_ENDPOINT", "localhost:9000"),
		AccessKey: env.GetString("MINIO_ACCESS_KEY", "minioadmin"),
		SecretKey: env.GetString("MINIO_SECRET_KEY", "minioadmin"),
		UseSSL:    env.GetBool("MINIO_USE_SSL", false),
		Buckets:   []string{UploadsBucket, StreamingBucket},
		PublicURL: env.GetString("MINIO_PUBLIC_URL", ""),
	}, logger)
	if err != nil {
		logger.Error("init minio client", "err", err)
		os.Exit(1)
	}
	logger.Info("connected to minio s3")

	dbDSN := env.GetString("DB_DSN", "postgres://postgres:postgres@localhost:5432/transcoder?sslmode=disable")
	logger.Info("connecting database", "dsn", dbDSN)
	dbConn, err := db.OpenDB(dbDSN)
	if err != nil {
		logger.Error("init database connection pool", "err", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	store := db.NewStorage(dbConn)

	app := application{
		config:   cfg,
		logger:   logger,
		queueMgr: queueMgr,
		s3:       s3Client,
		store:    store,
	}

	mux := app.mount()

	logger.Info("starting server", "addr", app.config.addr)
	if err := app.start(mux); err != nil {
		logger.Error("start server", "addr", app.config.addr, "err", err)
	}
}
