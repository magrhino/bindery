### Fixed

- **Library scan no longer rejects audiobooks when `BINDERY_AUDIOBOOK_DIR`
  differs from the ebook root** (#2033) — the scan's reconcile tiers matched a
  file by ASIN, fuzzy title, or series position and then checked the candidate
  against the ebook root regardless of the file's format. With a separate
  audiobook root, every correctly matched audiobook failed that containment
  check and fell through to the generic `no_title_match` reason, so the scan
  reported that it could not identify files it had in fact identified. Each
  file is now checked against the root for its own format.
