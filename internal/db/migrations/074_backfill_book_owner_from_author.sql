-- +migrate Up
-- Repair the books an author sync mis-attributed under #1872.
--
-- AuthorRepo.CreateForUser wrote owner_user_id to the authors row but never set
-- it on the struct it handed back, so AuthorHandler.FetchAuthorBooks copied a
-- zero owner onto every book it created. Zero is persisted as NULL, and NULL is
-- what every per-user query in this codebase treats as "shared", so on a
-- multi-user install the books belonging to one user's authors were listed for
-- all of them. The code fix stops new rows from landing this way; it cannot
-- restore the owner on the rows already written.
--
-- A NULL-owned book hanging off an owned author is that bug's signature. It is
-- not the shape of legacy data: migration 025 backfilled authors and books
-- together, so a pre-multi-user install has them agreeing (both user 1, or both
-- NULL). Deliberately shared content is NULL-owned on BOTH rows and is left
-- alone by the author-side predicate.
--
-- Books under a NULL-owned author are untouched — there is no owner to inherit
-- and guessing one would take content away from whoever can currently see it.
UPDATE books
SET owner_user_id = (SELECT a.owner_user_id FROM authors a WHERE a.id = books.author_id)
WHERE owner_user_id IS NULL
  AND author_id IN (SELECT id FROM authors WHERE owner_user_id IS NOT NULL);
