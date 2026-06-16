package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/theluminousartemis/video-transcoder/internal/db"
	"github.com/theluminousartemis/video-transcoder/internal/env"
	"github.com/theluminousartemis/video-transcoder/internal/queue"
	"github.com/valkey-io/valkey-go"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	redisAddr := env.GetString("REDIS_ADDR", "localhost:6379")
	logger.Info("connecting valkey client", "addr", redisAddr)
	vkClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{redisAddr},
	})
	if err != nil {
		logger.Error("init valkey client", "err", err)
		os.Exit(1)
	}
	defer vkClient.Close()

	dbDSN := env.GetString("DB_DSN", "postgres://postgres:postgres@localhost:5432/transcoder?sslmode=disable")
	logger.Info("connecting database", "dsn", dbDSN)
	dbPool, err := db.OpenDB(dbDSN)
	if err != nil {
		logger.Error("init database", "err", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	store := db.NewStorage(dbPool)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting outbox publisher relay")
	runOutboxRelay(ctx, dbPool, store, vkClient, logger)
	logger.Info("outbox publisher relay stopped")
}

func runOutboxRelay(ctx context.Context, dbPool *sql.DB, store *db.Storage, client valkey.Client, logger *slog.Logger) {
	const outboxLockID = 42_000_001
	lockRetryInterval := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		logger.Info("attempting to acquire advisory lock")
		conn, err := dbPool.Conn(ctx)
		if err != nil {
			logger.Error("get dedicated connection", "err", err)
			time.Sleep(lockRetryInterval)
			continue
		}

		var isLeader bool
		err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", outboxLockID).Scan(&isLeader)
		if err != nil {
			logger.Error("advisory lock query failed", "err", err)
			conn.Close()
			time.Sleep(lockRetryInterval)
			continue
		}

		if !isLeader {
			logger.Info("standby mode - lock held by another process")
			conn.Close()
			time.Sleep(lockRetryInterval)
			continue
		}

		logger.Info("acquired advisory lock - active leader status")

		err = processEventsLoop(ctx, conn, store, client, logger)
		if err != nil {
			logger.Error("leader loop error", "err", err)
		}

		conn.Close()
		logger.Info("released advisory lock connection")

		time.Sleep(lockRetryInterval)
	}
}

func processEventsLoop(ctx context.Context, conn *sql.Conn, store *db.Storage, client valkey.Client, logger *slog.Logger) error {
	pollInterval := 1 * time.Second
	backoff := 200 * time.Millisecond
	const maxBackoff = 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if err := conn.PingContext(ctx); err != nil {
			return fmt.Errorf("ping locking connection: %w", err)
		}

		events, err := store.Outbox.ListPending(ctx, 10)
		if err != nil {
			return fmt.Errorf("list pending outbox events: %w", err)
		}

		if len(events) == 0 {
			time.Sleep(pollInterval)
			continue
		}

		for _, event := range events {
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			if event.EventType != "video.transcode" {
				logger.Warn("unknown outbox event type, skipping", "id", event.ID, "type", event.EventType)
				if err := store.Outbox.Delete(ctx, event.ID); err != nil {
					return fmt.Errorf("delete skipped event: %w", err)
				}
				continue
			}

			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}

				if err := conn.PingContext(ctx); err != nil {
					return fmt.Errorf("ping locking connection: %w", err)
				}

				cmd := client.B().Xadd().Key(queue.StreamTranscode).Id("*").
					FieldValue().FieldValue("payload", event.Payload).Build()
				err := client.Do(ctx, cmd).Error()
				if err == nil {
					if err := store.Outbox.Delete(ctx, event.ID); err != nil {
						return fmt.Errorf("delete outbox event: %w", err)
					}
					backoff = 200 * time.Millisecond
					break
				}

				logger.Error("valkey publish failed, backing off", "err", err, "backoff", backoff)
				time.Sleep(backoff)
				backoff = backoff * 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}
	}
}
