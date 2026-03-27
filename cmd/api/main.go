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

	// Initialize structured logging for the API layer to output in standard text format.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Connect to the Valkey (Redis-compatible) cluster. The API leverages Valkey for 
	// dispatching transcode jobs downstream and subscribing to completion events.
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

	// Initialize the S3 client to generate presigned upload URLs for user files.
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

	// Mount the router with all registered routes and middleware attached.
	mux := app.mount()

	// Launch the HTTP server on the configured address. This call will block indefinitely.
	logger.Info("starting the server", "addr", app.config.addr)
	if err := app.start(mux); err != nil {
		logger.Error("error starting server", "addr", app.config.addr)
	}
}
