### Fixed
- **`models.Book.ISBNs` is now `ProviderISBNs`, so it stops looking like stored
  data** ([#1893](https://github.com/vavallee/bindery/issues/1893)) — no
  user-visible behaviour changes; this is an internal rename plus documentation
  and a regression test. The field sat in a struct otherwise full of real
  columns, but there is no `isbns` column and nothing in `internal/db` ever
  wrote or read it: metadata providers fill it in during a search, the
  aggregator uses it to dedup results across providers, and it is empty on
  every book loaded from the database. Anything built on it compiles, reads
  correctly, and matches nothing at runtime — which nearly shipped as the fix
  for the ISBN search criteria in
  [#1724](https://github.com/vavallee/bindery/issues/1724) before the persisted
  ISBNs were traced to `editions.isbn_13` / `isbn_10`. The declaration now spells
  out the field's lifetime and points at `indexer.CriteriaISBN` as the way to get
  a stored ISBN, and a database round-trip test pins the "not persisted"
  property so anyone who later adds a column has to change the test on purpose.
