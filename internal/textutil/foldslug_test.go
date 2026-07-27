package textutil

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestFoldForSlug_LatinAccents(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Ödland", "odland"},
		{"Ådland", "adland"},
		{"Königsmörder-Chronik", "konigsmorder-chronik"},
		{"La Comédie humaine", "la comedie humaine"},
		{"Straße", "strasse"},
		{"Søren Kierkegaard", "soren kierkegaard"},
		{"Stanisław Lem", "stanislaw lem"},
		{"Æsop", "aesop"},
		{"Łódź", "lodz"},
	} {
		if got := FoldForSlug(c.in); got != c.want {
			t.Errorf("FoldForSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Outside Latin and Greek a combining mark is part of the letter. Folding it
// merges distinct works — the collapse #1645 exists to prevent.
func TestFoldForSlug_NonLatinMarksPreserved(t *testing.T) {
	for _, p := range [][2]string{
		{"ハード", "ハート"},
		{"ドラゴン", "トラゴン"},
		{"ゲーム", "ケーム"},
		{"日本語シリーズ", "日本語シリース"},
		{"कमला", "कमल"},
		{"Толстой", "Толстои"},
	} {
		if a, b := FoldForSlug(p[0]), FoldForSlug(p[1]); a == b {
			t.Errorf("FoldForSlug collapsed distinct strings %q and %q onto %q", p[0], p[1], a)
		}
	}
}

// Greek drops the tonos in all-caps, and ToLower leaves an existing ς alone.
func TestFoldForSlug_GreekCaseVariants(t *testing.T) {
	for _, p := range [][2]string{
		{"Ἰλιάς", "ἸΛΙΆΣ"},
		{"Οδύσσεια", "ΟΔΎΣΣΕΙΑ"},
		{"Νίκος", "ΝΙΚΟΣ"},
	} {
		if a, b := FoldForSlug(p[0]), FoldForSlug(p[1]); a != b {
			t.Errorf("case variants disagree: %q->%q vs %q->%q", p[0], a, p[1], b)
		}
	}
}

func TestFoldForSlug_UnicodeFormInvariant(t *testing.T) {
	for _, s := range []string{"Les Misérables", "Ödland", "Åsa Larsson", "ハード", "कमला", "Ἰλιάς"} {
		if nfc, nfd := FoldForSlug(norm.NFC.String(s)), FoldForSlug(norm.NFD.String(s)); nfc != nfd {
			t.Errorf("NFC/NFD disagree for %q: %q vs %q", s, nfc, nfd)
		}
	}
}

func TestFoldForSlug_Idempotent(t *testing.T) {
	for _, s := range []string{"Ödland", "Straße", "三体", "日本語シリーズ", "कमला", "Ἰλιάς", "", "   "} {
		if once := FoldForSlug(s); FoldForSlug(once) != once {
			t.Errorf("not idempotent for %q: %q -> %q", s, once, FoldForSlug(once))
		}
	}
}
