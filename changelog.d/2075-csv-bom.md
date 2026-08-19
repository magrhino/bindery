### Fixed
- **CSV imports of files saved by Excel, Google Sheets or Numbers** (#2075) —
  those apps put a byte order mark at the start of a UTF-8 CSV, and it used to
  stick to the first cell. The header row was no longer recognised as a header,
  so it was imported as if it were an author name, matched some unrelated
  person on OpenLibrary, and pulled in that stranger's whole catalogue — often
  hundreds of provider requests, rate limiting and a bogus author in your
  library. The mark is now stripped before parsing, for author CSVs (both the
  two-column and the plain name-per-line form) and for Goodreads exports, where
  it made the Title column unfindable and the import fail outright.
