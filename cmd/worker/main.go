package main

import (
	"log/slog"
	"os"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/theluminousartemis/video-transcoder/internal/env"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
)

const (
	UploadsBucket   = "uploads"
	StreamingBucket = "streaming"
)

func main() {
	config := &config{
		redisAddr: env.GetString("REDIS_ADDR", "localhost:6379"),
		redisqueueCfg: asynqConfig{
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
		redisCfg: redisConfig{
			addr:     env.GetString("REDIS_ADDR", "localhost:6379"),
			password: env.GetString("REDIS_PASSWORD", ""),
			db:       env.GetInt("REDIS_DB", 0),
		},
	}

	// logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// redis queue
	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: config.redisAddr},
		asynq.Config{
			Concurrency: config.redisqueueCfg.Concurrency,
			Queues:      config.redisqueueCfg.Queues,
		},
	)
	logger.Info("successfully connected to redis")

	// redis pubsub
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.redisCfg.addr,
		Password: config.redisCfg.password,
		DB:       config.redisCfg.db,
	})

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

	// queue manager
	queueMgr := queue.QueueManager{
		AsynqClient: nil,
		AsynqServer: server,
	}

	app := application{
		logger:   logger,
		config:   config,
		queueMgr: queueMgr,
		rdb:      rdb,
		s3:       s3Client,
	}

	if err := runAsynqWorker(&app); err != nil {
		logger.Error(err.Error())
	}
}
