# ABS Import

Bindery can import Audiobookshelf (ABS) catalog metadata into the main Bindery library so you do not have to recreate authors, books, series, and editions by hand.

This page is the user-facing companion to [`docs/abs_import.md`](./abs_import.md), which remains the implementation-focused reference for contributors.

## What It Does

The current ABS importer supports one configured ABS source and one selected target book library at a time. From that source, Bindery can:

- test API-key access and discover the ABS libraries visible to that key
- import the ABS catalog metadata it can read for each visible item
- create or match shared authors, books, series, and editions in Bindery
- preserve ABS provenance so reruns stay idempotent
- queue low-confidence items for manual review instead of guessing
- show recent runs, dry-run previews, rollback previews, and conflict resolution state

The importer is metadata-first and shared-filesystem-aware. If ABS reports file paths that Bindery can see under its configured library roots, Bindery records those files through the normal file ownership logic. If the paths are missing, outside scope, or mounted differently, the import can still succeed as metadata-only.

The importer also supports ABS-to-Bindery path remaps when the same files are mounted at different prefixes on each side. For example, ABS might report `/audiobookshelf/media/Author/Book` while Bindery sees the same files at `/books/media/Author/Book`. In that case you can add a path remap so Bindery rewrites the ABS prefix before validating and attaching files.

## What Gets Imported

Bindery does not just create a shell entry with a title. It imports the ABS metadata fields it can map into Bindery's model, including:

- authors
- books
- series and sequence numbers when ABS provides them
- ebook and audiobook editions
- description
- release date
- genres
- language
- narrator
- duration
- ASIN
- media type
- ABS provenance and import-run history

If an ABS field has no Bindery equivalent, it may not be stored. This importer is broad, but it is not a raw mirror of every ABS field.

## Import Quality Matters

Import quality depends heavily on ABS metadata quality.

The more books you already have in Audiobookshelf with complete, high-quality metadata, the better the import will go. In particular, stable identifiers like ASINs make matching much more reliable, reduce ambiguous title-only decisions, and shrink the manual review queue.

A good pre-import cleanup pass in ABS is worth it. If your library already has strong author metadata, consistent titles, linked series, and ASINs where available, Bindery can resolve more items automatically and with fewer conflicts.

## Before You Start

- Use an ABS API key for a user that can see the target book library. ABS admin access is not required. The key only needs permission to authenticate and read the library you plan to import.
- Pick the one ABS library you want to import from.
- If ABS and Bindery see the same storage under different mount prefixes, configure path remaps such as `/audiobookshelf:/books/audiobookshelf`.
- Path remaps are prefix rewrites. The left side is the path prefix ABS reports, and the right side is the path prefix Bindery can actually read. Bindery applies the rewrite before checking whether the resolved file lives under a Bindery-visible library root.
- If you want the best initial match rate, it helps to make sure your ABS library metadata is already in good shape before importing.

## Import Flow

1. Open `Settings -> ABS`.
2. Save the ABS base URL, API key, label, target library, and any optional path remaps.
3. Test the connection and list available libraries.
4. Start a dry run if you want a safe first pass.
5. Start the import.
6. Review any queued items or metadata conflicts.
7. Use rollback preview or rollback if you need to undo a run.

## Review Queue And Conflicts

Bindery deliberately fails safe when matching is unclear.

- Low-confidence items are sent to the review queue so you can resolve the author or book match yourself.
- When ABS metadata and upstream metadata disagree for mapped fields, Bindery keeps the current applied value temporarily and records a conflict so you can choose the winning source.
- Placeholder ABS authors can be relinked during conflict review when Bindery can confidently connect them to upstream metadata.

## Known Behavior

- Only one ABS source is supported in the current MVP.
- Only one selected ABS library is imported per run.
- Imports are asynchronous.
- Non-visible file paths become metadata-only imports instead of hard failures.
- Ambiguous title matches are not auto-applied.

## Troubleshooting

- Connection test fails: verify the ABS base URL, API key, and that the key can see the selected library.
- Files are not attaching after import: check path remaps and make sure the ABS-reported paths resolve under a Bindery-visible library root.
- Too many review items: improve ABS metadata quality first, especially ASIN coverage and author/title consistency, then rerun.
- Unexpected metadata disagreements: resolve them from the conflicts panel instead of rerunning until the same field flips back and forth.

## See Also

- [`docs/abs_import.md`](./abs_import.md)
- [`docs/DEPLOYMENT.md`](./DEPLOYMENT.md)
