-- +migrate Up

CREATE TABLE IF NOT EXISTS series_hardcover_links (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    series_id              INTEGER NOT NULL UNIQUE REFERENCES series(id) ON DELETE CASCADE,
    hardcover_series_id    TEXT    NOT NULL,
    hardcover_provider_id  TEXT    NOT NULL DEFAULT '',
    hardcover_title        TEXT    NOT NULL DEFAULT '',
    hardcover_author_name  TEXT    NOT NULL DEFAULT '',
    hardcover_book_count   INTEGER NOT NULL DEFAULT 0,
    link_confidence        REAL    NOT NULL DEFAULT 0,
    linked_by              TEXT    NOT NULL DEFAULT 'auto',
    linked_at              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_series_hardcover_links_series_id ON series_hardcover_links(series_id);
CREATE INDEX IF NOT EXISTS idx_series_hardcover_links_hardcover_series_id ON series_hardcover_links(hardcover_series_id);

INSERT OR IGNORE INTO series_hardcover_links (
    series_id,
    hardcover_series_id,
    hardcover_provider_id,
    hardcover_title,
    link_confidence,
    linked_by,
    linked_at,
    created_at,
    updated_at
)
SELECT
    id,
    foreign_id,
    REPLACE(foreign_id, 'hc-series:', ''),
    title,
    0.8,
    'auto',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM series
WHERE foreign_id LIKE 'hc-series:%';

-- +migrate Down

DROP TABLE IF EXISTS series_hardcover_links;
