-- +migrate Up

-- Per-format file paths for dual-format support (issue #23).
-- A book with media_type='both' tracks an ebook and an audiobook independently.
-- The existing file_path column is kept in sync for backward compatibility.
ALTER TABLE books ADD COLUMN ebook_file_path TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN audiobook_file_path TEXT NOT NULL DEFAULT '';

-- Seed per-format columns from existing data so upgrades are seamless.
UPDATE books
SET ebook_file_path     = CASE WHEN media_type = 'ebook'     THEN file_path ELSE '' END,
    audiobook_file_path = CASE WHEN media_type = 'audiobook' THEN file_path ELSE '' END;

-- +migrate Down
-- SQLite cannot drop columns without a full table rebuild; leave them in
-- place on rollback to avoid data loss.
