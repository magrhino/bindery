ask: clean up ABS import rollback snapshots and author linking. Do code changes + tests.

Context:
- ABS importer lives in internal/abs/importer.go.
- DB tracking lives in internal/db/abs_imports.go, internal/models/abs_import.go, migration internal/db/migrations/031_abs_import_run_tracking.sql.
- API routes in cmd/bindery/main.go:
  POST /api/v1/abs/import/runs/{runID}/rollback/preview
  POST /api/v1/abs/import/runs/{runID}/rollback
- Current rollback code is Importer.rollback around internal/abs/importer.go:2309.
- Current author resolution is resolveAuthor/findAuthorByName/lookupUpstreamAuthor around internal/abs/importer.go:994-1448.
- Current book snapshot helpers are around internal/abs/importer.go:1700-1746.
- Existing rollback tests are around internal/abs/importer_test.go:1470+.
- Existing text normalization helper: internal/textutil/author.go NormalizeAuthorName.
- Existing Jaro-Winkler implementation already exists in internal/importer/scanner.go around jaroWinkler; prefer moving/reusing in textutil rather than duplicating. If adding dep, go-edlib is acceptable, but simple local helper is probably enough.

Problem:
ABS imports often create ABS-provider authors with abs: foreign IDs because imported ABS author names do not exactly match upstream OpenLibrary/metadata author names. This later makes rollback/linking messy. Authors that have never been linked well by the importer need better matching to upstream/local authors.

Goals:
1. Make ABS rollback reliable and idempotent.
2. Make rollback preview accurately show what rollback will do.
3. Snapshot enough pre-import state to restore linked/updated entities safely.
4. Improve ABS author matching so importer prefers existing/upstream canonical authors instead of creating abs: authors when names are close.

Author matching desired behavior:
- Normalize first, compare second.
- Normalization: lowercase, strip punctuation, collapse whitespace, remove suffixes like Jr/Sr/II/III/IV, handle initials, compare both "first last" and "last first" forms where useful.
- Best metric: Jaro-Winkler for names. Use conservative threshold, e.g. >=0.94 auto-match, 0.88-0.94 ambiguous/review unless there is supporting evidence.
- Do not silently merge ambiguous authors.
- Check aliases too.
- Prefer exact foreign ID/provenance/manual resolution first, then exact normalized, then Jaro-Winkler.
- If fuzzy match links ABS name to canonical author, record ABS name as alias.
- lookupUpstreamAuthor currently only accepts normalizeAuthorName(results[idx].Name) == want; loosen this with same safe matcher.

Rollback desired behavior:
- Current rollback gates every entity on abs_provenance current owner matching runID. That can skip legitimate snapshot restores after provenance changes. Re-evaluate: created entities should delete only if still safe/current; updated/linked entities with snapshots should restore local fields when current values still equal imported "after" values, preserving user edits.
- Current snapshot is book-only. Add author snapshot if import can mutate author name/foreignID/provider/conflict fields during enrichAuthor/resolveManualAuthor.
- Ensure snapshots capture before and after for updated/linked entities, not just created.
- Rollback should:
  - delete entities created by this run only when no later/user-owned data depends on them
  - restore updated linked entities from snapshots field-by-field only when current == after
  - preserve post-import user edits
  - unlink ABS provenance for this run
  - be idempotent
  - mark run rolled_back only when no failures
- Verify ordering: editions before books, books before authors, series unlink safe.
- Fix any nil repo risks in rollback around provenance/books/authors/series/editions.

Tests to add/repair:
- Existing tests must pass: go test ./internal/abs ./internal/db ./internal/api
- Add tests for fuzzy author match:
  - "R.R. Haywood" matches existing "RR Haywood" or "R R Haywood" and records alias
  - "Last, First" vs "First Last" if supported
  - ambiguous close matches go to review, not auto-create/merge
  - upstream search result close canonical name relinks ABS placeholder where safe
- Add rollback tests:
  - rollback restores author fields changed by ABS enrichment/relink
  - rollback preserves post-import edits
  - rollback preview matches real action types/counts
  - rollback remains idempotent
  - rollback of linked existing author/book unlinks provenance without deleting local canonical data

Implementation notes:
- Keep changes scoped. Prefer helpers in internal/textutil for name variants + Jaro-Winkler scoring.
- Avoid broad refactors.
- Run gofmt.
- Run go test ./internal/abs ./internal/db ./internal/api and report failures.'