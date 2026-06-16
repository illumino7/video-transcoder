package db

import (
	"context"
	"database/sql"
)

type Storage struct {
	Videos interface {
		Get(ctx context.Context, id string) (*Video, error)
		UpdateStatus(ctx context.Context, id, status string) error
		CreateWithOutbox(ctx context.Context, id, ext, payload string) error
	}
	Outbox interface {
		ListPending(ctx context.Context, limit int) ([]*OutboxEvent, error)
		Delete(ctx context.Context, id int64) error
	}
}

// NewStorage initializes a new database storage manager.
func NewStorage(db *sql.DB) *Storage {
	return &Storage{
		Videos: &VideoModel{db: db},
		Outbox: &OutboxModel{db: db},
	}
}
