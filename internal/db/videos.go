package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Video struct {
	ID        string
	Status    string
	Ext       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type VideoModel struct {
	db *sql.DB
}

func (m *VideoModel) CreateWithOutbox(ctx context.Context, id, ext, payload string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	videoQuery := `INSERT INTO videos (id, status, ext, created_at, updated_at) 
		VALUES ($1, 'PENDING', $2, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET status = 'PENDING', updated_at = NOW();`
	if _, err := tx.ExecContext(ctx, videoQuery, id, ext); err != nil {
		return fmt.Errorf("insert video: %w", err)
	}

	outboxQuery := `INSERT INTO outbox_events (event_type, payload, status, created_at)
		VALUES ('video.transcode', $1, 'PENDING', NOW());`
	if _, err := tx.ExecContext(ctx, outboxQuery, payload); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (m *VideoModel) Get(ctx context.Context, id string) (*Video, error) {
	query := `SELECT id, status, ext, created_at, updated_at FROM videos WHERE id = $1;`
	row := m.db.QueryRowContext(ctx, query, id)

	var v Video
	err := row.Scan(&v.ID, &v.Status, &v.Ext, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (m *VideoModel) UpdateStatus(ctx context.Context, id, status string) error {
	query := `UPDATE videos SET status = $1, updated_at = NOW() WHERE id = $2;`
	_, err := m.db.ExecContext(ctx, query, status, id)
	return err
}
