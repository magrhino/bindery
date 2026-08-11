### Fixed
- **Books created by an author sync now belong to the user who added the author**
  ([#1872](https://github.com/vavallee/bindery/issues/1872)) — `CreateForUser`
  wrote `owner_user_id` to the `authors` row but never set it on the struct it
  returned, so the catalogue sync stamped owner `0` onto every book it created.
  A `0` owner is stored as NULL, which per-user scoping reads as "shared", so on
  a multi-user install one user's whole catalogue was visible to every other
  account. The repo now reflects the persisted owner back onto the author, and
  the sync re-reads the author row before its insert loop so a stale snapshot
  can no longer carry the wrong owner into new books. Migration
  `074_backfill_book_owner_from_author.sql` repairs the rows already written: a
  NULL-owned book under an owned author inherits that author's owner. Books
  under a NULL-owned author are left alone, so deliberately shared content and
  pre-multi-user libraries are untouched. On a multi-user install this will
  remove books from other users' views — that is the fix working. The issue was
  reported as the allowed-languages filter dropping books; the language filter
  was not involved.
