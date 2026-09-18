package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
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

// handleSuccessTask acknowledges and deletes the task from stream, and cleans up retry state.
func (app *application) handleSuccessTask(ctx context.Context, msgID string, payloadBytes []byte) {
	vClient := app.queueMgr.ValkeyClient

	var payload queue.TranscodePayload
	if err := json.Unmarshal(payloadBytes, &payload); err == nil && payload.VideoID != "" {
		_ = vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(payload.VideoID).Build())
	}

	ackCmd := vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgID).Build()
	if err := vClient.Do(ctx, ackCmd).Error(); err != nil {
		app.logger.Error("failed to acknowledge completed stream task", "msg_id", msgID, "err", err)
	}

	delCmd := vClient.B().Xdel().Key(queue.StreamTranscode).Id(msgID).Build()
	if err := vClient.Do(ctx, delCmd).Error(); err != nil {
		app.logger.Error("failed to delete completed stream task", "msg_id", msgID, "err", err)
	}
}

// handleFailedTask manages retries with exponential backoff or relocates poisoned tasks to DLQ.
func (app *application) handleFailedTask(ctx context.Context, msgID string, payloadBytes []byte, taskErr error) {
	vClient := app.queueMgr.ValkeyClient

	var payload queue.TranscodePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		app.logger.Error("failed to unmarshal task payload on failure handling", "msg_id", msgID, "err", err)
		return
	}

	videoID := payload.VideoID
	retryCmd := vClient.B().Hincrby().Key(queue.HashTranscodeRetries).Field(videoID).Increment(1).Build()
	retries, err := vClient.Do(ctx, retryCmd).AsInt64()
	if err != nil {
		app.logger.Error("failed to increment retry counter", "video_id", videoID, "err", err)
		return
	}

	if retries <= 3 {
		// Exponential backoff: attempt 1 -> 15s, attempt 2 -> 30s, attempt 3 -> 60s
		backoffSeconds := int64(15 * (1 << (retries - 1)))
		scheduledTime := time.Now().Unix() + backoffSeconds

		app.logger.Warn("scheduling task for retry with exponential backoff",
			"video_id", videoID,
			"retries", retries,
			"backoff_sec", backoffSeconds,
			"err", taskErr)

		zaddCmd := vClient.B().Zadd().Key(queue.ZSetTranscodeDelayed).ScoreMember().
			ScoreMember(float64(scheduledTime), string(payloadBytes)).Build()
		if err := vClient.Do(ctx, zaddCmd).Error(); err != nil {
			app.logger.Error("failed to add task to delayed retry set", "video_id", videoID, "err", err)
		}

		_ = vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgID).Build())
		_ = vClient.Do(ctx, vClient.B().Xdel().Key(queue.StreamTranscode).Id(msgID).Build())
	} else {
		app.logger.Error("task exceeded maximum retries, moving to DLQ", "video_id", videoID, "retries", retries, "err", taskErr)

		// 1. Publish to DLQ with rich diagnostic metadata
		dlqCmd := vClient.B().Xadd().Key(queue.StreamTranscodeDLQ).Id("*").FieldValue().
			FieldValue("payload", string(payloadBytes)).
			FieldValue("video_id", videoID).
			FieldValue("error", taskErr.Error()).
			FieldValue("failed_at", time.Now().UTC().Format(time.RFC3339)).
			FieldValue("retry_count", strconv.FormatInt(retries, 10)).
			FieldValue("original_msg_id", msgID).
			Build()
		if err := vClient.Do(ctx, dlqCmd).Error(); err != nil {
			app.logger.Error("failed to publish task to DLQ", "video_id", videoID, "err", err)
		}

		// 2. Mark database status as FAILED
		if err := app.store.Videos.UpdateStatus(ctx, videoID, "FAILED"); err != nil {
			app.logger.Error("failed to update video status to FAILED in DB", "video_id", videoID, "err", err)
		}

		// 3. Broadcast FAILED status to SSE Pub/Sub
		app.broadcastStatus(ctx, videoID, "FAILED")

		// 4. Acknowledge and delete from active stream
		_ = vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgID).Build())
		_ = vClient.Do(ctx, vClient.B().Xdel().Key(queue.StreamTranscode).Id(msgID).Build())

		// 5. Clean up retry counter
		_ = vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(videoID).Build())
	}
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
					app.handleFailedTask(ctx, msgData.ID, payloadBytes, err)
					continue
				}
				app.handleSuccessTask(ctx, msgData.ID, payloadBytes)
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

					if err := app.HandleTranscodeTask(ctx, payloadBytes); err != nil {
						app.logger.Error("transcoding task execution failed", "msg_id", msgData.ID, "err", err)
						app.handleFailedTask(ctx, msgData.ID, payloadBytes, err)
						continue
					}

					app.handleSuccessTask(ctx, msgData.ID, payloadBytes)
				}
			}
		}
	}
}

// runValkeyJanitor handles delayed retry re-enqueueing and reaps abandoned tasks.
func runValkeyJanitor(ctx context.Context, app *application, wg *sync.WaitGroup) {
	defer wg.Done()
	vClient := app.queueMgr.ValkeyClient
	app.logger.Info("started valkey janitor and delayed retry scheduler")

	delayedTicker := time.NewTicker(5 * time.Second)
	defer delayedTicker.Stop()

	// Check PEL for abandoned tasks idle for > 20 minutes (1,200,000 ms)
	pelTicker := time.NewTicker(1 * time.Minute)
	defer pelTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			app.logger.Info("janitor shutting down")
			return

		case <-delayedTicker.C:
			// 1. Check delayed ZSET for tasks whose backoff score has elapsed
			nowStr := strconv.FormatInt(time.Now().Unix(), 10)
			zrangeCmd := vClient.B().Zrange().Key(queue.ZSetTranscodeDelayed).Min("-inf").Max(nowStr).Byscore().Build()
			dueTasks, err := vClient.Do(ctx, zrangeCmd).AsStrSlice()
			if err != nil {
				if err.Error() != "redis: nil" {
					app.logger.Error("failed to query delayed transcode set", "err", err)
				}
				continue
			}

			for _, payloadStr := range dueTasks {
				// Atomically claim the task from the delayed set first.
				// If multiple worker instances run concurrently, only the single winner (removed == 1) proceeds to re-enqueue.
				remCmd := vClient.B().Zrem().Key(queue.ZSetTranscodeDelayed).Member(payloadStr).Build()
				removed, err := vClient.Do(ctx, remCmd).AsInt64()
				if err != nil || removed == 0 {
					continue
				}

				addCmd := vClient.B().Xadd().Key(queue.StreamTranscode).Id("*").
					FieldValue().FieldValue("payload", payloadStr).Build()
				if err := vClient.Do(ctx, addCmd).Error(); err != nil {
					app.logger.Error("failed to re-enqueue delayed task to stream", "err", err)
					// Rollback: put it back into the delayed set so the task is not dropped
					_ = vClient.Do(ctx, vClient.B().Zadd().Key(queue.ZSetTranscodeDelayed).ScoreMember().ScoreMember(float64(time.Now().Unix()+5), payloadStr).Build())
					continue
				}

				app.logger.Info("re-enqueued delayed task to main stream for execution", "payload", payloadStr)
			}

		case <-pelTicker.C:
			// 2. Reap truly abandoned tasks idle for > 20 minutes (1,200,000 ms) from crashed workers
			cmd := vClient.B().Xpending().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Idle(1200000).Start("-").End("+").Count(100).Build()
			res, err := vClient.Do(ctx, cmd).ToArray()
			if err != nil && err.Error() != "redis: nil" {
				app.logger.Error("janitor failed to check pending messages in PEL", "err", err)
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

				// Atomically claim ownership with XCLAIM.
				// XCLAIM resets idle time to 0, ensuring no other concurrent janitor replica can double-claim it.
				claimCmd := vClient.B().Xclaim().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Consumer("janitor").MinIdleTime("1200000").Id(msgID).Build()
				claimed, err := vClient.Do(ctx, claimCmd).AsXRange()
				if err != nil || len(claimed) == 0 {
					continue
				}

				payloadStr := claimed[0].FieldValues["payload"]
				payloadBytes := []byte(payloadStr)

				// Delegate failure/retry/DLQ routing without blocking the janitor with FFmpeg
				app.logger.Warn("janitor detected and claimed abandoned task from crashed worker", "msg_id", msgID)
				app.handleFailedTask(ctx, msgID, payloadBytes, fmt.Errorf("worker process died or abandoned task"))
			}
		}
	}
}
