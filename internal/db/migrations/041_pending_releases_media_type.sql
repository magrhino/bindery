-- +migrate Up
-- Add media_type to pending_releases so that a dual-format book's ebook and
-- audiobook pending entries can be scoped independently. Without this column a
-- successful audiobook grab calls DeleteByBook and discards the ebook's pending
-- candidates, forcing an unnecessary re-search (see #707).
--
-- Existing rows get DEFAULT 'ebook' because the table pre-dates dual-format
-- support and all historical rows are ebook searches.
ALTER TABLE pending_releases ADD COLUMN media_type TEXT NOT NULL DEFAULT 'ebook';
