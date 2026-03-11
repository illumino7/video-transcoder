package main

import (
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
)

type config struct {
	redisAddr     string
	redisqueueCfg asynqConfig
	redisCfg      redisConfig
}

type asynqConfig struct {
	Concurrency int
	Queues      map[string]int
}

type redisConfig struct {
	addr     string
	password string
	db       int
}

type application struct {
	logger   *slog.Logger
	config   *config
	queueMgr queue.QueueManager
	rdb      *redis.Client
	s3       *storage.S3Client
}

func runAsynqWorker(app *application) error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeVideoTranscode, app.HandleTranscodeTask)
	if err := app.queueMgr.AsynqServer.Run(mux); err != nil {
		return err
	}
	return nil
}
