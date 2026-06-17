package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
)

func getTestDB(t testing.TB) (*sql.DB, func()) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/transcoder?sslmode=disable"
	}

	dbPool, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	cleanup := func() {
		_, _ = dbPool.Exec("DELETE FROM videos WHERE id LIKE 'test-%' OR id LIKE 'bench-%'")
		_, _ = dbPool.Exec("DELETE FROM outbox_events WHERE payload LIKE '%\"test\":true%'")
		dbPool.Close()
	}

	// Clean before starting
	_, _ = dbPool.Exec("DELETE FROM videos WHERE id LIKE 'test-%' OR id LIKE 'bench-%'")
	_, _ = dbPool.Exec("DELETE FROM outbox_events WHERE payload LIKE '%\"test\":true%'")

	return dbPool, cleanup
}

func TestCreateWithOutbox(t *testing.T) {
	dbPool, cleanup := getTestDB(t)
	defer cleanup()

	model := &VideoModel{db: dbPool}
	ctx := context.Background()

	testID := "test-" + uuid.New().String()
	ext := ".mp4"
	payload := `{"video_id":"` + testID + `","ext":".mp4","test":true}`

	err := model.CreateWithOutbox(ctx, testID, ext, payload)
	if err != nil {
		t.Fatalf("CreateWithOutbox failed: %v", err)
	}

	// Verify video was created
	video, err := model.Get(ctx, testID)
	if err != nil {
		t.Fatalf("failed to retrieve video: %v", err)
	}
	if video.ID != testID {
		t.Errorf("expected video ID %s, got %s", testID, video.ID)
	}
	if video.Status != "PENDING" {
		t.Errorf("expected video status PENDING, got %s", video.Status)
	}

	// Verify outbox event was created
	var count int
	err = dbPool.QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox_events WHERE payload = $1 AND status = 'PENDING'", payload).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query outbox events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 outbox event, got %d", count)
	}
}

func BenchmarkCreateWithOutbox(b *testing.B) {
	dbPool, cleanup := getTestDB(b)
	defer cleanup()

	model := &VideoModel{db: dbPool}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			id := "bench-" + uuid.New().String()
			payload := `{"video_id":"` + id + `","ext":".mp4","test":true}`
			err := model.CreateWithOutbox(ctx, id, ".mp4", payload)
			if err != nil {
				b.Errorf("failed in benchmark loop: %v", err)
			}
		}
	})
}
