package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type OutboxEvent struct {
	ID        int64
	EventType string
	Payload   string
	Status    string
	CreatedAt time.Time
}

type OutboxModel struct {
	db *sql.DB
}

func (m *OutboxModel) ListPending(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	query := `SELECT id, event_type, payload, status, created_at 
		FROM outbox_events 
		WHERE status = 'PENDING' 
		ORDER BY id ASC 
		LIMIT $1 
		FOR UPDATE SKIP LOCKED;`

	rows, err := m.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending events: %w", err)
	}
	defer rows.Close()

	var events []*OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload, &e.Status, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, &e)
	}
	return events, nil
}

func (m *OutboxModel) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM outbox_events WHERE id = $1;`
	_, err := m.db.ExecContext(ctx, query, id)
	return err
}
