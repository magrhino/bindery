package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// TestMigrate072AllowsSeriesLinkAndKeepsRows is the #1635 regression: migration
// 044 pinned entity_type to ('author', 'book', 'edition') on both Calibre
// run-tracking tables, so the importer's 'series_link' rows were rejected by
// the CHECK constraint. Migration 072 rebuilds both tables with the widened
// constraint; the rebuild must not lose the rows already in them.
func TestMigrate072AllowsSeriesLinkAndKeepsRows(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	runs := NewCalibreImportRunRepo(database)
	run := &models.CalibreImportRun{LibraryPath: "/lib", Status: "completed"}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	// Rewind calibre_provenance and calibre_entity_snapshots to their
	// migration-044 shape so the rerun exercises the real upgrade, then seed
	// each with a row that predates the fix.
	restorePreFixCalibreTables(t, database)

	prov := NewCalibreProvenanceRepo(database)
	if err := prov.Upsert(ctx, &models.CalibreProvenance{
		SourceID: "default", EntityType: "book", ExternalID: "book:7", LocalID: 7, ImportRunID: &run.ID,
	}); err != nil {
		t.Fatalf("seed provenance: %v", err)
	}
	snaps := NewCalibreEntitySnapshotRepo(database)
	if err := snaps.Record(ctx, &models.CalibreEntitySnapshot{
		RunID: run.ID, SourceID: "default", EntityType: "book", ExternalID: "book:7", LocalID: 7, Outcome: "created",
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	// Sanity: the old constraint really does reject series_link.
	if err := prov.Upsert(ctx, &models.CalibreProvenance{
		SourceID: "default", EntityType: "series_link", ExternalID: "calibre:series-link:7:3", LocalID: 7,
	}); err == nil {
		t.Fatal("pre-fix schema accepted a series_link row; the fixture is not reproducing migration 044")
	}

	v072 := migrationVersionForTest(t, "072_calibre_provenance_series_link.sql")
	if _, err := database.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, v072); err != nil {
		t.Fatalf("clear migration 072 marker: %v", err)
	}
	if err := migrate(database); err != nil {
		t.Fatalf("rerun migration 072: %v", err)
	}

	// Existing rows survive the table rebuild.
	got, err := prov.GetByExternal(ctx, "default", "book", "book:7")
	if err != nil {
		t.Fatalf("GetByExternal after migration: %v", err)
	}
	if got == nil {
		t.Fatal("provenance row lost by the migration 072 rebuild")
	}
	if got.LocalID != 7 || got.ImportRunID == nil || *got.ImportRunID != run.ID {
		t.Errorf("provenance row mangled by rebuild: %+v", got)
	}
	list, err := snaps.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListByRun after migration: %v", err)
	}
	if len(list) != 1 || list[0].ExternalID != "book:7" || list[0].Outcome != "created" {
		t.Errorf("snapshot rows after rebuild = %+v, want the seeded book:7 row", list)
	}

	// And series_link is now accepted on both tables.
	if err := prov.Upsert(ctx, &models.CalibreProvenance{
		SourceID: "default", EntityType: "series_link", ExternalID: "calibre:series-link:7:3", LocalID: 7, ImportRunID: &run.ID,
	}); err != nil {
		t.Fatalf("series_link provenance rejected after migration 072: %v", err)
	}
	if err := snaps.Record(ctx, &models.CalibreEntitySnapshot{
		RunID: run.ID, SourceID: "default", EntityType: "series_link",
		ExternalID: "calibre:series-link:7:3", LocalID: 7, Outcome: "created",
	}); err != nil {
		t.Fatalf("series_link snapshot rejected after migration 072: %v", err)
	}

	// The rebuild must leave the indexes behind, otherwise every rollback
	// provenance lookup degrades to a table scan.
	for _, idx := range []string{"idx_calibre_provenance_local", "idx_calibre_entity_snapshots_run"} {
		var name string
		err := database.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, idx).Scan(&name)
		if err != nil {
			t.Errorf("index %s missing after migration 072: %v", idx, err)
		}
	}
}

// restorePreFixCalibreTables recreates the two Calibre run-tracking tables with
// the migration-044 CHECK constraint (author/book/edition only), so a test can
// exercise migration 072 as a real upgrade rather than a no-op rerun. Both
// tables are empty at this point, so no data is copied.
func restorePreFixCalibreTables(t *testing.T, database *sql.DB) {
	t.Helper()
	stmts := []string{
		`DROP INDEX IF EXISTS idx_calibre_provenance_local`,
		`DROP TABLE calibre_provenance`,
		`CREATE TABLE calibre_provenance (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id     TEXT     NOT NULL DEFAULT 'default',
			entity_type   TEXT     NOT NULL CHECK(entity_type IN ('author', 'book', 'edition')),
			external_id   TEXT     NOT NULL,
			local_id      INTEGER  NOT NULL,
			import_run_id INTEGER  REFERENCES calibre_import_runs(id) ON DELETE SET NULL,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (source_id, entity_type, external_id)
		)`,
		`CREATE INDEX idx_calibre_provenance_local ON calibre_provenance(entity_type, local_id)`,
		`DROP INDEX IF EXISTS idx_calibre_entity_snapshots_run`,
		`DROP TABLE calibre_entity_snapshots`,
		`CREATE TABLE calibre_entity_snapshots (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id        INTEGER  NOT NULL REFERENCES calibre_import_runs(id) ON DELETE CASCADE,
			source_id     TEXT     NOT NULL DEFAULT 'default',
			entity_type   TEXT     NOT NULL CHECK(entity_type IN ('author', 'book', 'edition')),
			external_id   TEXT     NOT NULL,
			local_id      INTEGER  NOT NULL DEFAULT 0,
			outcome       TEXT     NOT NULL DEFAULT '',
			metadata_json TEXT     NOT NULL DEFAULT '{}',
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (run_id, entity_type, external_id, local_id)
		)`,
		`CREATE INDEX idx_calibre_entity_snapshots_run ON calibre_entity_snapshots(run_id, entity_type, local_id)`,
	}
	for _, stmt := range stmts {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("restore pre-fix calibre schema: %v\nSQL: %s", err, stmt)
		}
	}
}
