package importer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

// perFileCollisionFixture builds the exact shape reported in #2275: a
// multi-file audiobook torrent whose files live in sibling subdirectories of
// the download root, so no directory strictly below the root contains them all
// and resolveAudiobookSource falls through to per-file placement — and two of
// those files carry the same basename with different contents.
//
// Returned paths are the two colliding sources, in placement order.
func perFileCollisionFixture(t *testing.T, importMode string) (
	s *Scanner, dl *models.Download, dlRepo *db.DownloadRepo,
	downloadRoot string, explicit []string, ctx context.Context,
) {
	t.Helper()
	libraryDir := t.TempDir()
	s, dl, dlRepo, bookRepo, ctx := dataLossFixture(t, libraryDir, importMode)

	book, err := bookRepo.GetByID(ctx, *dl.BookID)
	if err != nil {
		t.Fatal(err)
	}
	book.MediaType = models.MediaTypeAudiobook
	if err := bookRepo.Update(ctx, book); err != nil {
		t.Fatal(err)
	}

	downloadRoot = t.TempDir()
	mk := func(rel, content string) string {
		p := filepath.Join(downloadRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Both named "… - 015.mp3", different sizes, as in the report.
	first := mk(filepath.Join("Morning Star Part 3", "Morning Star - 015.mp3"), "part three track fifteen")
	second := mk(filepath.Join("Morning Star Part 1-2", "Morning Star - 015.mp3"), "part one two track fifteen, a different length")
	explicit = []string{first, second}
	return s, dl, dlRepo, downloadRoot, explicit, ctx
}

// TestPerFileImport_CollisionBlocksBeforeAnyWrite is the regression test for
// #2275. Two tracks flatten onto one destination path; the import must be
// blocked with the destination directory never created, both sources
// untouched, and no audiobook path recorded against the book.
func TestPerFileImport_CollisionBlocksBeforeAnyWrite(t *testing.T) {
	for _, mode := range []string{"hardlink", "copy", "move"} {
		t.Run(mode, func(t *testing.T) {
			s, dl, dlRepo, downloadRoot, explicit, ctx := perFileCollisionFixture(t, mode)

			s.tryImportInternal(ctx, dl, downloadRoot, "", "", "", nil, explicit)

			dest := importedAudiobookDir(t, s)
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				entries, _ := os.ReadDir(dest)
				names := make([]string, 0, len(entries))
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Fatalf("destination %q was created despite the collision (contains %v)", dest, names)
			}
			// UniqueDir would have produced "Title T () (2)" on a retry; the
			// point of preflighting is that no attempt gets that far.
			if _, err := os.Stat(dest + " (2)"); !os.IsNotExist(err) {
				t.Errorf("a second destination %q was created", dest+" (2)")
			}

			// Both sources survive in every mode, move included: nothing was
			// placed, so nothing was consumed.
			for _, src := range explicit {
				if _, err := os.Stat(src); err != nil {
					t.Errorf("source %q was consumed by a blocked import: %v", src, err)
				}
			}

			got, err := dlRepo.GetByID(ctx, dl.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != models.StateImportBlocked {
				t.Errorf("status = %q, want %q", got.Status, models.StateImportBlocked)
			}
			// The message must name both sources and the shared destination,
			// so the user can act on it without reading the code.
			for _, want := range []string{"Morning Star Part 3", "Morning Star Part 1-2", "same path"} {
				if !strings.Contains(got.ErrorMessage, want) {
					t.Errorf("error message %q does not mention %q", got.ErrorMessage, want)
				}
			}
		})
	}
}

// TestPerFileImport_DistinctNamesStillImport pins that the preflight does not
// disturb the ordinary per-file case the branch exists to serve (#903).
func TestPerFileImport_DistinctNamesStillImport(t *testing.T) {
	libraryDir := t.TempDir()
	s, dl, _, bookRepo, ctx := dataLossFixture(t, libraryDir, "hardlink")
	book, err := bookRepo.GetByID(ctx, *dl.BookID)
	if err != nil {
		t.Fatal(err)
	}
	book.MediaType = models.MediaTypeAudiobook
	if err := bookRepo.Update(ctx, book); err != nil {
		t.Fatal(err)
	}

	downloadRoot := t.TempDir()
	var explicit []string
	for rel, content := range map[string]string{
		filepath.Join("Part A", "track-01.mp3"): "a1",
		filepath.Join("Part B", "track-02.mp3"): "b2",
	} {
		p := filepath.Join(downloadRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		explicit = append(explicit, p)
	}

	s.tryImportInternal(ctx, dl, downloadRoot, "", "", "", nil, explicit)

	dest := importedAudiobookDir(t, s)
	for _, name := range []string{"track-01.mp3", "track-02.mp3"} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Errorf("%s was not placed: %v", name, err)
		}
	}
	got, err := bookRepo.GetByID(ctx, *dl.BookID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AudiobookFilePath != dest {
		t.Errorf("audiobook path = %q, want %q", got.AudiobookFilePath, dest)
	}
}

// TestPerFileImport_RollsBackAfterMidLoopFailure covers the second half of
// #2275: a placement error that the preflight cannot predict must not leave a
// half-filled destination folder behind. osLink is failed on the second file
// with an error the preflight has no view of.
func TestPerFileImport_RollsBackAfterMidLoopFailure(t *testing.T) {
	libraryDir := t.TempDir()
	s, dl, dlRepo, bookRepo, ctx := dataLossFixture(t, libraryDir, "hardlink")
	book, err := bookRepo.GetByID(ctx, *dl.BookID)
	if err != nil {
		t.Fatal(err)
	}
	book.MediaType = models.MediaTypeAudiobook
	if err := bookRepo.Update(ctx, book); err != nil {
		t.Fatal(err)
	}

	downloadRoot := t.TempDir()
	var explicit []string
	for _, rel := range []string{
		filepath.Join("Part A", "track-01.mp3"),
		filepath.Join("Part B", "track-02.mp3"),
	} {
		p := filepath.Join(downloadRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		explicit = append(explicit, p)
	}

	orig := osLink
	t.Cleanup(func() { osLink = orig })
	calls := 0
	osLink = func(src, dst string) error {
		calls++
		if calls == 2 {
			return &os.LinkError{Op: "link", Old: src, New: dst, Err: syscall.ENOSPC}
		}
		return orig(src, dst)
	}

	s.tryImportInternal(ctx, dl, downloadRoot, "", "", "", nil, explicit)

	dest := importedAudiobookDir(t, s)
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(dest)
		t.Fatalf("partial destination %q survived the failure with %d entries", dest, len(entries))
	}
	// Hardlink mode never consumes a source, so rollback must not have
	// touched the download.
	for _, src := range explicit {
		if _, err := os.Stat(src); err != nil {
			t.Errorf("rollback removed a source file %q: %v", src, err)
		}
	}
	got, err := dlRepo.GetByID(ctx, dl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.StateImportBlocked {
		t.Errorf("status = %q, want %q", got.Status, models.StateImportBlocked)
	}
}

// TestPerFileImport_MoveKeepsPlacedFilesAfterFailure pins the deliberate
// asymmetry: after a move the placed files are the only copy, so a mid-loop
// failure must NOT roll them back. This is the same rule the
// SetFormatFilePath failure path already applies.
func TestPerFileImport_MoveKeepsPlacedFilesAfterFailure(t *testing.T) {
	libraryDir := t.TempDir()
	s, dl, _, bookRepo, ctx := dataLossFixture(t, libraryDir, "move")
	book, err := bookRepo.GetByID(ctx, *dl.BookID)
	if err != nil {
		t.Fatal(err)
	}
	book.MediaType = models.MediaTypeAudiobook
	if err := bookRepo.Update(ctx, book); err != nil {
		t.Fatal(err)
	}

	downloadRoot := t.TempDir()
	var explicit []string
	for _, rel := range []string{
		filepath.Join("Part A", "track-01.mp3"),
		filepath.Join("Part B", "track-02.mp3"),
	} {
		p := filepath.Join(downloadRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		explicit = append(explicit, p)
	}

	origRename, origCopy := moveFileRename, moveFileCopy
	t.Cleanup(func() { moveFileRename, moveFileCopy = origRename, origCopy })
	calls := 0
	moveFileRename = func(src, dst string) error {
		calls++
		if calls == 2 {
			return &os.LinkError{Op: "rename", Old: src, New: dst, Err: syscall.ENOSPC}
		}
		return origRename(src, dst)
	}
	moveFileCopy = func(ctx context.Context, src, dst string) error {
		if calls == 2 {
			return syscall.ENOSPC
		}
		return origCopy(ctx, src, dst)
	}

	s.tryImportInternal(ctx, dl, downloadRoot, "", "", "", nil, explicit)

	dest := importedAudiobookDir(t, s)
	if _, err := os.Stat(filepath.Join(dest, "track-01.mp3")); err != nil {
		t.Errorf("move mode deleted the only copy of a placed file: %v", err)
	}
	if _, err := os.Stat(explicit[0]); !os.IsNotExist(err) {
		t.Errorf("expected the first source to have been consumed by the move, stat err = %v", err)
	}
}

// TestCheckPerFileCollisions covers the projection helper directly.
func TestCheckPerFileCollisions(t *testing.T) {
	t.Run("collision names both sources and the destination", func(t *testing.T) {
		err := checkPerFileCollisions("/lib/Author/Book", []string{
			"/dl/root/Part 3/015.mp3",
			"/dl/root/Part 1-2/015.mp3",
		})
		var collision *PerFileCollisionError
		if !errors.As(err, &collision) {
			t.Fatalf("want *PerFileCollisionError, got %v", err)
		}
		if collision.Dest != filepath.Join("/lib/Author/Book", "015.mp3") {
			t.Errorf("Dest = %q", collision.Dest)
		}
		if collision.FirstSrc != "/dl/root/Part 3/015.mp3" || collision.SecondSrc != "/dl/root/Part 1-2/015.mp3" {
			t.Errorf("sources = %q, %q", collision.FirstSrc, collision.SecondSrc)
		}
	})

	t.Run("distinct basenames pass", func(t *testing.T) {
		if err := checkPerFileCollisions("/lib/Author/Book", []string{
			"/dl/root/Part 1/015.mp3",
			"/dl/root/Part 2/016.mp3",
		}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("case-differing basenames pass", func(t *testing.T) {
		// Byte-exact by design: this predicts what the placement calls do,
		// and on a case-sensitive filesystem these two genuinely coexist.
		if err := checkPerFileCollisions("/lib/Author/Book", []string{
			"/dl/root/Track.mp3",
			"/dl/root/track.mp3",
		}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty and single file pass", func(t *testing.T) {
		if err := checkPerFileCollisions("/lib/Author/Book", nil); err != nil {
			t.Errorf("nil: %v", err)
		}
		if err := checkPerFileCollisions("/lib/Author/Book", []string{"/dl/a.mp3"}); err != nil {
			t.Errorf("single: %v", err)
		}
	})
}

// TestDropPlaceAudiobook_CollisionBlocksBeforeAnyWrite covers the sibling
// consumer of the same flatten (#2275). It matters more here than on the
// managed path: dropPlaceFile removes an existing destination before placing,
// so without the preflight the collision overwrites silently in both modes
// instead of failing on EEXIST.
func TestDropPlaceAudiobook_CollisionBlocksBeforeAnyWrite(t *testing.T) {
	for _, mode := range []string{"hardlink", "copy"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _, _ := scannerFixture(t, t.TempDir())

			downloadRoot := t.TempDir()
			var explicit []string
			for i, rel := range []string{
				filepath.Join("Part 3", "015.mp3"),
				filepath.Join("Part 1-2", "015.mp3"),
			} {
				p := filepath.Join(downloadRoot, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(strings.Repeat("x", i+1)), 0o644); err != nil {
					t.Fatal(err)
				}
				explicit = append(explicit, p)
			}

			destDir := filepath.Join(t.TempDir(), "Author", "Book")
			err := s.dropPlaceAudiobook(context.Background(), downloadRoot, explicit, explicit, destDir, mode)

			var collision *PerFileCollisionError
			if !errors.As(err, &collision) {
				t.Fatalf("want *PerFileCollisionError, got %v", err)
			}
			if _, statErr := os.Stat(destDir); !os.IsNotExist(statErr) {
				t.Errorf("drop destination %q was created despite the collision", destDir)
			}
		})
	}
}

// TestLinkErrorClassification pins that a hardlink failure is reported by its
// actual cause. Before #2275 every non-EXDEV error carried "download dir and
// library must be on the same filesystem", which sent the reporter to check
// mounts for what was really a destination collision on one ZFS dataset.
func TestLinkErrorClassification(t *testing.T) {
	const mountAdvice = "must be on the same filesystem"
	cases := []struct {
		name       string
		err        error
		want       string
		wantAdvice bool
	}{
		{"exists", fs.ErrExist, "already at the destination", false},
		{"missing source", fs.ErrNotExist, "source file is missing", false},
		{"permission", fs.ErrPermission, "check the permissions", false},
		{"unknown keeps the mount hint", syscall.EIO, mountAdvice, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := linkError("/dl/a.mp3", "/lib/a.mp3", tc.err).Error()
			if !strings.Contains(got, tc.want) {
				t.Errorf("error %q does not contain %q", got, tc.want)
			}
			if strings.Contains(got, mountAdvice) != tc.wantAdvice {
				t.Errorf("error %q: mount advice present = %v, want %v", got, !tc.wantAdvice, tc.wantAdvice)
			}
			if !strings.Contains(got, "/dl/a.mp3") || !strings.Contains(got, "/lib/a.mp3") {
				t.Errorf("error %q lost the paths", got)
			}
		})
	}
}

// TestHardlinkFileExistingDestination is the end-to-end form of the same
// misreport: a real os.Link EEXIST must not be described as a mount problem.
func TestHardlinkFileExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp3")
	dst := filepath.Join(dir, "dst.mp3")
	if err := os.WriteFile(src, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := HardlinkFile(src, dst)
	if err == nil {
		t.Fatal("expected an error linking onto an existing file")
	}
	if !errors.Is(err, fs.ErrExist) {
		t.Errorf("error does not unwrap to fs.ErrExist: %v", err)
	}
	if strings.Contains(err.Error(), "must be on the same filesystem") {
		t.Errorf("EEXIST still reported as a filesystem mismatch: %v", err)
	}
	// The pre-existing destination must be left exactly as it was.
	b, readErr := os.ReadFile(dst)
	if readErr != nil || string(b) != "b" {
		t.Errorf("destination was modified: %q, %v", b, readErr)
	}
}
