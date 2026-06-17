package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/theluminousartemis/video-transcoder/internal/db"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/theluminousartemis/video-transcoder/internal/storage"
)

type config struct {
	redisAddr   string
	concurrency int
}

type application struct {
	logger   *slog.Logger
	config   *config
	queueMgr queue.QueueManager
	s3       *storage.S3Client
	store    *db.Storage
}

// runValkeyWorker processes incoming transcode messages from Valkey stream.
func runValkeyWorker(ctx context.Context, app *application, consumerName string, wg *sync.WaitGroup) {
	defer wg.Done()
	vClient := app.queueMgr.ValkeyClient
	app.logger.Info("started valkey worker", "consumer", consumerName)

	app.logger.Info("draining PEL for crashed jobs", "consumer", consumerName)
	for {
		if ctx.Err() != nil {
			return
		}
		pelCmd := vClient.B().Xreadgroup().Group(queue.ConsumerGroup, consumerName).Count(10).Streams().Key(queue.StreamTranscode).Id("0").Build()
		pelMap, err := vClient.Do(ctx, pelCmd).AsXRead()
		if err != nil {
			app.logger.Error("failed to read pending task list (PEL) during startup", "consumer", consumerName, "err", err)
			break
		}

		hasMessages := false
		for _, msgs := range pelMap {
			if len(msgs) == 0 {
				continue
			}
			hasMessages = true
			for _, msgData := range msgs {
				payloadBytes := []byte(msgData.FieldValues["payload"])
				if err := app.HandleTranscodeTask(ctx, payloadBytes); err != nil {
					app.logger.Error("failed to process pending task from PEL", "msg_id", msgData.ID, "err", err)
					continue
				}
				if err := vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgData.ID).Build()).Error(); err != nil {
					app.logger.Error("failed to acknowledge task from PEL", "msg_id", msgData.ID, "err", err)
				}
				if err := vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgData.ID).Build()).Error(); err != nil {
					app.logger.Error("failed to delete retry hash for PEL task", "msg_id", msgData.ID, "err", err)
				}
			}
		}
		if !hasMessages {
			break
		}
	}
	app.logger.Info("PEL drained, listening for new messages", "consumer", consumerName)

	for {
		select {
		case <-ctx.Done():
			app.logger.Info("worker shutting down", "consumer", consumerName)
			return
		default:
			cmd := vClient.B().Xreadgroup().Group(queue.ConsumerGroup, consumerName).Count(1).Block(0).Streams().Key(queue.StreamTranscode).Id(">").Build()
			streamsMap, err := vClient.Do(ctx, cmd).AsXRead()
			if err != nil {
				if ctx.Err() == nil {
					app.logger.Error("failed to read from queue stream", "err", err, "consumer", consumerName)
					time.Sleep(2 * time.Second)
				}
				continue
			}

			for _, msgs := range streamsMap {
				for _, msgData := range msgs {
					payloadBytes := []byte(msgData.FieldValues["payload"])

					err := app.HandleTranscodeTask(ctx, payloadBytes)
					if err != nil {
						app.logger.Error("transcoding task execution failed", "msg_id", msgData.ID, "err", err)
						continue
					}

					ackCmd := vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgData.ID).Build()
					if err := vClient.Do(ctx, ackCmd).Error(); err != nil {
						app.logger.Error("failed to acknowledge completed stream task", "msg_id", msgData.ID, "err", err)
					}

					hdelCmd := vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgData.ID).Build()
					if err := vClient.Do(ctx, hdelCmd).Error(); err != nil {
						app.logger.Error("failed to delete retry count hash for task", "msg_id", msgData.ID, "err", err)
					}
				}
			}
		}
	}
}

// runValkeyJanitor claims stale pending messages and handles poisoned tasks.
func runValkeyJanitor(ctx context.Context, app *application, wg *sync.WaitGroup) {
	defer wg.Done()
	vClient := app.queueMgr.ValkeyClient
	app.logger.Info("started valkey janitor")

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			app.logger.Info("janitor shutting down")
			return
		case <-ticker.C:
			cmd := vClient.B().Xpending().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Idle(900000).Start("-").End("+").Count(100).Build()
			res, err := vClient.Do(ctx, cmd).ToArray()
			if err != nil && err.Error() != "redis: nil" {
				app.logger.Error("janitor failed to check pending messages", "err", err)
				continue
			}

			for _, p := range res {
				pArr, err := p.ToArray()
				if err != nil || len(pArr) == 0 {
					continue
				}
				
				msgID, err := pArr[0].ToString()
				if err != nil {
					app.logger.Error("janitor failed to convert message ID to string", "err", err)
					continue
				}
				
				retryCmd := vClient.B().Hincrby().Key(queue.HashTranscodeRetries).Field(msgID).Increment(1).Build()
				retries, err := vClient.Do(ctx, retryCmd).AsInt64()
				if err != nil {
					app.logger.Error("janitor failed to increment task retry counter", "msg_id", msgID, "err", err)
					continue
				}
				
				if retries <= 3 {
					app.logger.Info("janitor claiming message for retry", "id", msgID, "retries", retries)
					claimCmd := vClient.B().Xclaim().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Consumer("janitor").MinIdleTime("900000").Id(msgID).Build()
					claimed, err := vClient.Do(ctx, claimCmd).AsXRange()
					if err != nil {
						app.logger.Error("janitor failed to claim stale task", "msg_id", msgID, "err", err)
						continue
					}
					
					for _, msg := range claimed {
						payloadBytes := []byte(msg.FieldValues["payload"])
						
						err := app.HandleTranscodeTask(ctx, payloadBytes)
						if err == nil {
							if err := vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgID).Build()).Error(); err != nil {
								app.logger.Error("janitor failed to acknowledge claimed task", "msg_id", msgID, "err", err)
							}
							if err := vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgID).Build()).Error(); err != nil {
								app.logger.Error("janitor failed to delete retry hash for claimed task", "msg_id", msgID, "err", err)
							}
						} else {
							app.logger.Error("janitor failed to process claimed task", "id", msgID, "err", err)
						}
					}
				} else {
					app.logger.Warn("janitor moving poisoned message to DLQ", "id", msgID)
					
					readIdCmd := vClient.B().Xrange().Key(queue.StreamTranscode).Start(msgID).End(msgID).Build()
					messages, err := vClient.Do(ctx, readIdCmd).AsXRange()
					if err != nil {
						app.logger.Error("janitor failed to fetch task payload from stream for DLQ relocation", "msg_id", msgID, "err", err)
						continue
					}
					
					if len(messages) > 0 {
						payloadStr := messages[0].FieldValues["payload"]
						
						dlqCmd := vClient.B().Xadd().Key(queue.StreamTranscodeDLQ).Id("*").FieldValue().FieldValue("payload", payloadStr).Build()
						if err := vClient.Do(ctx, dlqCmd).Error(); err != nil {
							app.logger.Error("janitor failed to publish task to DLQ", "msg_id", msgID, "err", err)
						}
					}
					
					if err := vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgID).Build()).Error(); err != nil {
						app.logger.Error("janitor failed to acknowledge poisoned task", "msg_id", msgID, "err", err)
					}
					if err := vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgID).Build()).Error(); err != nil {
						app.logger.Error("janitor failed to delete retry hash for poisoned task", "msg_id", msgID, "err", err)
					}
				}
			}
		}
	}
}
