### Fixed
- **Author refresh no longer spends every metadata round trip in sequence**
  ([#1888](https://github.com/vavallee/bindery/issues/1888)) — a refresh that
  added 65 books for one author took close to an hour. The catalogue sync loop
  was not the problem: the cost was in the three per-work enrichment phases that
  run *before* it, each a strictly serial walk of the whole work list. A 65-work
  author paid 195 upstream round trips one after another before the first book
  row was written, so any slow or timing-out provider multiplied straight into
  wall clock. Two of those phases were also asking OpenLibrary for the *same
  URL* twice: the work-language sampler (#891) and the work-cover sampler
  (#1748) both fetch `/works/{id}/editions.json?limit=5` and each kept its own
  cache. They now share one sample, which halves OpenLibrary requests for the
  pass — measured at 130 → 65 requests for a 65-work author — and the sampling
  and cover-enrichment passes run four works at a time instead of one, matching
  the pace already used elsewhere for provider fan-out. On a 65-work author with
  a 20 ms provider the sampling pass drops from 1.31 s to 0.35 s; against a real
  provider, where a round trip is seconds rather than milliseconds, the saving
  scales with it. Per-book Hardcover edition hydration inside the sync loop is
  still serial and is tracked separately.
