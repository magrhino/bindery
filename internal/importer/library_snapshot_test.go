package importer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// TestLibrarySnapshot_WalksARootOnce pins the whole point of the snapshot
// (#1888/#1929): a root is walked on the first query and never again, so a
// batch of per-book lookups pays one walk instead of one per book.
//
// The pin is behavioural, not instrumented: a file created AFTER the first
// query must be invisible to the same snapshot but visible to a fresh one. If
// the caching is removed the snapshot re-walks and finds the late file, and
// this test fails — which is exactly the per-book-walk behaviour being fixed.
func TestLibrarySnapshot_WalksARootOnce(t *testing.T) {
	libDir := t.TempDir()
	first := filepath.Join(libDir, "Jane Doe", "First Book - Jane Doe.epub")
	writeFile(t, first)

	snap := NewLibrarySnapshot(libDir, "")
	if got := snap.FindExisting(context.Background(), "First Book", "Jane Doe", models.MediaTypeEbook); got != first {
		t.Fatalf("first query: got %q, want %q", got, first)
	}

	late := filepath.Join(libDir, "Jane Doe", "Late Book - Jane Doe.epub")
	writeFile(t, late)

	if got := snap.FindExisting(context.Background(), "Late Book", "Jane Doe", models.MediaTypeEbook); got != "" {
		t.Errorf("snapshot saw a file created after its walk (%q) — the root was walked again", got)
	}
	if got := NewLibrarySnapshot(libDir, "").FindExisting(context.Background(), "Late Book", "Jane Doe", models.MediaTypeEbook); got != late {
		t.Errorf("fresh snapshot: got %q, want %q — the late file is genuinely findable", got, late)
	}
}

// TestLibrarySnapshot_MatchesWalkSemantics holds the snapshot to the exact
// predicate of the old per-query walk: the author-folder pre-filter rejects a
// same-titled file under another author's folder, files directly under the
// root are exempt from the pre-filter (their filename author still counts),
// and a title mismatch never matches.
func TestLibrarySnapshot_MatchesWalkSemantics(t *testing.T) {
	libDir := t.TempDir()
	// Same title under the WRONG author folder, with NO author in the filename:
	// the parsed author is empty, which authorMatch treats as unfalsifiable, so
	// the folder pre-filter is the ONLY thing standing between this file and a
	// false match for Matt Dinniman (the David Wong case the old walk's comment
	// names). A filename that carries the wrong author is caught later by the
	// filename-level check and would mask a pre-filter regression here.
	wrongAuthor := filepath.Join(libDir, "David Wong", "Dungeon Crawler Carl.epub")
	writeFile(t, wrongAuthor)
	// The real book, directly under the root — depth 1, so no folder to
	// pre-filter on; the parsed filename author carries the match.
	rootLevel := filepath.Join(libDir, "Dungeon Crawler Carl - Matt Dinniman.epub")
	writeFile(t, rootLevel)

	snap := NewLibrarySnapshot(libDir, "")
	if got := snap.FindExisting(context.Background(), "Dungeon Crawler Carl", "Matt Dinniman", models.MediaTypeEbook); got != rootLevel {
		t.Errorf("got %q, want %q — the author pre-filter or the root-level exemption drifted from the walk's", got, rootLevel)
	}
	if got := snap.FindExisting(context.Background(), "Some Other Title", "Matt Dinniman", models.MediaTypeEbook); got != "" {
		t.Errorf("title mismatch matched %q", got)
	}
}

// TestLibrarySnapshot_MediaTypeRootSelection carries the #488 root-selection
// contract over to the snapshot: ebook → library root, audiobook → audiobook
// root, audiobook falls back to the library root when no audiobook root is
// configured.
func TestLibrarySnapshot_MediaTypeRootSelection(t *testing.T) {
	libDir := t.TempDir()
	abDir := t.TempDir()
	ebookPath := filepath.Join(libDir, "Jane Doe", "Title - Jane Doe.epub")
	audioPath := filepath.Join(abDir, "Jane Doe", "Title - Jane Doe.m4b")
	writeFile(t, ebookPath)
	writeFile(t, audioPath)

	snap := NewLibrarySnapshot(libDir, abDir)
	if got := snap.FindExisting(context.Background(), "Title", "Jane Doe", models.MediaTypeEbook); got != ebookPath {
		t.Errorf("ebook: got %q, want %q", got, ebookPath)
	}
	if got := snap.FindExisting(context.Background(), "Title", "Jane Doe", models.MediaTypeAudiobook); got != audioPath {
		t.Errorf("audiobook: got %q, want %q", got, audioPath)
	}

	// No audiobook root configured: audiobook queries fall back to the library
	// root instead of finding nothing.
	fallback := NewLibrarySnapshot(libDir, "")
	audioInLib := filepath.Join(libDir, "Jane Doe", "Spoken - Jane Doe.m4b")
	writeFile(t, audioInLib)
	if got := fallback.FindExisting(context.Background(), "Spoken", "Jane Doe", models.MediaTypeAudiobook); got != audioInLib {
		t.Errorf("audiobook fallback: got %q, want %q", got, audioInLib)
	}
}

// TestLibrarySnapshot_CancelledContextIsNotCached covers the two halves of the
// cancellation contract. The old walk ignored its ctx entirely (#1929), so a
// cancelled sync kept paying the full walk; the snapshot must abandon the walk
// — and must NOT cache the truncated result, or one cancelled sync would blind
// every later query to the whole library.
func TestLibrarySnapshot_CancelledContextIsNotCached(t *testing.T) {
	libDir := t.TempDir()
	path := filepath.Join(libDir, "Jane Doe", "Title - Jane Doe.epub")
	writeFile(t, path)

	snap := NewLibrarySnapshot(libDir, "")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if got := snap.FindExisting(cancelled, "Title", "Jane Doe", models.MediaTypeEbook); got != "" {
		t.Errorf("cancelled query returned %q, want nothing", got)
	}
	// The same snapshot with a live context must do a real walk and find it.
	if got := snap.FindExisting(context.Background(), "Title", "Jane Doe", models.MediaTypeEbook); got != path {
		t.Errorf("post-cancel query: got %q, want %q — the abandoned walk was cached", got, path)
	}
}

// TestLibrarySnapshot_SupplementBesideAudioNeverBinds is the regression test
// for #2240, from ianepreston's second repro: a folder holding an audiobook
// release plus its cue sheet. bookExtensions includes .txt, so the walk
// collected the cue as a candidate book file, and titleMatch's
// two-significant-token bar let "Dogs of War.cue" clear both "Dogs of War" AND
// "Bear Head: Dogs of War, Book 2" — one supplement file bound to two
// different books in a single author-sync pass, each recorded as owned with
// its on-add search suppressed.
//
// The audio-folder guard from #2188 rejects exactly this shape in the scan
// loop; this pins that the snapshot walk applies it too: a supplement-class
// file whose folder also holds audio is the audiobook's material and must
// never be a FindExisting answer. The chapter-named audio files themselves
// match neither title, so both queries must come back empty.
func TestLibrarySnapshot_SupplementBesideAudioNeverBinds(t *testing.T) {
	abDir := t.TempDir()
	bookDir := filepath.Join(abDir, "Adrian Tchaikovsky", "Dogs of War (2025)")
	cue := filepath.Join(bookDir, "Dogs of War.cue.txt")
	writeFile(t, cue)
	writeFile(t, filepath.Join(bookDir, "Track 01.mp3"))
	writeFile(t, filepath.Join(bookDir, "Track 02.mp3"))

	snap := NewLibrarySnapshot("", abDir)
	for _, title := range []string{"Dogs of War", "Bear Head: Dogs of War, Book 2"} {
		got := snap.FindExisting(context.Background(), title, "Adrian Tchaikovsky", models.MediaTypeAudiobook)
		if got == cue {
			t.Errorf("FindExisting(%q) bound the cue sheet %q — the audio-folder guard is not reaching the snapshot walk", title, got)
		} else if got != "" {
			t.Errorf("FindExisting(%q) = %q, want no match", title, got)
		}
	}
}

// TestLibrarySnapshot_ContainerOutranksSupplement carries #2188's claim
// ranking into FindExisting: when a real ebook container and a supplement-class
// file both match the same book, the container must win regardless of which
// one the directory listing yields first ("(notes)" sorts before "- Author"
// here, so walk order alone would return the .txt).
func TestLibrarySnapshot_ContainerOutranksSupplement(t *testing.T) {
	libDir := t.TempDir()
	dir := filepath.Join(libDir, "William Gibson")
	notes := filepath.Join(dir, "Burning Chrome (notes).txt")
	epub := filepath.Join(dir, "Burning Chrome - William Gibson.epub")
	writeFile(t, notes)
	writeFile(t, epub)

	snap := NewLibrarySnapshot(libDir, "")
	if got := snap.FindExisting(context.Background(), "Burning Chrome", "William Gibson", models.MediaTypeEbook); got != epub {
		t.Errorf("got %q, want the real container %q — supplement ranking is not applied", got, epub)
	}
}

// TestLibrarySnapshot_TextOnlyLibraryStillMatches pins #2188's "ranking rather
// than excluding" property on the snapshot path: a supplement-class file with
// no audio beside it and no container competing for the book is a legitimate
// ebook and must still be found.
func TestLibrarySnapshot_TextOnlyLibraryStillMatches(t *testing.T) {
	libDir := t.TempDir()
	txt := filepath.Join(libDir, "Jane Doe", "Plain Book - Jane Doe.txt")
	writeFile(t, txt)

	snap := NewLibrarySnapshot(libDir, "")
	if got := snap.FindExisting(context.Background(), "Plain Book", "Jane Doe", models.MediaTypeEbook); got != txt {
		t.Errorf("got %q, want %q — a text-only library must keep matching", got, txt)
	}
}
