# ABS Import Contract Matrix

Pinned baseline: `2.33.2`

Harness layers:

- `internal/abs/*_test.go` keeps request/response normalization pinned to captured `v2.33.2` fixtures.
- `tests/abscontract` runs an end-to-end contract suite against a seeded Phase 6 fixture harness.
- The contract suite can optionally target an external pinned ABS instance when `BINDERY_ABS_CONTRACT_BASE_URL` and related env vars are provided.

Seeded scenarios:

- `single-file-audiobook`: complete list payload, no detail fetch required.
- `folder-multi-file-audiobook`: folder-backed item, detail fetch required.
- `ebook-only-item`: ebook provenance retained without forcing ABS-specific downloader behavior.
- `mixed-metadata-completeness`: list payload omits detail-critical duration/size metadata, detail fetch required.
- `series-linked-item`: complete series metadata survives normalization and importer mapping.
- `permission-limited-account`: non-admin API key can access only the selected book library.

Covered behaviors:

- auth success and auth failure
- permission-scoped library listing
- paging across a pinned multi-page library
- detail-fetch fallback for incomplete list payloads
- dry-run summary/report behavior
- idempotent rerun
- interrupted-run resume from persisted checkpoint
- CI gating via the `ABS contract` workflow job
