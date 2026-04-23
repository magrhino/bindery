-- +migrate Up

ALTER TABLE abs_review_queue ADD COLUMN resolved_author_foreign_id TEXT NOT NULL DEFAULT '';
ALTER TABLE abs_review_queue ADD COLUMN resolved_author_name TEXT NOT NULL DEFAULT '';
ALTER TABLE abs_review_queue ADD COLUMN resolved_book_foreign_id TEXT NOT NULL DEFAULT '';
ALTER TABLE abs_review_queue ADD COLUMN resolved_book_title TEXT NOT NULL DEFAULT '';
ALTER TABLE abs_review_queue ADD COLUMN edited_title TEXT NOT NULL DEFAULT '';

-- +migrate Down

CREATE TABLE abs_review_queue_old (
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
    latest_run_id  INTEGER,
    status         TEXT    NOT NULL DEFAULT 'pending',
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    UNIQUE (source_id, library_id, item_id)
);

INSERT INTO abs_review_queue_old (
    id, source_id, library_id, item_id, title, primary_author, asin, media_type,
    review_reason, payload_json, latest_run_id, status, created_at, updated_at
)
SELECT
    id, source_id, library_id, item_id, title, primary_author, asin, media_type,
    review_reason, payload_json, latest_run_id, status, created_at, updated_at
FROM abs_review_queue;

DROP TABLE abs_review_queue;
ALTER TABLE abs_review_queue_old RENAME TO abs_review_queue;
CREATE INDEX idx_abs_review_queue_status ON abs_review_queue(status, updated_at DESC);
