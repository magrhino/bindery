# ABS Import Guide

This document is the single source of truth for the Audiobookshelf (ABS) import feature in Bindery.

It replaces the earlier phase-by-phase planning notes with one implementation-focused guide for contributors.

## Scope

The ABS importer lets an admin:

- configure one ABS source
- test API-key access and discover visible libraries
- select a single target library
- import ABS metadata into Bindery
- review import runs, dry-runs, rollback previews, and review/conflict items

The implementation is metadata-first and shared-filesystem-aware:

- Bindery imports catalog metadata and provenance first.
- If ABS-reported paths are visible under Bindery-managed storage, Bindery records them through the normal `book_files` path and status logic.
- If paths are missing or out of scope, the import still succeeds as metadata-only instead of failing the whole batch.

## What To Expect

The importer is designed to pull over the ABS catalog metadata it can read for each visible item rather than only creating empty placeholders. In practice that includes author, book, series, edition, provenance, and review/conflict state plus the main metadata fields Bindery tracks such as description, release date, genres, language, narrator, duration, ASIN, and media type.

Import quality depends heavily on ABS metadata quality. The more books you already have in Audiobookshelf with strong metadata, especially stable identifiers like ASINs, the better Bindery can match existing authors and books, avoid ambiguous title-only fallbacks, and keep the review queue small.

## Main Code Paths

Backend:

- [internal/abs/client.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/client.go): ABS HTTP client, auth, library and item fetches
- [internal/abs/enumerator.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/enumerator.go): paged enumeration and checkpoint-aware traversal
- [internal/abs/importer.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/importer.go): import orchestration, dry-run, rollback planning, mapping, progress
- [internal/abs/types.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/types.go): ABS response and normalized item types
- [internal/api/abs.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs.go): config, connection test, library discovery
- [internal/api/abs_import.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs_import.go): start, status, recent runs, rollback endpoints
- [internal/api/abs_review.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs_review.go): review queue actions
- [internal/api/abs_conflicts.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs_conflicts.go): conflict listing and source resolution

Persistence:

- [internal/db/abs_imports.go](/Users/ryanjones/Documents/bindery/bindery/internal/db/abs_imports.go): run, provenance, run-entity, and review repositories
- [internal/db/abs_metadata_conflicts.go](/Users/ryanjones/Documents/bindery/bindery/internal/db/abs_metadata_conflicts.go): conflict persistence
- [internal/db/migrations/029_abs_imports.sql](/Users/ryanjones/Documents/bindery/bindery/internal/db/migrations/029_abs_imports.sql)
- [internal/db/migrations/030_abs_metadata_conflicts.sql](/Users/ryanjones/Documents/bindery/bindery/internal/db/migrations/030_abs_metadata_conflicts.sql)
- [internal/db/migrations/031_abs_import_run_tracking.sql](/Users/ryanjones/Documents/bindery/bindery/internal/db/migrations/031_abs_import_run_tracking.sql)
- [internal/db/migrations/032_abs_review_queue.sql](/Users/ryanjones/Documents/bindery/bindery/internal/db/migrations/032_abs_review_queue.sql)
- [internal/db/migrations/033_abs_review_resolution.sql](/Users/ryanjones/Documents/bindery/bindery/internal/db/migrations/033_abs_review_resolution.sql)

Frontend:

- [web/src/api/client.ts](/Users/ryanjones/Documents/bindery/bindery/web/src/api/client.ts): ABS API client types and methods
- [web/src/pages/SettingsPage.tsx](/Users/ryanjones/Documents/bindery/bindery/web/src/pages/SettingsPage.tsx): ABS settings/import UI
- [web/src/components/ABSAuthorConflictsPanel.tsx](/Users/ryanjones/Documents/bindery/bindery/web/src/components/ABSAuthorConflictsPanel.tsx): author conflict review panel

Bootstrap:

- [cmd/bindery/main.go](/Users/ryanjones/Documents/bindery/bindery/cmd/bindery/main.go): repo wiring, importer construction, and route registration

## Runtime Flow

1. An admin saves ABS settings under the `abs.*` keys.
2. The UI can probe the ABS instance to validate auth and list accessible libraries.
3. `POST /api/v1/abs/import` starts an async import using the stored config plus any request overrides.
4. The importer enumerates the selected ABS library, fetching detail payloads when list data is incomplete.
5. Each normalized ABS item is mapped into Bindery authors, books, series, editions, provenance, and optional review/conflict records.
6. Progress is exposed through `GET /api/v1/abs/import/status`.
7. Completed runs are persisted and surfaced through recent-runs and rollback endpoints.

## Mapping Rules

### Authors

Resolution order:

1. `abs_provenance` lookup for the ABS author id
2. exact case-insensitive match in `authors.name`
3. exact case-insensitive match in `author_aliases.name`
4. create a new shared author

Imported/shared defaults:

- ABS-created rows are shared/global by default
- `owner_user_id` remains `NULL`
- `metadata_provider` is set to `audiobookshelf` for newly created rows

Secondary ABS authors are recorded as aliases where possible.

### Books

Resolution order:

1. `abs_provenance` lookup for the ABS item id
2. fallback `books.foreign_id = abs:book:<library_id>:<item_id>`
3. existing book under the resolved author with the same normalized title
4. create a new shared book

Important applied fields:

- title and sort title
- description when non-empty
- release date
- genres
- language
- narrator
- duration seconds
- ASIN
- media type

The importer fails safe on ambiguous title matches rather than guessing.

### Series

Resolution order:

1. `abs_provenance` lookup for the ABS series id
2. existing normalized-title series match
3. create a new series

Sequence metadata is written through `series_books` when ABS provides it.

### Editions

Each ABS item may upsert:

- one `ebook` edition
- one `audiobook` edition

Edition ids are deterministic and derived from library, item, and format so reruns stay idempotent.

## File Reconciliation

The importer does not create an ABS-only ownership model. It reuses Bindery's existing file and status logic.

Accepted paths:

- ebook ownership comes from `NormalizedLibraryItem.EbookPath`
- audiobook ownership comes from the ABS item `Path`
- a path is only accepted when it resolves under a Bindery-visible library root

Effective roots can come from:

- `BINDERY_LIBRARY_DIR`
- `BINDERY_AUDIOBOOK_DIR`
- the author's explicit root folder
- `library.defaultRootFolderId`

When a path is visible and valid, Bindery records it through the normal book-file write path. When it is not, the item remains metadata-only and contributes to pending/manual follow-up rather than failing the whole run.

## Storage Model

Config is stored in existing settings rows for the single-source MVP:

- `abs.base_url`
- `abs.api_key`
- `abs.library_id`
- `abs.enabled`
- `abs.label`
- `abs.path_remap`

Run and provenance persistence adds these tables:

- `abs_import_runs`: batch envelope, status, config snapshot, checkpoint, summary
- `abs_import_run_entities`: per-entity outcomes for rollback and inspection
- `abs_provenance`: ABS-to-Bindery entity linkage
- `abs_review_items`: deferred/manual review work
- `abs_metadata_conflicts`: source-choice conflicts between ABS and upstream metadata

This gives the importer idempotent reruns, traceability, and rollback planning without overloading the generic settings store with run state.

## API Surface

Config and discovery:

- `GET /api/v1/abs/config`
- `PUT /api/v1/abs/config`
- `POST /api/v1/abs/test`
- `POST /api/v1/abs/libraries`

Import and rollback:

- `POST /api/v1/abs/import`
- `GET /api/v1/abs/import/status`
- `GET /api/v1/abs/import/runs`
- `POST /api/v1/abs/import/runs/{runID}/rollback/preview`
- `POST /api/v1/abs/import/runs/{runID}/rollback`

Review and conflict handling:

- `GET /api/v1/abs/review`
- `POST /api/v1/abs/review/{id}/approve`
- `POST /api/v1/abs/review/{id}/resolve-author`
- `POST /api/v1/abs/review/{id}/resolve-book`
- `POST /api/v1/abs/review/{id}/dismiss`
- `GET /api/v1/abs/conflicts`
- `POST /api/v1/abs/conflicts/{id}/resolve`

## Testing

Unit and handler coverage lives in:

- [internal/abs/client_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/client_test.go)
- [internal/abs/contract_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/contract_test.go)
- [internal/abs/enumerator_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/enumerator_test.go)
- [internal/abs/importer_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/importer_test.go)
- [internal/api/abs_import_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs_import_test.go)
- [internal/api/abs_review_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs_review_test.go)
- [internal/api/abs_conflicts_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs_conflicts_test.go)

Pinned contract coverage lives in [tests/abscontract](/Users/ryanjones/Documents/bindery/bindery/tests/abscontract) and is exposed through `make abs-contract`.

Pinned baseline:

- `2.33.2`

Seeded scenarios:

- `single-file-audiobook`
- `folder-multi-file-audiobook`
- `ebook-only-item`
- `mixed-metadata-completeness`
- `series-linked-item`
- `permission-limited-account`

Covered behaviors:

- auth success and failure
- permission-scoped library listing
- paging and detail-fetch fallback
- dry-run behavior
- idempotent reruns
- checkpoint-aware resume seams

## Contributor Notes

When changing the ABS importer:

- prefer extending the existing ABS importer and enumerator seams instead of introducing a second runner shape
- keep ABS-created rows shared unless product requirements explicitly change
- preserve the metadata-first, filesystem-aware behavior
- update `tests/abscontract` when external contract assumptions change
- run `go test ./cmd/... ./internal/... ./tests/abscontract/...` and `make abs-contract` when touching importer behavior

If this feature grows beyond one configured source or needs richer persisted source state, revisit whether `abs.*` settings should become a dedicated source table. Until then, keep this document focused on the implemented path rather than speculative phase planning.
