package db

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func openLogDB(t *testing.T) (*LogRepo, func()) {
	t.Helper()
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	return NewLogRepo(database), func() { database.Close() }
}

func insertEntry(t *testing.T, repo *LogRepo, ts time.Time, level, component, msg string) {
	t.Helper()
	err := repo.Insert(context.Background(), LogEntry{
		TS:        ts,
		Level:     level,
		Component: component,
		Message:   msg,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestLogRepo_InsertAndQuery(t *testing.T) {
	repo, cleanup := openLogDB(t)
	defer cleanup()

	now := time.Now().UTC()
	insertEntry(t, repo, now.Add(-2*time.Second), "INFO", "scheduler", "job started")
	insertEntry(t, repo, now.Add(-1*time.Second), "WARN", "downloader", "retry")
	insertEntry(t, repo, now, "ERROR", "scheduler", "job failed")

	entries, err := repo.Query(context.Background(), LogFilter{Limit: 100})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// newest-first
	if entries[0].Level != "ERROR" {
		t.Errorf("expected first entry ERROR, got %s", entries[0].Level)
	}
}

func TestLogRepo_QueryByLevel(t *testing.T) {
	repo, cleanup := openLogDB(t)
	defer cleanup()

	now := time.Now().UTC()
	insertEntry(t, repo, now.Add(-3*time.Second), "DEBUG", "api", "debug msg")
	insertEntry(t, repo, now.Add(-2*time.Second), "INFO", "api", "info msg")
	insertEntry(t, repo, now.Add(-1*time.Second), "WARN", "api", "warn msg")
	insertEntry(t, repo, now, "ERROR", "api", "error msg")

	entries, err := repo.Query(context.Background(), LogFilter{
		HasLevel: true,
		Level:    slog.LevelWarn,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries >= WARN, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Level != "WARN" && e.Level != "ERROR" {
			t.Errorf("unexpected level %q", e.Level)
		}
	}
}

func TestLogRepo_QueryByComponent(t *testing.T) {
	repo, cleanup := openLogDB(t)
	defer cleanup()

	now := time.Now().UTC()
	insertEntry(t, repo, now.Add(-2*time.Second), "INFO", "scheduler", "msg1")
	insertEntry(t, repo, now.Add(-1*time.Second), "INFO", "downloader", "msg2")
	insertEntry(t, repo, now, "INFO", "scheduler", "msg3")

	entries, err := repo.Query(context.Background(), LogFilter{
		Component: "scheduler",
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 scheduler entries, got %d", len(entries))
	}
}

func TestLogRepo_QueryByDateRange(t *testing.T) {
	repo, cleanup := openLogDB(t)
	defer cleanup()

	base := time.Now().UTC().Truncate(time.Second)
	insertEntry(t, repo, base.Add(-10*time.Minute), "INFO", "", "old")
	insertEntry(t, repo, base.Add(-5*time.Minute), "INFO", "", "mid")
	insertEntry(t, repo, base, "INFO", "", "new")

	entries, err := repo.Query(context.Background(), LogFilter{
		FromTS: base.Add(-6 * time.Minute),
		ToTS:   base.Add(-4 * time.Minute),
		Limit:  100,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in range, got %d", len(entries))
	}
	if entries[0].Message != "mid" {
		t.Errorf("unexpected message %q", entries[0].Message)
	}
}

func TestLogRepo_QueryFullText(t *testing.T) {
	repo, cleanup := openLogDB(t)
	defer cleanup()

	now := time.Now().UTC()
	insertEntry(t, repo, now.Add(-2*time.Second), "INFO", "", "book download started")
	insertEntry(t, repo, now.Add(-1*time.Second), "INFO", "", "metadata refresh")
	insertEntry(t, repo, now, "ERROR", "", "book download failed")

	entries, err := repo.Query(context.Background(), LogFilter{
		Q:     "download",
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries matching 'download', got %d", len(entries))
	}
}

func TestLogRepo_Trim(t *testing.T) {
	repo, cleanup := openLogDB(t)
	defer cleanup()

	now := time.Now().UTC()
	insertEntry(t, repo, now.Add(-20*24*time.Hour), "INFO", "", "old entry")
	insertEntry(t, repo, now.Add(-10*24*time.Hour), "INFO", "", "borderline entry")
	insertEntry(t, repo, now, "INFO", "", "new entry")

	cutoff := now.Add(-15 * 24 * time.Hour)
	if err := repo.Trim(context.Background(), cutoff); err != nil {
		t.Fatalf("trim: %v", err)
	}

	entries, err := repo.Query(context.Background(), LogFilter{Limit: 100})
	if err != nil {
		t.Fatalf("query after trim: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after trim, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Message == "old entry" {
			t.Errorf("old entry survived trim")
		}
	}
}

func TestLogRepo_Fields(t *testing.T) {
	repo, cleanup := openLogDB(t)
	defer cleanup()

	err := repo.Insert(context.Background(), LogEntry{
		TS:      time.Now().UTC(),
		Level:   "INFO",
		Message: "test fields",
		Fields:  map[string]string{"key": "value", "book": "Dune"},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	entries, err := repo.Query(context.Background(), LogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Fields["book"] != "Dune" {
		t.Errorf("expected Fields[book]=Dune, got %q", entries[0].Fields["book"])
	}
}
