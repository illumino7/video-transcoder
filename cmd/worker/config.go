package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

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
}

func runValkeyWorker(ctx context.Context, app *application, consumerName string, wg *sync.WaitGroup) {
	defer wg.Done()
	vClient := app.queueMgr.ValkeyClient
	app.logger.Info("started valkey worker", "consumer", consumerName)

	// Phase 1: Drain our own Pending Entries List (PEL) for jobs we crashed on.
	// Use ID "0" with NO blocking — Valkey returns immediately with pending msgs or empty.
	app.logger.Info("draining PEL for crashed jobs", "consumer", consumerName)
	for {
		if ctx.Err() != nil {
			return
		}
		pelCmd := vClient.B().Xreadgroup().Group(queue.ConsumerGroup, consumerName).Count(10).Streams().Key(queue.StreamTranscode).Id("0").Build()
		pelMap, err := vClient.Do(ctx, pelCmd).AsXRead()
		if err != nil {
			// Any error (including nil response) means PEL is empty or doesn't exist yet
			break
		}

		// Check if we actually got messages
		hasMessages := false
		for _, msgs := range pelMap {
			if len(msgs) == 0 {
				continue
			}
			hasMessages = true
			for _, msgData := range msgs {
				payloadBytes := []byte(msgData.FieldValues["payload"])
				if err := app.HandleTranscodeTask(ctx, payloadBytes); err != nil {
					app.logger.Error("PEL task failed", "msgId", msgData.ID, "err", err)
					continue
				}
				vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgData.ID).Build())
				vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgData.ID).Build())
			}
		}
		if !hasMessages {
			break
		}
	}
	app.logger.Info("PEL drained, listening for new messages", "consumer", consumerName)

	// Phase 2: Listen for new messages with blocking reads using ID ">".
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
					app.logger.Error("failed to read from stream", "err", err, "consumer", consumerName)
					time.Sleep(2 * time.Second)
				}
				continue
			}

			for _, msgs := range streamsMap {
				for _, msgData := range msgs {
					payloadBytes := []byte(msgData.FieldValues["payload"])

					// Process the task
					err := app.HandleTranscodeTask(ctx, payloadBytes)
					if err != nil {
						app.logger.Error("transcode task failed", "msgId", msgData.ID, "err", err)
						// The janitor will sweep it up if we don't ACK
						continue
					}

					// Acknowledge the message upon success
					ackCmd := vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgData.ID).Build()
					vClient.Do(ctx, ackCmd)

					// Clean up retries hash on success
					hdelCmd := vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgData.ID).Build()
					vClient.Do(ctx, hdelCmd)
				}
			}
		}
	}
}

func runValkeyJanitor(ctx context.Context, app *application, wg *sync.WaitGroup) {
	defer wg.Done()
	vClient := app.queueMgr.ValkeyClient
	app.logger.Info("started valkey janitor")

	// check every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			app.logger.Info("janitor shutting down")
			return
		case <-ticker.C:
			// 1. Find messages pending for > 15 minutes
			// XPENDING stream:transcode transcoder_group IDLE 900000 - + 100
			cmd := vClient.B().Xpending().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Idle(900000).Start("-").End("+").Count(100).Build()
			res, err := vClient.Do(ctx, cmd).ToArray()
			if err != nil && err.Error() != "redis: nil" {
				app.logger.Error("janitor failed to check pending messages", "err", err)
				continue
			}

			for _, p := range res {
				// Each pending message is an array where the first element is the message ID string
				pArr, err := p.ToArray()
				if err != nil || len(pArr) == 0 {
					continue
				}
				
				msgID, err := pArr[0].ToString()
				if err != nil {
					continue
				}
				
				// 2. Increment and check retries
				retryCmd := vClient.B().Hincrby().Key(queue.HashTranscodeRetries).Field(msgID).Increment(1).Build()
				retries, _ := vClient.Do(ctx, retryCmd).AsInt64()
				
				if retries <= 3 {
					app.logger.Info("janitor claiming message for retry", "id", msgID, "retries", retries)
					// XCLAIM stream:transcode transcoder_group janitor 900000 msgID
					claimCmd := vClient.B().Xclaim().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Consumer("janitor").MinIdleTime("900000").Id(msgID).Build()
					claimed, _ := vClient.Do(ctx, claimCmd).AsXRange()
					
					for _, msg := range claimed {
						payloadBytes := []byte(msg.FieldValues["payload"])
						
						// Try to process immediately
						err := app.HandleTranscodeTask(ctx, payloadBytes)
						if err == nil {
							// Successfully processed
							vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgID).Build())
							vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgID).Build())
						} else {
							app.logger.Error("janitor failed to process claimed task", "id", msgID, "err", err)
						}
					}
				} else {
					// 3. Poisoned message (>= 3 retries): Move to DLQ
					app.logger.Warn("janitor moving poisoned message to DLQ", "id", msgID)
					
					// Read the payload to move it
					readIdCmd := vClient.B().Xrange().Key(queue.StreamTranscode).Start(msgID).End(msgID).Build()
					messages, _ := vClient.Do(ctx, readIdCmd).AsXRange()
					
					if len(messages) > 0 {
						payloadStr := messages[0].FieldValues["payload"]
						
						// XADD to DLQ
						dlqCmd := vClient.B().Xadd().Key(queue.StreamTranscodeDLQ).Id("*").FieldValue().FieldValue("payload", payloadStr).Build()
						vClient.Do(ctx, dlqCmd)
					}
					
					// XACK to remove from main group's PEL
					vClient.Do(ctx, vClient.B().Xack().Key(queue.StreamTranscode).Group(queue.ConsumerGroup).Id(msgID).Build())
					vClient.Do(ctx, vClient.B().Hdel().Key(queue.HashTranscodeRetries).Field(msgID).Build())
				}
			}
		}
	}
}
