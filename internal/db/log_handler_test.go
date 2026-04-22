package db

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/logbuf"
)

func TestLogSlogHandler_MirrorsToDBAndRing(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer database.Close()

	repo := NewLogRepo(database)
	ring := logbuf.New(100)
	ring.SetLevel(slog.LevelDebug)

	dbHandler := NewLogSlogHandler(repo, slog.LevelDebug)
	logger := slog.New(logbuf.NewTee(ring, dbHandler))

	logger.Info("test message", "component", "api", "key", "value")

	// Give the async drain goroutine time to flush.
	deadline := time.Now().Add(2 * time.Second)
	var entries []LogEntry
	for time.Now().Before(deadline) {
		entries, err = repo.Query(context.Background(), LogFilter{Limit: 10})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(entries) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(entries) == 0 {
		t.Fatal("expected at least 1 DB entry, got 0")
	}
	if entries[0].Message != "test message" {
		t.Errorf("unexpected message %q", entries[0].Message)
	}
	if entries[0].Component != "api" {
		t.Errorf("unexpected component %q", entries[0].Component)
	}
	if entries[0].Fields["key"] != "value" {
		t.Errorf("unexpected Fields[key] %q", entries[0].Fields["key"])
	}

	// Ring buffer should also have the entry.
	snap := ring.Snapshot(slog.LevelDebug, 10)
	if len(snap) == 0 {
		t.Error("expected ring buffer to also have the entry")
	}
}

func TestLogSlogHandler_DropsOnFullChannel(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer database.Close()

	repo := NewLogRepo(database)
	h := NewLogSlogHandler(repo, slog.LevelInfo)

	// Flood the handler — should never block even when channel is full.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 2000 {
			rec := slog.NewRecord(time.Now(), slog.LevelInfo, "flood", 0)
			_ = i
			_ = h.Handle(context.Background(), rec)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler blocked — should be non-blocking")
	}
}
