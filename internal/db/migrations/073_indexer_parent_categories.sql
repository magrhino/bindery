-- +migrate Up
-- Opt in per indexer to searching the broad Books (7000) / Audio (3000) parent
-- category alongside the configured subcategories, for trackers that file
-- releases loosely under the parent instead of a specific child.
--
-- The searcher only ever sends the parent for a media type the indexer already
-- carries children under, so a books-only indexer opted in never receives 3000
-- on an audiobook search. 0 = off, which is what every existing row and every
-- newly synced Prowlarr row gets: the broad parents pull in comics, magazines
-- and music, so widening a search stays an explicit per-indexer decision.
ALTER TABLE indexers ADD COLUMN include_parent_categories INTEGER NOT NULL DEFAULT 0;

-- +migrate Down

ALTER TABLE indexers DROP COLUMN include_parent_categories;
