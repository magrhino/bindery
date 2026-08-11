package importer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveDir(t *testing.T) {
	src := t.TempDir()
	// Simulate an audiobook release folder with multi-part m4b + cover.
	mustWrite(t, filepath.Join(src, "Part.01.m4b"), "part1")
	mustWrite(t, filepath.Join(src, "Part.02.m4b"), "part2")
	mustWrite(t, filepath.Join(src, "nested", "cover.jpg"), "cover")

	dst := filepath.Join(t.TempDir(), "Author", "Title (2020)")
	if err := MoveDir(src, dst); err != nil {
		t.Fatal(err)
	}

	// Source should be gone, destination should hold everything preserved.
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source should have been removed, err = %v", err)
	}
	for _, name := range []string{"Part.01.m4b", "Part.02.m4b", "nested/cover.jpg"} {
		if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
			t.Errorf("missing %s in destination: %v", name, err)
		}
	}
}

func TestMoveDirRefusesExistingDst(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "x.m4b"), "x")
	dst := t.TempDir() // already exists
	if err := MoveDir(src, dst); err == nil {
		t.Error("expected error when dst already exists")
	}
}

// TestMoveDirRefusesDstInsideSrc is the regression test for #1809: a reorganize
// that turns a flat author folder into a per-book folder underneath it computes
// a destination inside the source. Rename cannot do that (EINVAL), so the
// copy-based slow path took over and recursed into the directory it had just
// created, copying the tree into itself until the disk (or the path length)
// gave out. Both the move and the copy entrypoint must refuse it up front.
func TestMoveDirRefusesDstInsideSrc(t *testing.T) {
	src := filepath.Join(t.TempDir(), "Jane Doe")
	mustWrite(t, filepath.Join(src, "part1.mp3"), "part1")
	dst := filepath.Join(src, "My Book (2020)")

	if err := MoveDir(src, dst); !errors.Is(err, ErrDestInsideSource) {
		t.Errorf("MoveDir err = %v, want ErrDestInsideSource", err)
	}
	if err := CopyDir(src, dst); !errors.Is(err, ErrDestInsideSource) {
		t.Errorf("CopyDir err = %v, want ErrDestInsideSource", err)
	}
	// Nothing copied, nothing created, source left exactly as it was.
	if _, err := os.Stat(filepath.Join(src, "part1.mp3")); err != nil {
		t.Errorf("source content must survive the refused move: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("destination should not have been created, stat err = %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
