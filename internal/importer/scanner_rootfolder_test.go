package importer

import (
	"context"
	"fmt"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

// scannerWithRootFolders builds a Scanner wired to a real in-memory DB that
// includes a RootFolderRepo, so effectiveLibraryDir can be exercised end-to-end.
func scannerWithRootFolders(t *testing.T, libraryDir string) (*Scanner, *db.RootFolderRepo, *db.AuthorRepo, context.Context) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	books := db.NewBookRepo(database)
	authors := db.NewAuthorRepo(database)
	history := db.NewHistoryRepo(database)
	downloads := db.NewDownloadRepo(database)
	clients := db.NewDownloadClientRepo(database)
	rf := db.NewRootFolderRepo(database)

	s := NewScanner(downloads, clients, books, authors, history, libraryDir, "", "", "", "")
	s.WithRootFolders(rf)
	return s, rf, authors, context.Background()
}

// scannerWithRootFoldersAndSettings builds a Scanner with both RootFolderRepo
// and SettingsRepo wired so the default-root-folder setting can be exercised.
func scannerWithRootFoldersAndSettings(t *testing.T, libraryDir string) (*Scanner, *db.RootFolderRepo, *db.SettingsRepo, context.Context) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	books := db.NewBookRepo(database)
	authors := db.NewAuthorRepo(database)
	history := db.NewHistoryRepo(database)
	downloads := db.NewDownloadRepo(database)
	clients := db.NewDownloadClientRepo(database)
	rf := db.NewRootFolderRepo(database)
	settings := db.NewSettingsRepo(database)

	s := NewScanner(downloads, clients, books, authors, history, libraryDir, "", "", "", "")
	s.WithRootFolders(rf)
	s.WithSettings(settings)
	return s, rf, settings, context.Background()
}

func TestEffectiveLibraryDir_NilAuthor(t *testing.T) {
	s, _, _, ctx := scannerWithRootFolders(t, "/default/lib")
	got := s.effectiveLibraryDir(ctx, nil)
	if got != "/default/lib" {
		t.Errorf("nil author: want /default/lib, got %q", got)
	}
}

func TestEffectiveLibraryDir_AuthorNoRootFolder(t *testing.T) {
	s, _, _, ctx := scannerWithRootFolders(t, "/default/lib")
	author := &models.Author{RootFolderID: nil}
	got := s.effectiveLibraryDir(ctx, author)
	if got != "/default/lib" {
		t.Errorf("author with nil RootFolderID: want /default/lib, got %q", got)
	}
}

func TestEffectiveLibraryDir_AuthorWithRootFolder(t *testing.T) {
	dir := t.TempDir()
	s, rf, _, ctx := scannerWithRootFolders(t, "/default/lib")

	created, err := rf.Create(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	id := created.ID
	author := &models.Author{RootFolderID: &id}
	got := s.effectiveLibraryDir(ctx, author)
	if got != dir {
		t.Errorf("want %q, got %q", dir, got)
	}
}

func TestEffectiveLibraryDir_AuthorWithDeletedRootFolder(t *testing.T) {
	dir := t.TempDir()
	s, rf, _, ctx := scannerWithRootFolders(t, "/default/lib")

	created, err := rf.Create(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	id := created.ID
	if err := rf.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}

	author := &models.Author{RootFolderID: &id}
	got := s.effectiveLibraryDir(ctx, author)
	// Deleted folder → falls back to global default
	if got != "/default/lib" {
		t.Errorf("deleted folder: want /default/lib, got %q", got)
	}
}

func TestEffectiveLibraryDir_NoRootFolderRepo(t *testing.T) {
	// Scanner without WithRootFolders — should always use libraryDir.
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	books := db.NewBookRepo(database)
	authors := db.NewAuthorRepo(database)
	history := db.NewHistoryRepo(database)
	downloads := db.NewDownloadRepo(database)
	clients := db.NewDownloadClientRepo(database)
	s := NewScanner(downloads, clients, books, authors, history, "/default/lib", "", "", "", "")
	// No WithRootFolders call — rootFolders field is nil.

	id := int64(42)
	author := &models.Author{RootFolderID: &id}
	got := s.effectiveLibraryDir(context.Background(), author)
	if got != "/default/lib" {
		t.Errorf("no repo: want /default/lib, got %q", got)
	}
}

func TestEffectiveLibraryDir_DefaultRootFolderSetting(t *testing.T) {
	dir := t.TempDir()
	s, rf, settings, ctx := scannerWithRootFoldersAndSettings(t, "/default/lib")

	created, err := rf.Create(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(ctx, "library.defaultRootFolderId", fmt.Sprintf("%d", created.ID)); err != nil {
		t.Fatal(err)
	}

	// Author with no explicit root folder should use the default setting.
	author := &models.Author{RootFolderID: nil}
	got := s.effectiveLibraryDir(ctx, author)
	if got != dir {
		t.Errorf("default setting: want %q, got %q", dir, got)
	}
}

func TestEffectiveLibraryDir_DefaultRootFolderUnset(t *testing.T) {
	s, _, _, ctx := scannerWithRootFoldersAndSettings(t, "/default/lib")

	// No setting → falls back to libraryDir.
	author := &models.Author{RootFolderID: nil}
	got := s.effectiveLibraryDir(ctx, author)
	if got != "/default/lib" {
		t.Errorf("unset default: want /default/lib, got %q", got)
	}
}

func TestEffectiveLibraryDir_DefaultRootFolderDeletedReverts(t *testing.T) {
	dir := t.TempDir()
	s, rf, settings, ctx := scannerWithRootFoldersAndSettings(t, "/default/lib")

	created, err := rf.Create(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(ctx, "library.defaultRootFolderId", fmt.Sprintf("%d", created.ID)); err != nil {
		t.Fatal(err)
	}
	// Delete the root folder — default should revert to libraryDir.
	if err := rf.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	author := &models.Author{RootFolderID: nil}
	got := s.effectiveLibraryDir(ctx, author)
	if got != "/default/lib" {
		t.Errorf("deleted default RF: want /default/lib, got %q", got)
	}
}

func TestEffectiveLibraryDir_AuthorRootFolderTakesPriorityOverDefault(t *testing.T) {
	authorDir := t.TempDir()
	defaultDir := t.TempDir()
	s, rf, settings, ctx := scannerWithRootFoldersAndSettings(t, "/default/lib")

	authorRF, err := rf.Create(ctx, authorDir)
	if err != nil {
		t.Fatal(err)
	}
	defaultRF, err := rf.Create(ctx, defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(ctx, "library.defaultRootFolderId", fmt.Sprintf("%d", defaultRF.ID)); err != nil {
		t.Fatal(err)
	}

	// Author with explicit root folder — should use that, not the default.
	author := &models.Author{RootFolderID: &authorRF.ID}
	got := s.effectiveLibraryDir(ctx, author)
	if got != authorDir {
		t.Errorf("author RF should take priority: want %q, got %q", authorDir, got)
	}
}
