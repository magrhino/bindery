package openlibrary

import "testing"

// TestSeriesSlug_ASCIIUnchanged is the compatibility guard. series.foreign_id is
// persisted and UNIQUE, and there is no migration or startup backfill for it, so
// any slug whose value changes re-keys on upgrade and leaves the old row behind
// holding the existing book links. Every ASCII series in every existing install
// must therefore keep its exact current key.
func TestSeriesSlug_ASCIIUnchanged(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Dune Chronicles", "dune-chronicles"},
		{"The Wheel of Time", "the-wheel-of-time"},
		{"A Song of Ice & Fire", "a-song-of-ice-fire"},
		{"Foundation 2", "foundation-2"},
		{"Sword of Truth -- Book 3", "sword-of-truth-book-3"},
		{"Hitchhiker's Guide", "hitchhiker-s-guide"},
		{"Star Wars: Legends", "star-wars-legends"},
		{"X-Men", "x-men"},
		{"21 Lessons", "21-lessons"},
		{"  Leading and trailing  ", "leading-and-trailing"},
	} {
		if got := seriesSlug(c.in); got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q (ASCII keys must not change)", c.in, got, c.want)
		}
	}
}

// Distinct series must not share an identity — the whole point of #1645. The
// first fix folded every combining mark, which reintroduced the same collapse
// for scripts where marks are letters.
func TestSeriesSlug_DistinctSeriesStayDistinct(t *testing.T) {
	for _, p := range [][2]string{
		{"ハード", "ハート"},
		{"ドラゴン", "トラゴン"},
		{"日本語シリーズ", "日本語シリース"},
		{"Ödland", "Ådland"},
		{"Ökotopia", "Kotopia"},
		{"कमला", "कमल"},
		{"三体", "日本語シリーズ"},
		{"Разбитая жизнь", "Другая жизнь"},
	} {
		a, b := seriesSlug(p[0]), seriesSlug(p[1])
		if a == b {
			t.Errorf("seriesSlug collapsed distinct series %q and %q onto %q", p[0], p[1], a)
		}
		if a == "" || b == "" {
			t.Errorf("seriesSlug produced an empty identity for %q/%q (%q/%q)", p[0], p[1], a, b)
		}
	}
}

// Non-Latin marks are word characters, not separators. Treating them as
// separators shatters one title into fragments.
func TestSeriesSlug_NonLatinMarksAreNotSeparators(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"गोदान", "गोदान"},
		{"三体", "三体"},
		{"日本語シリーズ", "日本語シリーズ"},
	} {
		if got := seriesSlug(c.in); got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Non-decomposable Latin now folds, so a series does not split on spelling.
func TestSeriesSlug_NonDecomposableLatinFolds(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"Straße-Serie", "Strasse-Serie"},
		{"Ø-serien", "O-serien"},
		{"Łódź Files", "Lodz Files"},
		{"Æsop Fables", "Aesop Fables"},
	} {
		if x, y := seriesSlug(c.a), seriesSlug(c.b); x != y {
			t.Errorf("spelling variants of one series disagree: %q->%q vs %q->%q", c.a, x, c.b, y)
		}
	}
}

// A title with no letters or digits still yields no identity, so parseSeriesRef
// keeps reporting ok=false and callers keep dropping the ref (#1645).
func TestSeriesSlug_NoUsableIdentity(t *testing.T) {
	for _, in := range []string{"", "   ", "!!!", "---", "..."} {
		if got := seriesSlug(in); got != "" {
			t.Errorf("seriesSlug(%q) = %q, want empty", in, got)
		}
		if _, ok := parseSeriesRef(in); ok {
			t.Errorf("parseSeriesRef(%q) reported ok=true; caller would emit a prefix-only ForeignID", in)
		}
	}
}
