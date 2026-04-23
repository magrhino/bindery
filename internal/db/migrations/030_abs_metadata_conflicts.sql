-- +migrate Up

CREATE TABLE abs_metadata_conflicts (
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
    resolution_status TEXT    NOT NULL DEFAULT 'unresolved' CHECK(resolution_status IN ('unresolved', 'resolved')),
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (entity_type, local_id, field_name)
);

CREATE INDEX idx_abs_metadata_conflicts_status ON abs_metadata_conflicts(resolution_status, updated_at DESC);
CREATE INDEX idx_abs_metadata_conflicts_item ON abs_metadata_conflicts(source_id, library_id, item_id);

-- +migrate Down

DROP INDEX IF EXISTS idx_abs_metadata_conflicts_item;
DROP INDEX IF EXISTS idx_abs_metadata_conflicts_status;
DROP TABLE IF EXISTS abs_metadata_conflicts;
