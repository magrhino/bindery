package abscontract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vavallee/bindery/internal/abs"
	"github.com/vavallee/bindery/internal/db"
)

func TestFixtureManifestMatchesPinnedBaseline(t *testing.T) {
	t.Parallel()

	cfg := LoadHarnessConfig()
	manifest, err := LoadFixtureManifest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BaselineVersion != cfg.Baseline.Version {
		t.Fatalf("fixture manifest baseline = %q, want %q", manifest.BaselineVersion, cfg.Baseline.Version)
	}
	if len(manifest.Scenarios) < 6 {
		t.Fatalf("fixture scenarios = %d, want at least 6 Phase 6 scenarios", len(manifest.Scenarios))
	}

	seen := map[string]struct{}{}
	for _, scenario := range manifest.Scenarios {
		if scenario.ID == "" {
			t.Fatal("fixture scenario id is required")
		}
		if _, ok := seen[scenario.ID]; ok {
			t.Fatalf("fixture scenario %q is duplicated", scenario.ID)
		}
		seen[scenario.ID] = struct{}{}
		if scenario.SeedPath == "" {
			t.Fatalf("fixture scenario %q seedPath is required", scenario.ID)
		}
		if _, err := os.Stat(filepath.Join("testdata", "fixtures", scenario.SeedPath)); err != nil {
			t.Fatalf("fixture scenario %q seed path missing: %v", scenario.ID, err)
		}
	}
}

func TestContractHarness_ClientAuthAndPermissionScope(t *testing.T) {
	t.Parallel()

	cfg := LoadHarnessConfig()
	manifest, err := LoadFixtureManifest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	harness := newFixtureABSHarness(t, cfg, manifest)

	client, err := abs.NewClient(harness.BaseURL(), cfg.APIKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	authz, err := client.Authorize(context.Background())
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if authz.ServerSettings.Version != cfg.Baseline.Version {
		t.Fatalf("ABS version = %q, want %q", authz.ServerSettings.Version, cfg.Baseline.Version)
	}

	libraries, err := client.ListLibraries(context.Background())
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libraries) < 2 {
		t.Fatalf("libraries = %d, want at least 2 for contract coverage", len(libraries))
	}

	limitedClient, err := abs.NewClient(harness.BaseURL(), cfg.LimitedAPIKey)
	if err != nil {
		t.Fatalf("NewClient limited: %v", err)
	}
	limitedLibraries, err := limitedClient.ListLibraries(context.Background())
	if err != nil {
		t.Fatalf("ListLibraries limited: %v", err)
	}
	if len(limitedLibraries) != 1 || limitedLibraries[0].ID != cfg.LibraryID {
		t.Fatalf("limited libraries = %+v, want only %q", limitedLibraries, cfg.LibraryID)
	}

	badClient, err := abs.NewClient(harness.BaseURL(), "bad-key")
	if err != nil {
		t.Fatalf("NewClient bad key: %v", err)
	}
	_, err = badClient.Authorize(context.Background())
	var apiErr *abs.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		t.Fatalf("Authorize bad key error = %v, want 401 APIError", err)
	}
}

func TestContractHarness_EnumeratesPagingAndDetailFallback(t *testing.T) {
	t.Parallel()

	cfg := LoadHarnessConfig()
	manifest, err := LoadFixtureManifest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	harness := newFixtureABSHarness(t, cfg, manifest)

	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	settings := db.NewSettingsRepo(database)

	client, err := abs.NewClient(harness.BaseURL(), cfg.APIKey)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	enumerator := abs.NewEnumerator(client, settings, 50)
	got := map[string]abs.NormalizedLibraryItem{}
	stats, err := enumerator.Enumerate(context.Background(), cfg.LibraryID, func(_ context.Context, item abs.NormalizedLibraryItem) error {
		got[item.ItemID] = item
		return nil
	})
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if stats.PagesScanned != 3 {
		t.Fatalf("pagesScanned = %d, want 3", stats.PagesScanned)
	}
	if stats.ItemsSeen != 5 || stats.ItemsNormalized != 5 {
		t.Fatalf("stats = %+v, want 5 items seen/normalized", stats)
	}
	if stats.ItemsDetailFetched != 2 {
		t.Fatalf("itemsDetailFetched = %d, want 2", stats.ItemsDetailFetched)
	}

	for _, scenario := range manifest.Scenarios {
		if scenario.ExpectedItemID == "" {
			continue
		}
		item, ok := got[scenario.ExpectedItemID]
		if !ok {
			t.Fatalf("missing normalized item for scenario %q (%s)", scenario.ID, scenario.ExpectedItemID)
		}
		if item.MediaType != scenario.ExpectedMediaType {
			t.Fatalf("scenario %q mediaType = %q, want %q", scenario.ID, item.MediaType, scenario.ExpectedMediaType)
		}
		if item.DetailFetched != scenario.RequiresDetail {
			t.Fatalf("scenario %q detailFetched = %v, want %v", scenario.ID, item.DetailFetched, scenario.RequiresDetail)
		}
		if scenario.ExpectsEbook && item.EbookPath == "" {
			t.Fatalf("scenario %q expected ebook path", scenario.ID)
		}
		if scenario.ExpectsSeries && len(item.Series) == 0 {
			t.Fatalf("scenario %q expected linked series metadata", scenario.ID)
		}
	}
}

func TestContractHarness_ImporterDryRunAndIdempotentRerun(t *testing.T) {
	t.Parallel()

	cfg := LoadHarnessConfig()
	manifest, err := LoadFixtureManifest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	harness := newFixtureABSHarness(t, cfg, manifest)
	importer, authorRepo, bookRepo, _, runRepo := newContractImporterFixture(t)

	dryRunStats, err := importer.Run(context.Background(), abs.ImportConfig{
		SourceID:  abs.DefaultSourceID,
		BaseURL:   harness.BaseURL(),
		APIKey:    cfg.APIKey,
		LibraryID: cfg.LibraryID,
		Label:     "Fixture Shelf",
		Enabled:   true,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("dry-run Run: %v", err)
	}
	if dryRunStats.BooksCreated != 3 || dryRunStats.AuthorsCreated != 3 || dryRunStats.ReviewQueued != 2 {
		t.Fatalf("dry-run stats = %+v, want 3 created books/authors and 2 queued reviews", dryRunStats)
	}

	authors, err := authorRepo.List(context.Background())
	if err != nil {
		t.Fatalf("List authors: %v", err)
	}
	if len(authors) != 0 {
		t.Fatalf("authors = %d, want 0 after dry-run", len(authors))
	}
	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("List books after dry-run: %v", err)
	}
	if len(books) != 0 {
		t.Fatalf("books = %d, want 0 after dry-run", len(books))
	}

	commitStats, err := importer.Run(context.Background(), abs.ImportConfig{
		SourceID:  abs.DefaultSourceID,
		BaseURL:   harness.BaseURL(),
		APIKey:    cfg.APIKey,
		LibraryID: cfg.LibraryID,
		Label:     "Fixture Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("commit Run: %v", err)
	}
	if commitStats.BooksCreated != 3 || commitStats.ReviewQueued != 2 {
		t.Fatalf("commit stats = %+v, want 3 created books and 2 queued reviews", commitStats)
	}
	books, err = bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("List books after commit: %v", err)
	}
	if len(books) != 3 {
		t.Fatalf("books = %d, want 3 after commit", len(books))
	}

	rerunStats, err := importer.Run(context.Background(), abs.ImportConfig{
		SourceID:  abs.DefaultSourceID,
		BaseURL:   harness.BaseURL(),
		APIKey:    cfg.APIKey,
		LibraryID: cfg.LibraryID,
		Label:     "Fixture Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("rerun Run: %v", err)
	}
	if rerunStats.BooksCreated != 0 {
		t.Fatalf("rerun stats = %+v, want no new books", rerunStats)
	}
	books, err = bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("List books after rerun: %v", err)
	}
	if len(books) != 3 {
		t.Fatalf("books after rerun = %d, want 3", len(books))
	}

	runs, err := runRepo.ListRecent(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(runs))
	}
	latestDryRun := abs.HydrateRun(runs[2])
	if !latestDryRun.Summary.DryRun || latestDryRun.Summary.Stats.BooksCreated != 3 || latestDryRun.Summary.Stats.ReviewQueued != 2 {
		t.Fatalf("dry-run summary = %+v", latestDryRun.Summary)
	}
}

func TestContractHarness_ImporterResumeFromFailedCheckpoint(t *testing.T) {
	t.Parallel()

	cfg := LoadHarnessConfig()
	manifest, err := LoadFixtureManifest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	harness := newFixtureABSHarness(t, cfg, manifest)
	harness.FailPage(1, 3)

	importer, _, bookRepo, settingsRepo, runRepo := newContractImporterFixture(t)
	_, err = importer.Run(context.Background(), abs.ImportConfig{
		SourceID:  abs.DefaultSourceID,
		BaseURL:   harness.BaseURL(),
		APIKey:    cfg.APIKey,
		LibraryID: cfg.LibraryID,
		Label:     "Fixture Shelf",
		Enabled:   true,
	})
	if err == nil {
		t.Fatal("expected first run to fail on injected page error")
	}

	failedRuns, err := runRepo.ListRecent(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListRecent failed run: %v", err)
	}
	if len(failedRuns) != 1 {
		t.Fatalf("failed runs = %d, want 1", len(failedRuns))
	}
	failed := abs.HydrateRun(failedRuns[0])
	if failed.Status != "failed" {
		t.Fatalf("failed run status = %q, want failed", failed.Status)
	}
	if failed.Checkpoint == nil || failed.Checkpoint.Page != 1 {
		t.Fatalf("failed run checkpoint = %+v, want page 1", failed.Checkpoint)
	}
	setting, err := settingsRepo.Get(context.Background(), abs.SettingABSImportCheckpoint)
	if err != nil {
		t.Fatalf("Get checkpoint setting: %v", err)
	}
	if setting == nil || setting.Value == "" {
		t.Fatal("expected importer failure to retain checkpoint setting")
	}

	resumeStats, err := importer.Run(context.Background(), abs.ImportConfig{
		SourceID:  abs.DefaultSourceID,
		BaseURL:   harness.BaseURL(),
		APIKey:    cfg.APIKey,
		LibraryID: cfg.LibraryID,
		Label:     "Fixture Shelf",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("resume Run: %v", err)
	}
	if resumeStats.BooksCreated != 1 || resumeStats.ReviewQueued != 2 {
		t.Fatalf("resume stats = %+v, want 1 newly created book and 2 queued reviews after resuming", resumeStats)
	}

	books, err := bookRepo.ListIncludingExcluded(context.Background())
	if err != nil {
		t.Fatalf("List books after resume: %v", err)
	}
	if len(books) != 3 {
		t.Fatalf("books after resume = %d, want 3", len(books))
	}

	runs, err := runRepo.ListRecent(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListRecent after resume: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs after resume = %d, want 2", len(runs))
	}
	resumed := abs.HydrateRun(runs[0])
	if resumed.Status != "completed" {
		t.Fatalf("resumed run status = %q, want completed", resumed.Status)
	}
	if !resumed.Summary.ResumedFromCheckpoint {
		t.Fatalf("resumed summary = %+v, want resumedFromCheckpoint=true", resumed.Summary)
	}
	setting, err = settingsRepo.Get(context.Background(), abs.SettingABSImportCheckpoint)
	if err != nil {
		t.Fatalf("Get checkpoint setting after resume: %v", err)
	}
	if setting != nil {
		t.Fatalf("checkpoint setting = %+v, want cleared after successful resume", setting)
	}
}
