# ABS Import Architecture Map

Recommended architecture for Phase 1+ based on the reusable seams identified in Phase 0.

## Recommended Backbone

Use three existing Bindery patterns together instead of inventing a parallel subsystem:

- Configuration and admin UX:
  - [web/src/pages/SettingsPage.tsx](../web/src/pages/SettingsPage.tsx) `ImportTab()`
  - [internal/api/settings_handler.go](../internal/api/settings_handler.go) `SettingsHandler`
  - [internal/db/settings.go](../internal/db/settings.go) `SettingsRepo`
- Long-running import orchestration:
  - [internal/api/calibre_import.go](../internal/api/calibre_import.go) `CalibreImportHandler`
  - [internal/calibre/importer.go](../internal/calibre/importer.go) `Importer`, `ImportProgress`, `ImportStats`
- Owned/imported reconciliation:
  - [internal/importer/scanner.go](../internal/importer/scanner.go) `FindExisting`, `ScanLibrary`
  - [internal/db/books.go](../internal/db/books.go) `AddBookFile`, `SetFormatFilePath`, `SetFilePath`

## Proposed ABS Component Map

| Concern | Reuse from existing code | ABS addition |
| --- | --- | --- |
| Source config storage | [internal/api/settings_handler.go](../internal/api/settings_handler.go), [internal/db/settings.go](../internal/db/settings.go) | Add `abs.*` keys first for the single-source MVP. |
| Settings UI | [web/src/pages/SettingsPage.tsx](../web/src/pages/SettingsPage.tsx) `ImportTab()` and `CalibreSection(...)` | Add an `AudiobookshelfSection()` under the Import tab. |
| Connection test | [internal/api/calibre.go](../internal/api/calibre.go) `CalibreHandler.Test` | Add `ABSHandler.Test` for auth + reachability. |
| Library discovery | [internal/api/import_lists.go](../internal/api/import_lists.go) `HardcoverLists` | Add `ABSHandler.Libraries` to enumerate accessible ABS libraries. |
| Long-running import start/status | [internal/api/calibre_import.go](../internal/api/calibre_import.go) | Add `ABSImportHandler.Start` and `ABSImportHandler.Status`. |
| Import runner | [internal/calibre/importer.go](../internal/calibre/importer.go) | Add `abs.Importer` with `Start`, `Run`, `RunSync`, `Progress`. |
| Result/progress payloads | [internal/calibre/importer.go](../internal/calibre/importer.go) `ImportStats`, `ImportProgress` | Reuse shape where possible; add ABS-specific counters only where needed. |
| Mapping/upsert pattern | [internal/calibre/importer.go](../internal/calibre/importer.go) `resolveAuthor`, `upsertBook`, `upsertEdition` | Add ABS-specific provenance lookup and item normalization, but keep the same control flow. |
| Already-owned marking | [internal/db/books.go](../internal/db/books.go), [internal/importer/scanner.go](../internal/importer/scanner.go) | Verify shared path plus effective library root, then call `AddBookFile(...)` for each visible file, use `SetFormatFilePath(...)` only for single canonical-path cases, or defer to `ScanLibrary()`. |
| Persisted reporting | [internal/db/history.go](../internal/db/history.go), [internal/importer/scanner.go](../internal/importer/scanner.go) | Add ABS run summary + per-item failures/history payloads. |
| Checkpoint/resume | no existing true equivalent | Add new ABS checkpoint persistence; current Bindery code has only coarse timestamps/summaries. |

## Recommended Runtime Flow

1. Admin opens the Settings `Import` tab.

2. The ABS section loads/stores its fields through the generic settings API, the same way Calibre fields currently do.

3. The UI calls a dedicated preflight endpoint to:
   - validate ABS auth
   - fetch visible libraries
   - confirm the selected library id

4. When the admin clicks `Import`, the UI calls a fire-and-forget start endpoint modeled on `CalibreImportHandler.Start(...)`. The handler should:
   - load current `abs.*` config
   - reject empty/invalid config early
   - call `context.WithoutCancel(r.Context())`
   - return `202 Accepted` with the initial progress snapshot

5. `abs.Importer` should then follow the Calibre runner pattern:
   - single-run mutex/guard
   - process-local progress snapshot
   - synchronous `Run(...)` plus async `Start(...)`
   - final summary counters

6. For each ABS library item, the runner should:
   - normalize the paginated list payload into one internal ABS item shape
   - detect incomplete list items and fetch full detail when needed
   - resolve/create the Bindery author as a shared/global row (`owner_user_id = NULL`)
   - upsert the Bindery book as a shared/global row (`owner_user_id = NULL`)
   - upsert edition/provenance rows
   - record per-item outcome in run stats/history

7. For ownership/imported state:
   - if ABS exposes one or more paths Bindery can see on the same filesystem, verify they live under the effective library root and call `BookRepo.AddBookFile(...)` for each visible file
   - if ABS only has a single canonical path per format, `BookRepo.SetFormatFilePath(...)` is still acceptable as a thin wrapper
   - otherwise keep the row as metadata-only and surface it as pending/manual
   - if path visibility is uncertain, reuse `Scanner.ScanLibrary()` as the fallback reconciliation pass rather than writing a second matcher

8. At the end of a successful run, persist:
   - final summary counters
   - coarse success timestamp
   - checkpoint/completion state for resume/rerun safety

## Recommended File Layout

The smallest additive layout that matches current repo conventions is:

- `internal/abs/client.go`
  - ABS HTTP client
  - auth header injection
  - library listing
  - item listing/detail fetch
- `internal/abs/types.go`
  - list/detail response types
  - normalized import item shape
- `internal/abs/importer.go`
  - long-running runner
  - progress/stats/checkpoint handling
  - mapping + upsert orchestration
- `internal/api/abs.go`
  - `Test` and `Libraries` endpoints
- `internal/api/abs_import.go`
  - `Start` and `Status` endpoints
- `web/src/api/client.ts`
  - ABS settings/discovery/import methods
- `web/src/pages/SettingsPage.tsx`
  - `AudiobookshelfSection()` under `ImportTab()`

## Storage Recommendation

For the first ABS source, prefer settings rows over a new source table.

Suggested initial keys:

- `abs.base_url`
- `abs.api_key`
- `abs.library_id`
- `abs.enabled`
- `abs.label`
- `abs.last_import_at`

Why settings rows first:

- Bindery already has mature plumbing for `GET /setting` and `PUT /setting/{key}`.
- The Calibre section proves that settings-backed configuration plus dedicated `Test`/`Start` endpoints works well.
- The Phase 0 spec only requires one selected ABS library, not multi-source management.

Migration note:

- `origin/main` now ends at [internal/db/migrations/028_book_files.sql](../internal/db/migrations/028_book_files.sql).
- If ABS introduces tables later, start at `029_*.sql` or above; do not assume the pre-logs/pre-book-files numbering.

When to stop using settings rows:

- If ABS needs multiple named sources
- If checkpoint payloads get large
- If per-run history/querying becomes first-class

At that point, introduce a dedicated ABS source/run/checkpoint table instead of overloading `settings`.

## Reconciliation Map

### Preferred path

- Shared-path visible ABS item
- verify path/folder exists and is under the effective library root (`author.root_folder_id` -> `library.defaultRootFolderId` -> env fallback)
- call `BookRepo.AddBookFile(...)` for each visible file
- let existing `NeedsEbook()` / `NeedsAudiobook()` logic decide whether status flips to `imported`

### Fallback path

- ABS metadata import succeeds but path is not locally visible
- store provenance and leave the book in a non-imported state
- optionally expose a manual reconcile action that triggers `LibraryHandler.Scan(...)`

## Ownership Policy

- ABS is a global background importer, not a per-user importer.
- New ABS-imported catalog rows should default to shared/global ownership by storing `owner_user_id = NULL`.
- Do not assign ABS-imported catalog rows to the admin account unless product requirements explicitly change later.

### Existing helpers to lean on

- [internal/importer/scanner.go](../internal/importer/scanner.go) `FindExisting(...)`
- [internal/importer/scanner.go](../internal/importer/scanner.go) `ScanLibrary(...)`
- [internal/db/books.go](../internal/db/books.go) `AddBookFile(...)`
- [internal/db/books.go](../internal/db/books.go) `SetFormatFilePath(...)`
- [internal/api/books.go](../internal/api/books.go) `DeleteFile(...)`
- [internal/api/bulk.go](../internal/api/bulk.go) `reevaluateBookStatus(...)`

## Reporting Map

ABS should reuse Bindery’s current reporting style instead of inventing a metrics subsystem in phase 1:

- polled progress snapshot like `calibre.ImportProgress`
- final summary counters like `calibre.ImportStats`
- per-item failure collection like `calibre.SyncError` or `migrate.Result.Failures`
- structured `slog` fields
- persistent history events via [internal/db/history.go](../internal/db/history.go)

Recommended ABS counters:

- librariesScanned
- itemsSeen
- itemsDetailFetched
- authorsCreated
- booksCreated
- booksLinked
- editionsCreated
- itemsMarkedOwned
- itemsPendingManual
- skipped
- failed

## Gaps That Need New Code

These do not have a strong existing reuse target and should be treated as new ABS-specific work:

- paginated ABS library enumeration
- incomplete-item detection plus detail-fetch fallback
- ABS provenance persistence
- resumable checkpoints
- true dry-run mode

That is still compatible with the recommended reuse path: new ABS-specific enumeration/checkpoint code can sit on top of existing Bindery config, runner, reporting, and reconcile seams.
