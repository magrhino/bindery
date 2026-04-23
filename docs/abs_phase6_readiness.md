# ABS Phase 6 Readiness

Phase 6 should build on the current ABS importer seams rather than inventing a second test harness shape.

## Reuse

- Import runner: [internal/abs/importer.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/importer.go)
  - `Importer.Run`
  - `Importer.Start`
  - `Importer.RecentRuns`
  - `Importer.RollbackPreview`
  - `Importer.Rollback`
  - `HydrateRun`
  - `ImportStats`, `ImportSummary`, `ImportProgress`
- Paging and resume seam: [internal/abs/enumerator.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/enumerator.go)
  - `Enumerator.Enumerate`
  - `Enumerator.WithCheckpointObserver`
  - `ImportCheckpoint`
- Run tracking and rollback provenance: [internal/db/abs_imports.go](/Users/ryanjones/Documents/bindery/bindery/internal/db/abs_imports.go)
  - `ABSImportRunRepo.Create`
  - `ABSImportRunRepo.UpdateCheckpoint`
  - `ABSImportRunRepo.Finish`
  - `ABSImportRunEntityRepo.Record`
  - `ABSImportRunEntityRepo.ListByRun`
  - `ABSProvenanceRepo.Upsert`
  - `ABSProvenanceRepo.GetByExternal`
- Existing ABS tests to extend:
  - [internal/abs/client_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/client_test.go)
  - [internal/abs/contract_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/contract_test.go)
  - [internal/abs/enumerator_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/enumerator_test.go)
  - [internal/abs/importer_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/importer_test.go)
  - [internal/api/abs_import_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/api/abs_import_test.go)
- Existing CI/test wiring to mirror:
  - [Makefile](/Users/ryanjones/Documents/bindery/bindery/Makefile) targets `smoke`, `predeploy-smoke`
  - [tests/smoke/smoke_test.go](/Users/ryanjones/Documents/bindery/bindery/tests/smoke/smoke_test.go)
  - [.github/workflows/ci.yml](/Users/ryanjones/Documents/bindery/bindery/.github/workflows/ci.yml) jobs `validate`, `smoke`, `predeploy-smoke`

## Phase 5 Prerequisites

- Dry-run behavior exists and is already asserted in [internal/abs/importer_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/importer_test.go:814).
- Resume/checkpoint behavior exists at the enumerator layer and is asserted in [internal/abs/enumerator_test.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/enumerator_test.go:171).
- Import-run summaries/reporting exist through `ImportStats`, `ImportSummary`, persisted `abs_import_runs`, and `HydrateRun`.
- Rollback-safe provenance markers exist through `abs_provenance`, `abs_import_run_entities`, and the rollback planner in [internal/abs/importer.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/importer.go:1654).

## Missing Or Weak Phase 5 Seams

- Failed runs currently clear persisted `checkpoint_json` in [internal/db/abs_imports.go](/Users/ryanjones/Documents/bindery/bindery/internal/db/abs_imports.go:53), which weakens interrupted-run inspection and makes importer-level resume assertions less trustworthy than the enumerator-level test.
- Rollback does not currently update the run status to `rolled_back` even though the constant exists in [internal/abs/importer.go](/Users/ryanjones/Documents/bindery/bindery/internal/abs/importer.go:36). Phase 6 can still test rollback safety, but operator-facing run history is not yet ideal.
- There is no importer-level interrupted-run test yet; the current resume proof is only at the enumerator seam.
- There is no snapshot-style summary/report assertion yet; current coverage checks selected counters only.

## Recommended Harness Shape

- Keep the pinned baseline single-version first. Use one post-`v2.26` ABS baseline only: `2.33.2`.
- Put the new harness under [tests/abscontract](/Users/ryanjones/Documents/bindery/bindery/tests/abscontract) so it mirrors existing top-level smoke suites instead of expanding `internal/abs` unit fixtures into an ersatz integration rig.
- Phase 6 should be two layers:
  - Layer 1: keep the existing `internal/abs` fixture and `httptest` coverage for request/response normalization.
  - Layer 2: add a pinned contract suite that talks to one seeded ABS instance and then drives Bindery’s ABS client/importer seams against that instance.
- The contract suite should validate:
  - auth success/failure
  - library listing and library filtering
  - paging
  - detail-fetch fallback
  - idempotent rerun
  - dry-run
  - resume once failed-run checkpoint persistence is tightened
- Prefer environment-driven harness config first:
  - base URL
  - API key
  - library ID
  - pinned version expectation
- Delay container orchestration code until the seeded fixture library layout is stable; the first useful contract pass can target an externally started pinned instance.

## Recommended Fixture Dataset Shape

Use one seeded ABS library with explicit scenario directories declared in a manifest:

- `single-file-audiobook`
- `folder-multi-file-audiobook`
- `ebook-only-item`
- `mixed-metadata-completeness`
- `series-linked-item`
- `permission-limited-account`

Keep one manifest as the source of truth for:

- scenario id
- intended media shape
- expected ABS item/library identifiers
- whether detail fetch is expected
- whether dry-run should create only virtual results
- whether rollback should only unlink provenance vs delete created rows

## Smallest CI Gate Strategy

- Reuse the existing `Makefile` + Go-test pattern instead of introducing a separate runner.
- Add one dormant CI job that runs `make abs-contract`.
- Gate activation behind a repo variable so the stub stays non-blocking until the harness is green.
- Once green:
  - enable the job on pull requests
  - make ABS importer changes require that job
  - keep it pinned to the single baseline before considering a version matrix

## New Scaffolding In This Pass

- Contract scaffold: [tests/abscontract](/Users/ryanjones/Documents/bindery/bindery/tests/abscontract)
- Fixture manifest: [tests/abscontract/testdata/fixtures/manifest.json](/Users/ryanjones/Documents/bindery/bindery/tests/abscontract/testdata/fixtures/manifest.json)
- Dormant CI hook: [.github/workflows/ci.yml](/Users/ryanjones/Documents/bindery/bindery/.github/workflows/ci.yml)
- Guarded-disable seam: `BINDERY_ABS_ENABLED`, exposed to the admin ABS config response and used to hide the ABS settings tab plus omit ABS mutating routes when disabled.
