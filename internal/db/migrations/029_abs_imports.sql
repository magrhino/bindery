-- +migrate Up

CREATE TABLE abs_import_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id    TEXT    NOT NULL DEFAULT 'default',
    source_label TEXT    NOT NULL DEFAULT '',
    base_url     TEXT    NOT NULL DEFAULT '',
    library_id   TEXT    NOT NULL,
    status       TEXT    NOT NULL DEFAULT 'running',
    summary_json TEXT    NOT NULL DEFAULT '{}',
    started_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at  DATETIME
);

CREATE INDEX idx_abs_import_runs_library ON abs_import_runs(library_id, started_at DESC);

CREATE TABLE abs_provenance (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id     TEXT    NOT NULL DEFAULT 'default',
    library_id    TEXT    NOT NULL,
    entity_type   TEXT    NOT NULL CHECK(entity_type IN ('author', 'book', 'series', 'edition')),
    external_id   TEXT    NOT NULL,
    local_id      INTEGER NOT NULL,
    item_id       TEXT    NOT NULL DEFAULT '',
    format        TEXT    NOT NULL DEFAULT '',
    file_ids_json TEXT    NOT NULL DEFAULT '[]',
    import_run_id INTEGER REFERENCES abs_import_runs(id) ON DELETE SET NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_id, library_id, entity_type, external_id)
);

CREATE INDEX idx_abs_provenance_local ON abs_provenance(entity_type, local_id);
CREATE INDEX idx_abs_provenance_item ON abs_provenance(library_id, item_id);

-- +migrate Down

DROP INDEX IF EXISTS idx_abs_provenance_item;
DROP INDEX IF EXISTS idx_abs_provenance_local;
DROP TABLE IF EXISTS abs_provenance;
DROP INDEX IF EXISTS idx_abs_import_runs_library;
DROP TABLE IF EXISTS abs_import_runs;
