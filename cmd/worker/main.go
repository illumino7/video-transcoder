package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/theluminousartemis/video-transcoder/internal/db"
	"github.com/theluminousartemis/video-transcoder/internal/env"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
	"github.com/valkey-io/valkey-go"
)

const (
	UploadsBucket   = "uploads"
	StreamingBucket = "streaming"
)

func main() {
	config := &config{
		redisAddr:   env.GetString("REDIS_ADDR", "localhost:6379"),
		concurrency: env.GetInt("WORKER_CONCURRENCY", 2),
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("connecting valkey client", "addr", config.redisAddr)
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{config.redisAddr},
	})
	if err != nil {
		logger.Error("init valkey client", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	s3Client, err := storage.NewS3Client(storage.S3Config{
		Endpoint:  env.GetString("MINIO_ENDPOINT", "localhost:9000"),
		AccessKey: env.GetString("MINIO_ACCESS_KEY", "minioadmin"),
		SecretKey: env.GetString("MINIO_SECRET_KEY", "minioadmin"),
		UseSSL:    env.GetBool("MINIO_USE_SSL", false),
		Buckets:   []string{UploadsBucket, StreamingBucket},
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
		logger.Error("init database", "err", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	store := db.NewStorage(dbConn)

	queueMgr := queue.QueueManager{
		ValkeyClient: client,
	}

	app := application{
		logger:   logger,
		config:   config,
		queueMgr: queueMgr,
		s3:       s3Client,
		store:    store,
	}

	ctx := context.Background()
	_ = client.Do(ctx, client.B().XgroupCreate().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id("0").Mkstream().Build())

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	hostname, _ := os.Hostname()

	for i := 0; i < config.concurrency; i++ {
		consumerName := fmt.Sprintf("%s-worker-%d", hostname, i)
		wg.Add(1)
		go runValkeyWorker(ctx, &app, consumerName, &wg)
	}

	wg.Add(1)
	go runValkeyJanitor(ctx, &app, &wg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down workers...")
	cancel()
	wg.Wait()
	logger.Info("workers stopped")
}
