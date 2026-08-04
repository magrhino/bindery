-- +migrate Up
-- Sweep author_identifiers rows whose author no longer exists (#1728).
--
-- author_identifiers.author_id declares ON DELETE CASCADE (migration 052), but
-- the cascade only fires while PRAGMA foreign_keys is on, and that pragma was
-- applied once via db.Exec rather than per connection (#1727). Any install
-- whose pool replaced its connection stopped enforcing the cascade for the rest
-- of the process, so author deletes left the identifier rows behind.
--
-- Those orphans are not merely untidy: foreign_id is NOT NULL UNIQUE, so a dead
-- row permanently blocks recreating an author with the same foreignAuthorId,
-- surfacing as a misleading 409 "author already exists" for an author that no
-- query can find. Users hit this by deleting an author and re-adding it.
--
-- Idempotent, and a no-op on installs that never lost the pragma.
DELETE FROM author_identifiers
WHERE author_id NOT IN (SELECT id FROM authors);

-- +migrate Down
-- Irreversible: the deleted rows referenced authors that no longer exist, so
-- there is nothing to restore them from.
