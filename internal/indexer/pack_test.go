package indexer

import "testing"

// TestMultiBookPackMarker_ReportedRelease pins the verbatim release name from
// #2276. An automatic search for the single book "Red Rising" selected it and
// the importer then tried to place all 117 files of a four-book pack into one
// book's folder.
func TestMultiBookPackMarker_ReportedRelease(t *testing.T) {
	const release = "Red Rising Series - Books 1 - 4 by Pierce Brown [ENG / M4B MP3] [VIP]"
	got := MultiBookPackMarker(release)
	if got == "" {
		t.Fatalf("release %q was not recognised as a multi-book pack", release)
	}
	if got != "Books 1 - 4" {
		t.Errorf("marker = %q, want %q", got, "Books 1 - 4")
	}
}

func TestMultiBookPackMarker_Packs(t *testing.T) {
	for _, title := range []string{
		"Pierce Brown - Red Rising Books 1-4 [M4B]",
		"Red Rising Book 1 to 4 - Pierce Brown",
		"Red Rising Books 1 through 5",
		"Brandon Sanderson - Mistborn Vols. 1 & 2",
		"Mistborn Volumes 1-3 (Unabridged)",
		"The Expanse Box Set",
		"The Expanse Boxed Set [EPUB]",
		"The Expanse Boxset",
		"Discworld 5 Book Set",
		"Discworld 3 Volume Collection",
		"Pierce Brown - Red Rising Omnibus",
		"The Sandman Complete Series",
		"Sherlock Holmes Complete Collection [MOBI]",
	} {
		if MultiBookPackMarker(title) == "" {
			t.Errorf("%q was not recognised as a multi-book pack", title)
		}
	}
}

// TestMultiBookPackMarker_SingleBooks is the half that matters: a false
// positive here silently stops a book being grabbed at all, with no setting to
// turn it back on. Each case records why that shape was left out.
func TestMultiBookPackMarker_SingleBooks(t *testing.T) {
	for _, tc := range []struct {
		title string
		why   string
	}{
		{"Red Rising - Pierce Brown [M4B]", "the plain single-book release"},
		{"Pierce Brown - Red Rising Series - Book 1", "a book number with no range"},
		{
			"Morning Star Book III of the Red Rising Trilogy (Unabridged) Part 1-2",
			"\"part\" ranges are how one long audiobook is split, and \"trilogy\" is how single books name their series — this is a real file name from the #2275 download",
		},
		{"The Complete Sherlock Holmes", "\"complete\" alone is a single-volume edition's own name"},
		{"Ursula K. Le Guin - The Collected Stories", "a bare \"collected\"/\"collection\" is a single book"},
		{"The Best American Short Stories Anthology 2020", "an anthology is one book"},
		{"Pierce Brown - Red Rising (2014-2016 reissue)", "a bare numeric range is far more often a year span"},
		{"Red Rising Book 1 - 2020 Edition", "an edition year must not read as a range of 2020 books"},
		{"Dune Volume 1", "a single volume with no range"},
		{"The Boxer - Pierce Brown", "\"box\" must not match inside another word"},
	} {
		if got := MultiBookPackMarker(tc.title); got != "" {
			t.Errorf("%q matched %q but is a single book (%s)", tc.title, got, tc.why)
		}
	}
}
