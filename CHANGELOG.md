# Changelog

All notable changes to Bindery are documented here. Format loosely follows
[Keep a Changelog](https://keepachangelog.com) and versions follow
[Semantic Versioning](https://semver.org).

## [v0.4.1] — 2026-04-12

### Security & quality patch

#### Fixed
- Rebuilt against go1.25.9, clearing 17 stdlib CVEs reachable via the API, TLS, and URL-parsing paths (most notably GO-2026-4870 TLS KeyUpdate DoS, GO-2025-4012 cookie memory exhaustion, GO-2025-4009 PEM quadratic complexity, GO-2025-4007 x509 name-constraint quadratic).
- Repaired `.golangci.yml` — removed `gosimple` (absorbed into `staticcheck` in lint v2) and dropped `continue-on-error` on the lint job. The lint CI gate had been silently failing since the v2 upgrade.
- qBittorrent client no longer panics on session-expiry retry when `http.NewRequestWithContext` fails — the error is now propagated instead of calling `Do` on a nil request.
- API handlers that take `{id}` in the URL path now return HTTP 400 for non-numeric IDs instead of silently acting on ID 0. New `parseID` helper in `internal/api/helpers.go` consolidates the pattern.
- Library-scan importer no longer dereferences nil pointers when a book or author lookup fails; lookup errors are logged and the file falls through to the unmatched-import path.
- History-blocklist handler logs corrupt JSON `data` columns instead of silently returning a zero-value event to the client.
- SQL UPDATE in `downloads.UpdateStatus` no longer interpolates a column name via `fmt.Sprintf`. Three explicit statements, one per known status, with the column name as a fixed literal.
- Primary HTTP server now sets `ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout` instead of running with the defaults (which are effectively unlimited). Mitigates slow-loris and resource-exhaustion attacks on the public API surface.

#### Added
- Startup warning logged when `BINDERY_API_KEY` is unset, making it obvious that `/api/v1/*` is unauthenticated.
- Helm chart `deployment.yaml` now sets a hardened pod+container `securityContext`: `runAsNonRoot: true`, `runAsUser: 65532`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `seccompProfile.RuntimeDefault`, plus a writable `emptyDir` mounted at `/tmp`.
- CI workflow now auto-creates a GitHub Release with notes extracted from the matching CHANGELOG section on every `v*` tag push. Title is derived from the first `###` subheading. Idempotent — updates notes if a release already exists.

#### Changed
- Dockerfile base image pinned to `golang:1.25.9-alpine` (was `golang:1.25-alpine`) and runtime switched to `gcr.io/distroless/static-debian12:nonroot` with `USER nonroot`.
- `go.mod` bumped to `go 1.25.9`.
- Internal DB queries using `sql.ErrNoRows` comparison now use `errors.Is` for wrap-safety.

#### Frontend
- Fixed four missing `reset` dependencies in `useEffect` hooks on Authors / Books / History / Wanted pages (react-hooks/exhaustive-deps).
- Extracted `usePagination` hook into its own file (`web/src/components/usePagination.ts`) so `Pagination.tsx` is a pure component module (fixes react-refresh/only-export-components).

## [v0.4.0] — 2026-04-12

### Search overhaul

Inspired by the matching patterns in Readarr, Sonarr, and LazyLibrarian.
Fixes the long-standing "short titles get junk results" problem (e.g.
searching "The Sparrow" by Mary Doria Russell no longer returns unrelated
sparrow-themed books, comics, and music releases).

#### Added
- **Four-tier query fallback** in `BookSearch`: `t=book` → `surname+title`
  → `author+title` → title-only. The new surname+title tier disambiguates
  short titles without the noise of full-name queries that some indexers
  fail to match.
- **Word-boundary keyword matching** (`\b...\b`) everywhere in the filter
  and language checks. `sparrow` no longer leaks into `sparrowhawk` or
  `sparrows`.
- **Contiguous-phrase matching** for multi-word titles. A release must
  contain the title words together; scattered occurrences no longer pass.
- **Subtitle handling** for `Title: Subtitle` books. "Dune: Messiah"
  accepts releases tagged as either "Dune" or "Dune Messiah".
- **Composite ranking score**: quality × 100 + edition tag (RETAIL +50,
  UNABRIDGED +30, ABRIDGED −50) + year-match (±20/10/5) +
  log₁₀(grabs) × 10 + size tiebreaker + ISBN exact-match +200.
- **Release parser** (`internal/indexer/release.go`): extracts year,
  format, RETAIL/UNABRIDGED/ABRIDGED flags, release group, and ISBN from
  NZB titles.
- **Blocklist consulted during search** (both manual and auto-grab). The
  infrastructure existed but was never wired into the search flow.
- **Download quality populated on grab** via the new release parser, in
  both the manual grab handler and the scheduler auto-grab path.
- 23 new unit tests covering the matching and ranking pipeline.

#### Fixed
- Scheduler now resolves and passes the book's author name to `SearchBook`
  (previously always empty, which silently disabled the `t=book` tier,
  the `author+title` tier, and the filter's surname anchor for every
  automated search).
- Foreign-language tag filter now word-boundary-anchored. The tag `RUSSE`
  (French for "Russian") was substring-matching inside `RUSSELL`, causing
  books by authors named Russell, Russ, Russo, etc. to be rejected as
  Russian-language releases.

#### Changed
- `Searcher.SearchBook` signature: now takes `MatchCriteria{Title, Author,
  Year, ISBN}` instead of `(title, author)` so ranking can use year and
  ISBN signals.

#### Deliberately out of scope
- qBittorrent grab path and `Download.Protocol` handling (bigger refactor
  planned separately).
- Readarr-style user-facing Quality Profiles (overkill for a single-user
  tool; hardcoded weights serve 95% of cases).

## [v0.3.0] — 2026-04-12

### Added
- Mobile browsing support: responsive layout, hamburger nav, card views
  for History / Blocklist, agenda view for Calendar.
- Blocklist-from-history action for grabbed/failed events (one-click add).
- Preferred language filter for download search results (English default).
- Quick search filter on the Wanted page.
- Inline edit + enable/disable toggles for indexers, clients, and
  notifications in Settings.
- GitHub profile link in the footer.
- "No results" message when indexer search returns empty (previously
  silent).

### Fixed
- Scanner false matches; tightened title matching in library scan.
- Non-English books incorrectly ingested from OpenLibrary author works.
- `imported` books now display as "In Library" in Books page; removed the
  transient `downloaded` filter.
- Version badge only shown for tagged releases; short SHA for branch
  builds.

### Changed
- CI pushes `:latest` image tag on version-tag builds.
- Image SHA tags shortened to 7 chars.

## [v0.2.0] — 2026-04-12

### Added
- Full Readarr feature parity: tag system, metadata profiles, import
  lists, quality profiles with cutoffs, custom formats, delay profiles,
  notifications, backup/restore, and API key authentication.
- Authors / Books / Wanted / History / Blocklist list pagination.
- History page shows error details; grab events are recorded.
- Download error messages surfaced in queue UI.
- `downloaded` status filter + badge on Books page.
- App logo and favicon.

### Fixed
- OpenLibrary author works endpoint now used for accurate book fetching.
- Author search results show top work, book count, and ratings.
- Version / commit / build-date injected into Docker image via ldflags.

## [v0.1.0] — 2026-04-11

Initial public release.

### Added
- Author monitoring with OpenLibrary metadata.
- Per-book status workflow (wanted → downloading → downloaded → imported).
- Series tracking with dedicated page.
- Edition tracking (format, ISBN, publisher, page count).
- Library scan for pre-existing files.
- Newznab / Torznab indexer support with parallel querying.
- SABnzbd download client integration.
- qBittorrent client (scaffolded).
- Automatic import with naming template tokens (`{Author}`, `{Title}`,
  `{Year}`, `{ext}`).
- Cross-filesystem move support (atomic rename → copy+verify+delete).
- Webhook notifications for grab / import / failure events.
- Google Books and Hardcover.app as enricher metadata sources.
- Single-binary distribution with embedded React frontend.
- Distroless Docker image and Helm chart.

[v0.4.1]: https://github.com/vavallee/bindery/releases/tag/v0.4.1
[v0.4.0]: https://github.com/vavallee/bindery/releases/tag/v0.4.0
[v0.3.0]: https://github.com/vavallee/bindery/releases/tag/v0.3.0
[v0.2.0]: https://github.com/vavallee/bindery/releases/tag/v0.2.0
[v0.1.0]: https://github.com/vavallee/bindery/releases/tag/v0.1.0
