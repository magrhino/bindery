package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/telemetry"
)

func newSetupStateFixture(t *testing.T) (*SetupStateHandler, *db.IndexerRepo, *db.DownloadClientRepo, *db.SettingsRepo) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	indexers := db.NewIndexerRepo(database)
	clients := db.NewDownloadClientRepo(database)
	authors := db.NewAuthorRepo(database)
	settings := db.NewSettingsRepo(database)
	return NewSetupStateHandler(indexers, clients, authors, settings), indexers, clients, settings
}

func getSetupState(t *testing.T, h *SetupStateHandler) SetupState {
	t.Helper()
	w := httptest.NewRecorder()
	h.Get(w, httptest.NewRequest(http.MethodGet, "/api/v1/system/setup-state", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var s SetupState
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return s
}

func TestSetupState_EmptyInstall(t *testing.T) {
	h, _, _, _ := newSetupStateFixture(t)
	s := getSetupState(t, h)
	if s.HasIndexer || s.HasClient || s.HasAuthor || s.HasGrab || s.HasImport || s.Complete {
		t.Errorf("fresh install reports progress: %+v", s)
	}
}

// Only ENABLED rows count — a disabled indexer is never searched and a
// disabled client never receives grabs, so neither is a completed step.
func TestSetupState_IgnoresDisabledRows(t *testing.T) {
	h, indexers, clients, _ := newSetupStateFixture(t)
	ctx := context.Background()

	if err := indexers.Create(ctx, &models.Indexer{Name: "ix", URL: "http://x/api", Type: "newznab", Enabled: false}); err != nil {
		t.Fatalf("create indexer: %v", err)
	}
	if err := clients.Create(ctx, &models.DownloadClient{Name: "sab", Type: "sabnzbd", Host: "h", Port: 8080, Enabled: false}); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if s := getSetupState(t, h); s.HasIndexer || s.HasClient {
		t.Errorf("disabled rows counted as configured: %+v", s)
	}

	if err := indexers.Create(ctx, &models.Indexer{Name: "ix2", URL: "http://y/api", Type: "newznab", Enabled: true}); err != nil {
		t.Fatalf("create enabled indexer: %v", err)
	}
	if s := getSetupState(t, h); !s.HasIndexer {
		t.Error("enabled indexer not detected")
	}
}

// Indexer/client are CURRENT state: deleting the last indexer must flip the
// step back to outstanding, unlike the ever-happened funnel markers.
func TestSetupState_IndexerStepIsCurrentState(t *testing.T) {
	h, indexers, _, _ := newSetupStateFixture(t)
	ctx := context.Background()

	ix := &models.Indexer{Name: "ix", URL: "http://x/api", Type: "newznab", Enabled: true}
	if err := indexers.Create(ctx, ix); err != nil {
		t.Fatalf("create: %v", err)
	}
	if s := getSetupState(t, h); !s.HasIndexer {
		t.Fatal("expected HasIndexer after create")
	}
	if err := indexers.Delete(ctx, ix.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if s := getSetupState(t, h); s.HasIndexer {
		t.Error("HasIndexer stayed true after the last indexer was deleted")
	}
}

// Grab/import are ever-happened, read from the funnel markers, so they
// survive history pruning and book deletion.
func TestSetupState_GrabAndImportFromFunnelMarkers(t *testing.T) {
	h, _, _, settings := newSetupStateFixture(t)
	ctx := context.Background()

	telemetry.MarkFirst(ctx, settings, telemetry.SettingFirstGrabAt)
	s := getSetupState(t, h)
	if !s.HasGrab {
		t.Error("HasGrab not read from the funnel marker")
	}
	if s.HasImport {
		t.Error("HasImport true without a marker")
	}

	telemetry.MarkFirst(ctx, settings, telemetry.SettingFirstImportAt)
	if s := getSetupState(t, h); !s.HasImport {
		t.Error("HasImport not read from the funnel marker")
	}
}

// An install predating the funnel markers still has authors; the fallback
// keeps its author step from being permanently unchecked.
func TestSetupState_AuthorFallsBackToLibrary(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()
	ctx := context.Background()
	authors := db.NewAuthorRepo(database)
	h := NewSetupStateHandler(db.NewIndexerRepo(database), db.NewDownloadClientRepo(database), authors, db.NewSettingsRepo(database))

	if s := getSetupState(t, h); s.HasAuthor {
		t.Fatal("empty library reports HasAuthor")
	}
	if err := authors.Create(ctx, &models.Author{Name: "Frank Herbert"}); err != nil {
		t.Fatalf("create author: %v", err)
	}
	if s := getSetupState(t, h); !s.HasAuthor {
		t.Error("HasAuthor not derived from library contents when no marker exists")
	}
}

func TestSetupState_CompleteRequiresTheWholePipeline(t *testing.T) {
	h, indexers, clients, settings := newSetupStateFixture(t)
	ctx := context.Background()

	if err := indexers.Create(ctx, &models.Indexer{Name: "ix", URL: "http://x/api", Type: "newznab", Enabled: true}); err != nil {
		t.Fatalf("indexer: %v", err)
	}
	if err := clients.Create(ctx, &models.DownloadClient{Name: "sab", Type: "sabnzbd", Host: "h", Port: 8080, Enabled: true}); err != nil {
		t.Fatalf("client: %v", err)
	}
	if err := settings.Set(ctx, telemetry.SettingFirstAuthorAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("author marker: %v", err)
	}
	// Everything but the import: still incomplete.
	if s := getSetupState(t, h); s.Complete {
		t.Error("Complete true before any import")
	}
	if err := settings.Set(ctx, telemetry.SettingFirstImportAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("import marker: %v", err)
	}
	if s := getSetupState(t, h); !s.Complete {
		t.Error("Complete false with the full pipeline satisfied")
	}
}
