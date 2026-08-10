package db

import (
	"context"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

// TestHistoryRepoFirstEventAt covers the accessor the telemetry setup funnel
// uses to derive an install's first grab / first import: earliest row of a
// type, nil when the type has never happened, and unaffected by later rows.
func TestHistoryRepoFirstEventAt(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	repo := NewHistoryRepo(database)

	if at, err := repo.FirstEventAt(ctx, "grabbed"); err != nil || at != nil {
		t.Fatalf("empty table: got (%v, %v), want (nil, nil)", at, err)
	}

	// Two grabs and an unrelated type. Create() stamps time.Now(), so force
	// distinct created_at values to make "earliest" unambiguous.
	for _, ev := range []string{"grabbed", "grabbed", "imported"} {
		if err := repo.Create(ctx, &models.HistoryEvent{EventType: ev, SourceTitle: "x"}); err != nil {
			t.Fatalf("create %s: %v", ev, err)
		}
	}
	early := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	late := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(ctx,
		`UPDATE history SET created_at = ? WHERE id = (SELECT MIN(id) FROM history WHERE event_type='grabbed')`, early,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`UPDATE history SET created_at = ? WHERE id = (SELECT MAX(id) FROM history WHERE event_type='grabbed')`, late,
	); err != nil {
		t.Fatalf("postdate: %v", err)
	}

	at, err := repo.FirstEventAt(ctx, "grabbed")
	if err != nil {
		t.Fatalf("first grabbed: %v", err)
	}
	if at == nil || !at.Equal(early) {
		t.Errorf("first grabbed = %v, want %v (earliest, not latest)", at, early)
	}

	// A type that exists is found; one that doesn't stays nil even with rows
	// of other types present.
	if at, err := repo.FirstEventAt(ctx, "imported"); err != nil || at == nil {
		t.Errorf("first imported = (%v, %v), want a timestamp", at, err)
	}
	if at, err := repo.FirstEventAt(ctx, "failed"); err != nil || at != nil {
		t.Errorf("first failed = (%v, %v), want (nil, nil)", at, err)
	}
}
