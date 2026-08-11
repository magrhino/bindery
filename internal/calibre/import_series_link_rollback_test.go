package calibre

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

// newSeriesRollbackFixture wires an importer with both run tracking and the
// series repo, which is what production does (cmd/bindery/main.go) and what
// #1635 exercises: series links recorded against a tracked run.
func newSeriesRollbackFixture(t *testing.T) (*Importer, *fakeReader, *db.SeriesRepo, *db.BookRepo, *db.CalibreImportRunRepo, *db.CalibreEntitySnapshotRepo, *db.CalibreProvenanceRepo) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	editionRepo := db.NewEditionRepo(database)
	aliasRepo := db.NewAuthorAliasRepo(database)
	settingsRepo := db.NewSettingsRepo(database)
	seriesRepo := db.NewSeriesRepo(database)
	runsRepo := db.NewCalibreImportRunRepo(database)
	snapshotRepo := db.NewCalibreEntitySnapshotRepo(database)
	provRepo := db.NewCalibreProvenanceRepo(database)

	fr := &fakeReader{}
	imp := NewImporter(authorRepo, aliasRepo, bookRepo, editionRepo, settingsRepo).
		WithRunTracking(runsRepo, snapshotRepo, provRepo).
		WithSeries(seriesRepo)
	imp.openReader = func(string) (readerIface, error) { return fr, nil }

	return imp, fr, seriesRepo, bookRepo, runsRepo, snapshotRepo, provRepo
}

func seriesCalibreBook(id int64, title, author, seriesName string, position float64) CalibreBook {
	cb := sampleCalibreBook(id, title, author)
	cb.Series = &CalibreSeries{Name: seriesName, Position: position}
	return cb
}

func latestRunID(t *testing.T, runs *db.CalibreImportRunRepo) int64 {
	t.Helper()
	list, err := runs.ListRecent(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list))
	}
	return list[0].ID
}

func findSnapshot(snaps []models.CalibreEntitySnapshot, entityType string) *models.CalibreEntitySnapshot {
	for i := range snaps {
		if snaps[i].EntityType == entityType {
			return &snaps[i]
		}
	}
	return nil
}

// TestImporter_SeriesLinkRecordsProvenance is the #1635 regression: migration
// 044 pinned calibre_provenance.entity_type to author/book/edition, so every
// series link the importer recorded failed the CHECK constraint and left no
// run-tracking row behind.
func TestImporter_SeriesLinkRecordsProvenance(t *testing.T) {
	imp, fr, seriesRepo, bookRepo, runsRepo, snapshotRepo, provRepo := newSeriesRollbackFixture(t)
	ctx := context.Background()
	fr.books = []CalibreBook{seriesCalibreBook(1, "Sufficiently Advanced Magic", "Andrew Rowe", "Arcane Ascension", 1)}

	stats, err := imp.Run(ctx, "/lib")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.SeriesLinked != 1 || stats.SeriesFailures != 0 {
		t.Fatalf("series stats: linked=%d failures=%d, want 1/0", stats.SeriesLinked, stats.SeriesFailures)
	}

	book, err := bookRepo.GetByCalibreID(ctx, 1)
	if err != nil || book == nil {
		t.Fatalf("book not found post-import: %v", err)
	}
	all, err := seriesRepo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 series, got %d (%v)", len(all), err)
	}
	externalID := calibreSeriesLinkExternalID(book.ID, all[0].ID)

	prov, err := provRepo.GetByExternal(ctx, defaultSourceID, entityTypeSeriesLink, externalID)
	if err != nil {
		t.Fatalf("GetByExternal: %v", err)
	}
	if prov == nil {
		t.Fatal("no calibre_provenance row for the series link (CHECK constraint rejected it?)")
	}
	if prov.EntityType != entityTypeSeriesLink {
		t.Errorf("provenance entity_type = %q, want %q", prov.EntityType, entityTypeSeriesLink)
	}
	if prov.LocalID != book.ID {
		t.Errorf("provenance local_id = %d, want book id %d", prov.LocalID, book.ID)
	}
	runID := latestRunID(t, runsRepo)
	if prov.ImportRunID == nil || *prov.ImportRunID != runID {
		t.Errorf("provenance import_run_id = %v, want %d", prov.ImportRunID, runID)
	}

	snaps, err := snapshotRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("ListByRun: %v", err)
	}
	snap := findSnapshot(snaps, entityTypeSeriesLink)
	if snap == nil {
		t.Fatal("no calibre_entity_snapshots row for the series link, so rollback would never see it")
	}
	if snap.Outcome != outcomeCreated {
		t.Errorf("series link snapshot outcome = %q, want %q", snap.Outcome, outcomeCreated)
	}
	if snap.ExternalID != externalID {
		t.Errorf("series link snapshot external_id = %q, want %q", snap.ExternalID, externalID)
	}
}

// TestRollback_RemovesSeriesLink covers the other half of #1635: the link must
// actually be unwound by rollback, not merely permitted by the constraint.
func TestRollback_RemovesSeriesLink(t *testing.T) {
	imp, fr, seriesRepo, bookRepo, runsRepo, _, provRepo := newSeriesRollbackFixture(t)
	ctx := context.Background()
	fr.books = []CalibreBook{seriesCalibreBook(1, "Sufficiently Advanced Magic", "Andrew Rowe", "Arcane Ascension", 1)}

	if _, err := imp.Run(ctx, "/lib"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	book, err := bookRepo.GetByCalibreID(ctx, 1)
	if err != nil || book == nil {
		t.Fatalf("book not found post-import: %v", err)
	}
	all, err := seriesRepo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 series, got %d (%v)", len(all), err)
	}
	seriesID := all[0].ID
	externalID := calibreSeriesLinkExternalID(book.ID, seriesID)
	runID := latestRunID(t, runsRepo)

	preview, err := imp.PreviewRollback(ctx, runID)
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	if !hasAction(preview.Actions, entityTypeSeriesLink, "unlink_series") {
		t.Fatalf("preview has no unlink_series action: %+v", preview.Actions)
	}

	if _, err := imp.Rollback(ctx, runID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	linked, err := seriesRepo.ListBooksInSeries(ctx, seriesID)
	if err != nil {
		t.Fatalf("ListBooksInSeries: %v", err)
	}
	if len(linked) != 0 {
		t.Errorf("series still has %d linked book(s) after rollback, want 0", len(linked))
	}
	prov, err := provRepo.GetByExternal(ctx, defaultSourceID, entityTypeSeriesLink, externalID)
	if err != nil {
		t.Fatalf("GetByExternal: %v", err)
	}
	if prov != nil {
		t.Errorf("series link provenance survived rollback: %+v", prov)
	}
	// The series row itself is deliberately retained — a shared series must
	// outlive a single-run rollback.
	remaining, err := seriesRepo.List(ctx)
	if err != nil {
		t.Fatalf("List series: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("series rows after rollback = %d, want 1 (series survive rollback)", len(remaining))
	}
}

// TestRollback_KeepsSeriesLinkItDidNotCreate guards the blast radius: a
// membership that already existed before the run is refreshed, not owned, so
// rollback must leave it in place.
func TestRollback_KeepsSeriesLinkItDidNotCreate(t *testing.T) {
	imp, fr, seriesRepo, bookRepo, runsRepo, _, _ := newSeriesRollbackFixture(t)
	ctx := context.Background()

	// First run creates the book and the link.
	fr.books = []CalibreBook{seriesCalibreBook(1, "Sufficiently Advanced Magic", "Andrew Rowe", "Arcane Ascension", 1)}
	if _, err := imp.Run(ctx, "/lib"); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	book, err := bookRepo.GetByCalibreID(ctx, 1)
	if err != nil || book == nil {
		t.Fatalf("book not found post-import: %v", err)
	}
	all, err := seriesRepo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 series, got %d (%v)", len(all), err)
	}
	seriesID := all[0].ID

	// Second run re-sees the same library: the link already exists, so this
	// run must not claim it.
	if _, err := imp.Run(ctx, "/lib"); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	secondRunID := latestRunID(t, runsRepo)

	if _, err := imp.Rollback(ctx, secondRunID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	linked, err := seriesRepo.ListBooksInSeries(ctx, seriesID)
	if err != nil {
		t.Fatalf("ListBooksInSeries: %v", err)
	}
	if len(linked) != 1 || linked[0].ID != book.ID {
		t.Errorf("pre-existing series link was dropped by rollback: %+v", linked)
	}
}

func hasAction(actions []RollbackAction, entityType, want string) bool {
	for _, a := range actions {
		if a.EntityType == entityType && a.Action == want {
			return true
		}
	}
	return false
}
