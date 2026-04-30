-- +migrate Up

ALTER TABLE download_clients ADD COLUMN post_import_category TEXT;

-- +migrate Down
-- SQLite cannot drop columns without rebuilding the table; keep column.
