# ABS Import Mapping Spec

Phase 3 mapping rules for the Audiobookshelf importer.

## Scope

This phase imports ABS metadata into Bindery's catalog and records enough provenance to make reruns idempotent and traceable.

- No file ownership/imported-state reconciliation yet.
- No dry-run mode yet.
- Imported ABS-created catalog rows remain global/shared by default because the current `Create(...)` paths do not assign `owner_user_id`.

## Source Of Truth

The normalized ABS item from [internal/abs/types.go](../internal/abs/types.go) is the source payload for mapping.

The importer implementation lives in [internal/abs/importer.go](../internal/abs/importer.go).

Provenance persistence lives in:

- [internal/db/migrations/029_abs_imports.sql](../internal/db/migrations/029_abs_imports.sql)
- [internal/db/abs_imports.go](../internal/db/abs_imports.go)

## Mapping Rules

### Authors

Primary author resolution order:

1. `abs_provenance` lookup by `entity_type='author'` and ABS author id.
2. Exact case-insensitive author name match in `authors.name`.
3. Exact case-insensitive alias match in `author_aliases.name`.
4. Create a new shared author with:
   - `foreign_id = abs:author:<library_id>:<abs_author_id_or_name_key>`
   - `metadata_provider = 'audiobookshelf'`
   - `sort_name` derived from the full name when ABS does not provide one.

Secondary ABS authors are recorded as aliases on the canonical author when possible.

### Book / Work

Book resolution order:

1. `abs_provenance` lookup by `entity_type='book'` and ABS item id.
2. `books.foreign_id = abs:book:<library_id>:<item_id>` for crash recovery if provenance write was missed.
3. Existing book under the resolved author with the same normalized title.
   - Normalization uses Bindery's existing dedupe helper in [internal/indexer/dedup.go](../internal/indexer/dedup.go).
   - If more than one existing row matches, the importer fails safe for that item instead of guessing.
4. Create a new shared book.

Applied book fields:

- `title`
- `sort_title`
- `description` when non-empty
- `release_date` from ABS published date/year
- `genres`
- `language`
- `narrator`
- `duration_seconds`
- `asin`
- `media_type`
- `metadata_provider` only when the existing row does not already have one

Books created through ABS are currently imported with `status='wanted'`. Phase 4 will layer ownership/imported-state reconciliation on top.

### Series

Series resolution order:

1. `abs_provenance` lookup by `entity_type='series'` and ABS series id.
2. Existing `series` row with the same normalized title.
3. Create a new series with `foreign_id = abs:series:<library_id>:<series_id_or_name_key>`.

Series links are written through `series_books` using the ABS sequence string when present.

### Editions

Each ABS item upserts one or two Bindery editions:

- `ebook` edition when ABS exposes an ebook path
- `audiobook` edition when ABS exposes audio files

Edition foreign ids are deterministic:

- `abs:edition:<library_id>:<item_id>:ebook`
- `abs:edition:<library_id>:<item_id>:audiobook`

Edition fields populated:

- title
- ISBN-13 / ISBN-10 when derivable
- ASIN
- publisher
- publish date
- format
- language
- `is_ebook`
- `edition_info = "Imported from Audiobookshelf"`

## Provenance Model

`abs_import_runs` records the batch/run envelope:

- source id
- source label
- base URL snapshot
- library id
- status
- summary JSON
- started / finished timestamps

`abs_provenance` records entity-level mapping:

- `entity_type` in `author | book | series | edition`
- ABS external id
- local Bindery row id
- ABS item id when applicable
- format when applicable
- ABS file ids / equivalent file provenance
- latest import run id

This makes it possible to:

- rerun the same ABS import without duplicating rows
- trace a Bindery row back to its ABS item/entity
- recover safely if a prior run created rows before the provenance write completed

## Failure Policy

The importer is additive and fail-safe:

- missing primary author or title fails only that item
- ambiguous normalized-title matches fail only that item
- series or edition upsert issues are logged per item without crashing the whole import unless the underlying enumerate/run context fails

## Reporting

The importer reports:

- page/item enumeration counts
- created/linked/updated counts for authors, books, series, and editions
- skipped / failed counts
- per-item outcomes in progress snapshots

API surface:

- `POST /api/v1/abs/import`
- `GET /api/v1/abs/import/status`
