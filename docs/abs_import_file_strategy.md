# ABS Import File Strategy

## MVP decision

Phase 4 uses a shared-filesystem, metadata-first strategy:

- Bindery imports ABS catalog metadata and provenance first.
- For each imported item, Bindery tries to verify the ABS-reported ebook and audiobook paths on the local filesystem.
- When a verified path is inside Bindery's configured library storage, the importer reuses `BookRepo.SetFormatFilePath` to register it in `book_files` and let normal status recalculation mark the format as owned/imported.
- When a referenced path is missing or outside Bindery's visible storage, the import still succeeds as metadata-only and the run records a pending/manual follow-up count instead of failing the whole batch.

## Why this is the MVP path

- It reuses the existing `book_files` plus aggregate status logic instead of creating an ABS-only ownership state.
- It matches the current Bindery assumption that imported/owned means Bindery can see the file or folder on disk.
- It avoids a new HTTP download/import pipeline, which would add more failure modes and more cleanup work than the MVP needs.

## Current reconciliation rules

- Ebook ownership uses `NormalizedLibraryItem.EbookPath`.
- Audiobook ownership uses the ABS item `Path`, which may be a single file or a folder-backed audiobook directory.
- Paths are only accepted when they resolve under Bindery's configured library roots:
  - `BINDERY_LIBRARY_DIR`
  - `BINDERY_AUDIOBOOK_DIR`
  - the author's explicit root folder, when present
  - the default root folder from `library.defaultRootFolderId`, when present
- Missing or out-of-scope paths leave the book in its metadata/imported-provenance state without forcing `status=imported`.

## Deferred work

- HTTP download fallback for non-shared-path deployments
- richer operator review UI for pending/manual ABS items
- batch rollback beyond the provenance/run scaffolding already added in earlier phases
