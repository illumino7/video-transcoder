package main

import (
	"log/slog"
	"os"

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

	// logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// valkey client
	logger.Info("connecting valkey client to redis", "addr", cfg.redisAddr)
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{cfg.redisAddr},
	})
	if err != nil {
		logger.Error("failed to init valkey client", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	queueMgr := &queue.QueueManager{
		ValkeyClient: client,
	}

	// minio s3
	s3Client, err := storage.NewS3Client(storage.S3Config{
		Endpoint:  env.GetString("MINIO_ENDPOINT", "localhost:9000"),
		AccessKey: env.GetString("MINIO_ACCESS_KEY", "minioadmin"),
		SecretKey: env.GetString("MINIO_SECRET_KEY", "minioadmin"),
		UseSSL:    env.GetBool("MINIO_USE_SSL", false),
		Buckets:   []string{UploadsBucket, StreamingBucket},
	}, logger)
	if err != nil {
		logger.Error("failed to init minio client", "err", err)
		os.Exit(1)
	}
	logger.Info("connected to minio s3")

	app := application{
		config:   cfg,
		logger:   logger,
		queueMgr: queueMgr,
		s3:       s3Client,
	}

	// mux
	mux := app.mount()

	// starting the server
	logger.Info("starting the server", "addr", app.config.addr)
	if err := app.start(mux); err != nil {
		logger.Error("error starting server", "addr", app.config.addr)
	}
}
