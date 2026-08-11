-- +migrate Up

-- Allow 'series_link' as a Calibre run-tracking entity type (#1635).
--
-- Migration 044 pinned entity_type to ('author', 'book', 'edition') on both
-- calibre_provenance and calibre_entity_snapshots. The series persistence
-- added later (#905) records book-to-series links under entity_type
-- 'series_link', so every series link in a Calibre import failed the CHECK,
-- logged a warning, and left the link with no run-tracking row — which meant
-- rollback could never unwind it.
--
-- SQLite cannot ALTER a CHECK constraint directly, so we recreate each table
-- with the widened constraint, copy the data, swap the name, and rebuild the
-- indexes. Same shape as 034_abs_conflict_resolving_status.sql.

PRAGMA foreign_keys = OFF;

CREATE TABLE calibre_provenance_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id     TEXT     NOT NULL DEFAULT 'default',
    entity_type   TEXT     NOT NULL CHECK(entity_type IN ('author', 'book', 'edition', 'series_link')),
    external_id   TEXT     NOT NULL,
    local_id      INTEGER  NOT NULL,
    import_run_id INTEGER  REFERENCES calibre_import_runs(id) ON DELETE SET NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_id, entity_type, external_id)
);

INSERT INTO calibre_provenance_new (id, source_id, entity_type, external_id, local_id, import_run_id, created_at, updated_at)
    SELECT id, source_id, entity_type, external_id, local_id, import_run_id, created_at, updated_at
    FROM calibre_provenance;

DROP TABLE calibre_provenance;
ALTER TABLE calibre_provenance_new RENAME TO calibre_provenance;

CREATE INDEX idx_calibre_provenance_local ON calibre_provenance(entity_type, local_id);

CREATE TABLE calibre_entity_snapshots_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        INTEGER  NOT NULL REFERENCES calibre_import_runs(id) ON DELETE CASCADE,
    source_id     TEXT     NOT NULL DEFAULT 'default',
    entity_type   TEXT     NOT NULL CHECK(entity_type IN ('author', 'book', 'edition', 'series_link')),
    external_id   TEXT     NOT NULL,
    local_id      INTEGER  NOT NULL DEFAULT 0,
    outcome       TEXT     NOT NULL DEFAULT '',
    metadata_json TEXT     NOT NULL DEFAULT '{}',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (run_id, entity_type, external_id, local_id)
);

INSERT INTO calibre_entity_snapshots_new (id, run_id, source_id, entity_type, external_id, local_id, outcome, metadata_json, created_at)
    SELECT id, run_id, source_id, entity_type, external_id, local_id, outcome, metadata_json, created_at
    FROM calibre_entity_snapshots;

DROP TABLE calibre_entity_snapshots;
ALTER TABLE calibre_entity_snapshots_new RENAME TO calibre_entity_snapshots;

CREATE INDEX idx_calibre_entity_snapshots_run ON calibre_entity_snapshots(run_id, entity_type, local_id);

PRAGMA foreign_keys = ON;

-- +migrate Down

-- Drop the series-link rows first: they cannot satisfy the narrowed
-- constraint, and keeping them would abort the rebuild.

DELETE FROM calibre_entity_snapshots WHERE entity_type = 'series_link';
DELETE FROM calibre_provenance WHERE entity_type = 'series_link';

PRAGMA foreign_keys = OFF;

CREATE TABLE calibre_provenance_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id     TEXT     NOT NULL DEFAULT 'default',
    entity_type   TEXT     NOT NULL CHECK(entity_type IN ('author', 'book', 'edition')),
    external_id   TEXT     NOT NULL,
    local_id      INTEGER  NOT NULL,
    import_run_id INTEGER  REFERENCES calibre_import_runs(id) ON DELETE SET NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_id, entity_type, external_id)
);

INSERT INTO calibre_provenance_old (id, source_id, entity_type, external_id, local_id, import_run_id, created_at, updated_at)
    SELECT id, source_id, entity_type, external_id, local_id, import_run_id, created_at, updated_at
    FROM calibre_provenance;

DROP TABLE calibre_provenance;
ALTER TABLE calibre_provenance_old RENAME TO calibre_provenance;

CREATE INDEX idx_calibre_provenance_local ON calibre_provenance(entity_type, local_id);

CREATE TABLE calibre_entity_snapshots_old (
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
);

INSERT INTO calibre_entity_snapshots_old (id, run_id, source_id, entity_type, external_id, local_id, outcome, metadata_json, created_at)
    SELECT id, run_id, source_id, entity_type, external_id, local_id, outcome, metadata_json, created_at
    FROM calibre_entity_snapshots;

DROP TABLE calibre_entity_snapshots;
ALTER TABLE calibre_entity_snapshots_old RENAME TO calibre_entity_snapshots;

CREATE INDEX idx_calibre_entity_snapshots_run ON calibre_entity_snapshots(run_id, entity_type, local_id);

PRAGMA foreign_keys = ON;
