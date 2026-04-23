-- +migrate Up

CREATE TABLE abs_review_queue (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id      TEXT    NOT NULL,
    library_id     TEXT    NOT NULL,
    item_id        TEXT    NOT NULL,
    title          TEXT    NOT NULL DEFAULT '',
    primary_author TEXT    NOT NULL DEFAULT '',
    asin           TEXT    NOT NULL DEFAULT '',
    media_type     TEXT    NOT NULL DEFAULT '',
    review_reason  TEXT    NOT NULL DEFAULT '',
    payload_json   TEXT    NOT NULL DEFAULT '{}',
    resolved_author_foreign_id TEXT NOT NULL DEFAULT '',
    resolved_author_name       TEXT NOT NULL DEFAULT '',
    resolved_book_foreign_id   TEXT NOT NULL DEFAULT '',
    resolved_book_title        TEXT NOT NULL DEFAULT '',
    edited_title               TEXT NOT NULL DEFAULT '',
    latest_run_id  INTEGER,
    status         TEXT    NOT NULL DEFAULT 'pending',
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    UNIQUE (source_id, library_id, item_id)
);

CREATE INDEX idx_abs_review_queue_status ON abs_review_queue(status, updated_at DESC);

-- +migrate Down

DROP INDEX IF EXISTS idx_abs_review_queue_status;
DROP TABLE IF EXISTS abs_review_queue;
