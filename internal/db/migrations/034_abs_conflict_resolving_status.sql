-- +migrate Up

-- Extend the resolution_status CHECK constraint to include 'resolving' so the
-- conflict-resolve endpoint can atomically claim a row before applying the
-- entity update, preventing a TOCTOU race between concurrent resolves.
--
-- SQLite cannot ALTER a CHECK constraint directly, so we recreate the table
-- with the new constraint, copy the data, swap the name, and rebuild indexes.

PRAGMA foreign_keys = OFF;

CREATE TABLE abs_metadata_conflicts_new (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id         TEXT    NOT NULL DEFAULT 'default',
    library_id        TEXT    NOT NULL DEFAULT '',
    item_id           TEXT    NOT NULL DEFAULT '',
    entity_type       TEXT    NOT NULL CHECK(entity_type IN ('author', 'book')),
    local_id          INTEGER NOT NULL,
    field_name        TEXT    NOT NULL,
    abs_value         TEXT    NOT NULL DEFAULT '',
    upstream_value    TEXT    NOT NULL DEFAULT '',
    applied_source    TEXT    NOT NULL DEFAULT '',
    preferred_source  TEXT    NOT NULL DEFAULT '',
    resolution_status TEXT    NOT NULL DEFAULT 'unresolved'
        CHECK(resolution_status IN ('unresolved', 'resolving', 'resolved')),
    created_at        DATETIME NOT NULL,
    updated_at        DATETIME NOT NULL,
    UNIQUE(entity_type, local_id, field_name)
);

INSERT INTO abs_metadata_conflicts_new
    SELECT id, source_id, library_id, item_id, entity_type, local_id, field_name,
           abs_value, upstream_value, applied_source, preferred_source,
           resolution_status, created_at, updated_at
    FROM abs_metadata_conflicts;

DROP TABLE abs_metadata_conflicts;
ALTER TABLE abs_metadata_conflicts_new RENAME TO abs_metadata_conflicts;

CREATE INDEX idx_abs_metadata_conflicts_status
    ON abs_metadata_conflicts(resolution_status, updated_at DESC);

PRAGMA foreign_keys = ON;

-- +migrate Down

-- Revert 'resolving' → 'unresolved' for any rows that are mid-flight,
-- then recreate the table without the 'resolving' status.

UPDATE abs_metadata_conflicts SET resolution_status = 'unresolved' WHERE resolution_status = 'resolving';

PRAGMA foreign_keys = OFF;

CREATE TABLE abs_metadata_conflicts_old (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id         TEXT    NOT NULL DEFAULT 'default',
    library_id        TEXT    NOT NULL DEFAULT '',
    item_id           TEXT    NOT NULL DEFAULT '',
    entity_type       TEXT    NOT NULL CHECK(entity_type IN ('author', 'book')),
    local_id          INTEGER NOT NULL,
    field_name        TEXT    NOT NULL,
    abs_value         TEXT    NOT NULL DEFAULT '',
    upstream_value    TEXT    NOT NULL DEFAULT '',
    applied_source    TEXT    NOT NULL DEFAULT '',
    preferred_source  TEXT    NOT NULL DEFAULT '',
    resolution_status TEXT    NOT NULL DEFAULT 'unresolved'
        CHECK(resolution_status IN ('unresolved', 'resolved')),
    created_at        DATETIME NOT NULL,
    updated_at        DATETIME NOT NULL,
    UNIQUE(entity_type, local_id, field_name)
);

INSERT INTO abs_metadata_conflicts_old
    SELECT id, source_id, library_id, item_id, entity_type, local_id, field_name,
           abs_value, upstream_value, applied_source, preferred_source,
           resolution_status, created_at, updated_at
    FROM abs_metadata_conflicts;

DROP TABLE abs_metadata_conflicts;
ALTER TABLE abs_metadata_conflicts_old RENAME TO abs_metadata_conflicts;

CREATE INDEX idx_abs_metadata_conflicts_status
    ON abs_metadata_conflicts(resolution_status, updated_at DESC);

PRAGMA foreign_keys = ON;
