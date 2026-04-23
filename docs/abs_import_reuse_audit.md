# ABS Import Reuse Audit

Phase 0 audit for an Audiobookshelf (ABS) -> Bindery import adapter.

## Executive Summary

The fastest reusable backbone already exists, but it is split across three different features:

- Long-running import orchestration already exists in [internal/calibre/importer.go](../internal/calibre/importer.go) via `calibre.Importer`.
- Owned/imported reconciliation already exists in [internal/importer/scanner.go](../internal/importer/scanner.go) and [internal/db/books.go](../internal/db/books.go).
- Settings/UI patterns for external sources already exist in [web/src/pages/SettingsPage.tsx](../web/src/pages/SettingsPage.tsx), [internal/api/settings_handler.go](../internal/api/settings_handler.go), and [internal/api/import_lists.go](../internal/api/import_lists.go).

The biggest reuse gap is dry-run and checkpoint persistence:

- There is no existing import dry-run runner in code.
- There is no persisted import checkpoint/run table today.
- Existing run tracking is either in-memory (`calibre.ImportProgress`, `calibre.SyncProgress`) or coarse timestamps/summaries (`calibre.last_import_at`, `import_lists.last_sync_at`, `library.lastScan`).

## Current Import-Related Entrypoints

| Surface | File path | Entrypoint(s) | What it does | Reuse value for ABS |
| --- | --- | --- | --- | --- |
| HTTP | [internal/api/calibre_import.go](../internal/api/calibre_import.go) | `CalibreImportHandler.Start`, `CalibreImportHandler.Status` | Starts a background library -> Bindery import and exposes polled status. | Best existing pattern for `POST /abs/import` + `GET /abs/import/status`. |
| HTTP | [internal/api/calibre_sync.go](../internal/api/calibre_sync.go) | `CalibreSyncHandler.Start`, `CalibreSyncHandler.Status` | Starts a second long-running job with progress/errors/stats. | Good pattern for a second ABS job shape if import and reconcile need to be split later. |
| HTTP | [internal/api/migrate.go](../internal/api/migrate.go) | `MigrateHandler.ImportCSV`, `MigrateHandler.ImportReadarr` | Synchronous one-shot upload imports with structured result payloads. | Reusable result/report shape ideas, but not a good orchestration seam for ABS. |
| HTTP | [internal/api/library.go](../internal/api/library.go) | `LibraryHandler.Scan`, `LibraryHandler.ScanStatus` | Fires library reconciliation asynchronously and serves the last persisted summary. | Reusable for post-import shared-filesystem reconciliation. |
| HTTP | [internal/api/import_lists.go](../internal/api/import_lists.go) | `ImportListHandler.List/Create/Update/Delete`, `HardcoverLists` | CRUD for typed external import sources plus provider-specific discovery. | Good UI/API pattern for source discovery, but the current schema is too narrow for a full ABS import run. |
| CLI | [cmd/bindery/migrate.go](../cmd/bindery/migrate.go) | `runMigrate` | Runs CSV/Readarr imports from the shell and prints JSON summaries. | Useful reference if ABS ever needs a CLI backfill command. |
| Startup | [cmd/bindery/main.go](../cmd/bindery/main.go) | `calibreImporter.Run(...)` branch | Kicks off a background import during boot when enabled. | Reusable pattern for optional ABS startup sync. |
| Scheduler | [internal/scheduler/scheduler.go](../internal/scheduler/scheduler.go) | `Scheduler.Start`, `WithCalibreSyncer`, `WithHardcoverSyncer` | Runs recurring library scan, Calibre import, and Hardcover sync jobs. | Reusable if ABS later grows a scheduled rescan mode. |
| Author ingest side-effect | [internal/api/authors.go](../internal/api/authors.go) | `AuthorHandler.FetchAuthorBooks` | Imports metadata from providers and short-circuits downloads when a local file already exists. | Important owned-state seam for ABS-created books. |

## Current Import Services

| Service | File path | Type / function(s) | What it owns today | ABS relevance |
| --- | --- | --- | --- | --- |
| Downloader -> library importer | [internal/importer/scanner.go](../internal/importer/scanner.go) | `type Scanner`, `CheckDownloads`, `ScanLibrary`, `FindExisting`, `failImport`, `createHistoryEvent` | Moves completed downloads into the library, reconciles files already on disk, and persists import/history outcomes. | Best reuse target for owned/imported reconciliation, but not for ABS catalog enumeration. |
| Library -> Bindery importer | [internal/calibre/importer.go](../internal/calibre/importer.go) | `type Importer`, `Start`, `Run`, `Progress`, `RunSync` | Long-running external-catalog import with progress, stats, and idempotent upserts. | Best orchestration template for ABS. |
| Bindery -> external library sync | [internal/calibre/syncer.go](../internal/calibre/syncer.go) | `type Syncer`, `Start`, `Progress` | Bulk push of already-imported books to Calibre with stats/errors. | Useful reporting/progress template, not the core ABS path. |
| Scheduled external source sync | [internal/hardcoverlistsyncer/syncer.go](../internal/hardcoverlistsyncer/syncer.go) | `type ListSyncer`, `Sync`, `RunSync` | Pulls Hardcover list items into Bindery as wanted books and stamps `last_sync_at`. | Good model for typed source config plus recurring sync, but not enough for ABS resume/dry-run. |
| Synchronous migration imports | [internal/migrate/csv.go](../internal/migrate/csv.go), [internal/migrate/readarr.go](../internal/migrate/readarr.go) | `ImportCSVAuthors`, `ImportReadarr` | One-shot imports that return structured JSON summaries. | Good summary shape references, but the wrong runtime model for ABS. |

## Existing Runners, Result Structs, And Run Tracking

### Best existing orchestration runner

[internal/calibre/importer.go](../internal/calibre/importer.go) is the strongest template:

- `type ImportStats`
- `type ImportProgress`
- `type Importer`
- `func (i *Importer) Start(ctx context.Context, libraryPath string) error`
- `func (i *Importer) Run(ctx context.Context, libraryPath string) (*ImportStats, error)`
- `func (i *Importer) Progress() ImportProgress`
- `func (i *Importer) RunSync(ctx context.Context)`

Why this is the best ABS seam:

- It already supports fire-and-forget HTTP start plus polling.
- It already has single-run locking (`ErrAlreadyRunning`).
- It already separates API thinness from importer orchestration.
- It already reports counters that the UI can render after completion.

### Other reusable result/report structs

| File path | Type(s) | Reuse value |
| --- | --- | --- |
| [internal/calibre/importer.go](../internal/calibre/importer.go) | `ImportStats`, `ImportProgress` | Best model for ABS per-run counters and polled progress. |
| [internal/calibre/syncer.go](../internal/calibre/syncer.go) | `SyncStats`, `SyncProgress`, `SyncError` | Good pattern when a job needs both counters and per-item failures. |
| [internal/migrate/csv.go](../internal/migrate/csv.go) | `Result` | Compact summary for synchronous imports. |
| [internal/migrate/readarr.go](../internal/migrate/readarr.go) | `ReadarrResult` | Shows how to bundle multi-section import reporting. |
| [internal/models/settings.go](../internal/models/settings.go) | `HistoryEvent` plus `HistoryEventImportFailed`, `HistoryEventBookImported` | Existing persistent event/reporting surface. |

### Existing run tracking objects

| Tracking style | File path | Object | Notes |
| --- | --- | --- | --- |
| In-memory progress | [internal/calibre/importer.go](../internal/calibre/importer.go) | `Importer.running`, `Importer.progress` | Process-local only; lost on restart. |
| In-memory progress | [internal/calibre/syncer.go](../internal/calibre/syncer.go) | `Syncer.running`, `Syncer.progress` | Process-local only; lost on restart. |
| Persisted timestamp | [internal/calibre/importer.go](../internal/calibre/importer.go), [internal/db/migrations/018_calibre_sync.sql](../internal/db/migrations/018_calibre_sync.sql) | `calibre.last_import_at` | Coarse success marker only; no checkpoint detail. |
| Persisted summary blob | [internal/importer/scanner.go](../internal/importer/scanner.go), [internal/api/library.go](../internal/api/library.go) | `library.lastScan` | Good for summary/status, not resume. |
| Persisted timestamp | [internal/models/import_list.go](../internal/models/import_list.go), [internal/db/import_lists.go](../internal/db/import_lists.go) | `ImportList.LastSyncAt` | Useful for scheduled sources, but not enough for resumable ABS paging. |

## Metrics, Counters, And Reporting That Can Be Extended

There is no dedicated metrics registry or existing `source_type` field in the codebase today. Current import/reporting is embedded in structured logs, progress structs, and history/settings persistence.

Reusable surfaces:

- [internal/calibre/importer.go](../internal/calibre/importer.go)
  - `ImportStats` counters: `AuthorsAdded`, `AuthorsLinked`, `BooksAdded`, `BooksUpdated`, `EditionsAdded`, `DuplicatesMerged`, `Skipped`
  - final structured log in `run()`
- [internal/calibre/syncer.go](../internal/calibre/syncer.go)
  - `SyncStats` counters: `Total`, `Processed`, `Pushed`, `AlreadyInCalibre`, `Failed`
  - per-item `SyncError`
- [internal/importer/scanner.go](../internal/importer/scanner.go)
  - `writeScanResult(...)` persists `files_found`, `reconciled`, `unmatched`, `tag_read_failed`
  - `createHistoryEvent(...)` already persists structured per-item outcomes
- [internal/migrate/csv.go](../internal/migrate/csv.go) and [internal/migrate/readarr.go](../internal/migrate/readarr.go)
  - `Result` / `ReadarrResult` are simple JSON summaries that the UI already understands

Recommendation:

- Add `source_type=abs` first to structured logs and history-event payloads.
- Model ABS final counters after `calibre.ImportStats`.
- Model ABS per-item failure reporting after `calibre.SyncError` or `migrate.Result.Failures`.

## Dry-Run Finding

Repository search for `dry-run`, `dryRun`, and related no-write import terms found upgrade docs, but no import dry-run implementation in backend code.

Implication:

- ABS cannot fully reuse an existing dry-run engine because none exists.
- The best near-term seam is to copy the `calibre.Importer` execution model and add an ABS-specific no-write branch inside that runner.
- Existing no-write/preflight patterns worth imitating are:
  - [internal/api/calibre.go](../internal/api/calibre.go) `CalibreHandler.Test`
  - [internal/api/import_lists.go](../internal/api/import_lists.go) `ImportListHandler.HardcoverLists`

## Library Scan, Reconcile, And Owned-State Update Seams

### Primary owned/imported write seam

[internal/db/books.go](../internal/db/books.go):

- `func (r *BookRepo) AddBookFile(ctx context.Context, bookID int64, format, path string) error`
- `func (r *BookRepo) SetFormatFilePath(ctx context.Context, id int64, mediaType, filePath string) error`
- `func (r *BookRepo) SetFilePath(ctx context.Context, id int64, filePath string) error`

This is the canonical code path that:

- records one or more on-disk paths in `book_files`
- keeps legacy `ebook_file_path` / `audiobook_file_path` / `file_path` API views in sync
- flips aggregate status to `imported` when all required formats are present

`AddBookFile(...)` is now the source-of-truth writer. `SetFormatFilePath(...)` is still available, but it delegates to `AddBookFile(...)` and is best treated as the single-path wrapper.

If ABS can verify shared filesystem paths, this is the cleanest place to mark imported/owned state.

### Existing reconciliation logic

[internal/importer/scanner.go](../internal/importer/scanner.go):

- `func (s *Scanner) FindExisting(ctx context.Context, title, authorName string) string`
- `func (s *Scanner) ScanLibrary(ctx context.Context)`
- `func (s *Scanner) writeScanResult(...)`

What these do:

- `FindExisting` checks whether a matching file is already on disk before downloads are queued.
- `ScanLibrary` walks the library, reads tags for audiobooks, matches against wanted books, verifies paths against the effective library root (`author.root_folder_id` -> `library.defaultRootFolderId` -> env fallback), and then calls `BookRepo.AddBookFile(...)`.
- `writeScanResult` persists a summary already surfaced by the UI.

### Current callers that already rely on those seams

- [internal/api/authors.go](../internal/api/authors.go)
  - `AuthorHandler.WithFinder(...)`
  - `AuthorHandler.FetchAuthorBooks(...)`
  - If `FindExisting(...)` returns a path, the handler calls `books.SetFilePath(...)` and skips auto-search.
- [internal/api/library.go](../internal/api/library.go)
  - `LibraryHandler.Scan(...)`
  - `LibraryHandler.ScanStatus(...)`

### Reverse transition / status reevaluation seams

- [internal/api/bulk.go](../internal/api/bulk.go) `reevaluateBookStatus(...)`
- [internal/api/books.go](../internal/api/books.go) `BookHandler.DeleteFile(...)`
- [internal/models/book.go](../internal/models/book.go) `NeedsEbook()`, `NeedsAudiobook()`, `WantsEbook()`, `WantsAudiobook()`

These are the existing places that move a book back from `imported` to `wanted` when a required format disappears or a media-type expectation changes.

## Settings And UI Entry Points For An ABS Source

### Best frontend insertion points

[web/src/pages/SettingsPage.tsx](../web/src/pages/SettingsPage.tsx):

- `type Tab = ... | 'import' | 'calibre'`
- `function ImportTab()`
- `function HardcoverListsSection()`
- `function CalibreTab()`
- `function CalibreSection(...)`
- `function GeneralTab()`

Most likely placement:

- Put ABS source configuration in `ImportTab()` beside the existing import/migration flows.
- Reuse the polling/progress interaction pattern from `CalibreSection(...)` for `Test connection`, `Import`, and `Status`.
- Reuse the `GeneralTab()` library-scan messaging plus the default-root-folder setting to explain shared-path reconciliation.

### Best frontend API client seams

[web/src/api/client.ts](../web/src/api/client.ts):

- generic settings: `listSettings`, `getSetting`, `setSetting`
- long-running import patterns: `calibreImportStart`, `calibreImportStatus`
- source CRUD patterns: `listImportLists`, `addImportList`, `updateImportList`, `deleteImportList`
- discovery/preflight pattern: `hardcoverLists(token)`

### Best backend config seams

- [internal/api/settings_handler.go](../internal/api/settings_handler.go) `SettingsHandler.List/Get/Set/Delete`
- [internal/db/settings.go](../internal/db/settings.go) `SettingsRepo`
- [internal/api/calibre.go](../internal/api/calibre.go) shows the existing pattern for:
  - centralized setting-key constants
  - config materialization helpers
  - validation plus `Test connection`

### Existing typed external-source seam

- [internal/models/import_list.go](../internal/models/import_list.go) `ImportList`
- [internal/db/import_lists.go](../internal/db/import_lists.go) `ImportListRepo`
- [internal/api/import_lists.go](../internal/api/import_lists.go) `ImportListHandler`
- [internal/hardcoverlistsyncer/syncer.go](../internal/hardcoverlistsyncer/syncer.go) `ListSyncer`

This is useful as a reference for a typed source plus last-sync timestamp, but not sufficient as-is for ABS because the current schema has no:

- selected library id
- checkpoint payload
- run progress
- detailed per-run summary
- multi-step preflight/discovery state

## Fastest Reusable Integration Path

Recommended path for ABS MVP:

1. Use the Calibre import stack as the orchestration template:
   - thin API handler
   - background `Start(...)`
   - polled `Progress()`
   - `ImportStats`-style counters
2. Use the Settings page `Import` tab as the admin home for ABS.
3. Store single-source ABS config as `abs.*` settings first:
   - `abs.base_url`
   - `abs.api_key`
   - `abs.library_id`
   - `abs.enabled`
   - `abs.label`
   This is faster than forcing ABS into `import_lists` immediately.
4. Reuse Bindery’s owned/imported seam by calling `BookRepo.SetFormatFilePath(...)` when ABS paths are visible locally.
   Prefer `BookRepo.AddBookFile(...)` when ABS exposes multiple visible files for the same format; keep `SetFormatFilePath(...)` as the single-canonical-path wrapper.
5. Reuse `Scanner.ScanLibrary()` and `Scanner.FindExisting()` as fallback/shared-filesystem reconciliation helpers.
6. Reuse `HistoryRepo` plus `ImportStats`-style JSON responses for reporting.
7. Add new ABS-specific checkpoint persistence instead of trying to stretch current coarse `last_import_at` / `lastSyncAt` fields into resume support.

Migration numbering note:

- `origin/main` currently ends at [internal/db/migrations/028_book_files.sql](../internal/db/migrations/028_book_files.sql).
- If ABS introduces DB tables, start at `029_*.sql` or above to avoid the `026` / `027` / `028` collisions that just landed across #283, #241, and #343/#350.

## What Not To Build On

- Do not build the core ABS importer on [internal/migrate](../internal/migrate): those imports are synchronous, upload-driven, and have no progress/checkpointing.
- Do not clone the downloader/import scanner for ABS: [internal/importer/scanner.go](../internal/importer/scanner.go) is for post-download filesystem ingestion, not external catalog enumeration.
- Do not rely on `ImportListRepo` alone for run state: it is a good source-config reference, but not a run-tracking system.
