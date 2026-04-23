package db

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestABSImportRunRepoFinishRetainsCheckpointOnFailure(t *testing.T) {
	t.Parallel()

	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	repo := NewABSImportRunRepo(database)
	run := &models.ABSImportRun{
		SourceID:       "default",
		SourceLabel:    "Shelf",
		BaseURL:        "https://abs.example.com",
		LibraryID:      "lib-books",
		Status:         "running",
		CheckpointJSON: `{"libraryId":"lib-books","page":1}`,
	}
	if err := repo.Create(context.Background(), run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.UpdateCheckpoint(context.Background(), run.ID, map[string]any{
		"libraryId": "lib-books",
		"page":      1,
	}); err != nil {
		t.Fatalf("UpdateCheckpoint: %v", err)
	}
	if err := repo.Finish(context.Background(), run.ID, "failed", map[string]any{"error": "boom"}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	stored, err := repo.GetByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored run")
	}
	if stored.CheckpointJSON == "" || stored.CheckpointJSON == "{}" {
		t.Fatalf("checkpoint_json = %q, want persisted checkpoint", stored.CheckpointJSON)
	}
}

func TestABSImportRunRepoUpdateStatus(t *testing.T) {
	t.Parallel()

	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	repo := NewABSImportRunRepo(database)
	run := &models.ABSImportRun{
		SourceID:    "default",
		SourceLabel: "Shelf",
		BaseURL:     "https://abs.example.com",
		LibraryID:   "lib-books",
		Status:      "completed",
	}
	if err := repo.Create(context.Background(), run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.UpdateStatus(context.Background(), run.ID, "rolled_back"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	stored, err := repo.GetByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored == nil || stored.Status != "rolled_back" {
		t.Fatalf("run = %+v, want rolled_back status", stored)
	}
}
