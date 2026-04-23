-- +migrate Up

ALTER TABLE abs_import_runs ADD COLUMN dry_run INTEGER NOT NULL DEFAULT 0;
ALTER TABLE abs_import_runs ADD COLUMN source_config_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE abs_import_runs ADD COLUMN checkpoint_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE abs_import_run_entities (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        INTEGER NOT NULL REFERENCES abs_import_runs(id) ON DELETE CASCADE,
    source_id     TEXT    NOT NULL DEFAULT 'default',
    library_id    TEXT    NOT NULL,
    item_id       TEXT    NOT NULL DEFAULT '',
    entity_type   TEXT    NOT NULL CHECK(entity_type IN ('author', 'book', 'series', 'edition')),
    external_id   TEXT    NOT NULL,
    local_id      INTEGER NOT NULL DEFAULT 0,
    outcome       TEXT    NOT NULL DEFAULT '',
    metadata_json TEXT    NOT NULL DEFAULT '{}',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (run_id, entity_type, external_id, local_id)
);

CREATE INDEX idx_abs_import_run_entities_run ON abs_import_run_entities(run_id, entity_type, local_id);

-- +migrate Down

DROP INDEX IF EXISTS idx_abs_import_run_entities_run;
DROP TABLE IF EXISTS abs_import_run_entities;
