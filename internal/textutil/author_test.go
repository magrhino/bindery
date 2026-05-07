package textutil

import (
	"reflect"
	"testing"
)

func TestNormalizeAuthorName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"R.R. Haywood", "r r haywood"},
		{"  John   Smith  ", "john smith"},
		{"", ""},
		{"Jean-Luc Picard", "jean luc picard"},
	}
	for _, tc := range cases {
		if got := NormalizeAuthorName(tc.in); got != tc.want {
			t.Fatalf("NormalizeAuthorName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeAuthorNameWithVariants(t *testing.T) {
	cases := []struct {
		in   string
		want []string // must be subset-match (all listed strings present)
	}{
		{in: "R.R. Haywood", want: []string{"r r haywood", "rr haywood", "haywood r r"}},
		{in: "Haywood, R.R.", want: []string{"haywood r r", "r r haywood", "rr haywood"}},
		{in: "John Smith Jr.", want: []string{"john smith", "smith john"}},
		{in: "Andy Weir", want: []string{"andy weir", "weir andy"}},
	}
	for _, tc := range cases {
		got := NormalizeAuthorNameWithVariants(tc.in)
		have := make(map[string]bool, len(got))
		for _, v := range got {
			have[v] = true
		}
		for _, want := range tc.want {
			if !have[want] {
				t.Fatalf("variants(%q) = %v, missing %q", tc.in, got, want)
			}
		}
	}
}

func TestNormalizeAuthorNameWithVariants_Idempotent(t *testing.T) {
	a := NormalizeAuthorNameWithVariants("R.R. Haywood")
	b := NormalizeAuthorNameWithVariants("R.R. Haywood")
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("variants should be deterministic: %v vs %v", a, b)
	}
}

func TestMatchAuthorName(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		kind AuthorMatchKind
	}{
		{"identical", "R.R. Haywood", "r r haywood", AuthorMatchExact},
		{"compact initials", "R.R. Haywood", "RR Haywood", AuthorMatchExact},
		{"spaced initials", "R.R. Haywood", "R R Haywood", AuthorMatchExact},
		{"suffix jr", "John Smith Jr.", "John Smith", AuthorMatchExact},
		{"suffix iii", "Henry VIII III", "Henry VIII", AuthorMatchExact},
		{"diacritics", "Gabriel García Márquez", "Gabriel Garcia Marquez", AuthorMatchExact},
		{"last first swap", "Haywood, R.R.", "R.R. Haywood", AuthorMatchExact},
		{"last first comma", "Weir, Andy", "Andy Weir", AuthorMatchExact},
		{"fuzzy auto", "Brandon Sanderson", "Brandon Sandersen", AuthorMatchFuzzyAuto},
		{"fuzzy ambiguous", "Alice Jones", "Alice James", AuthorMatchFuzzyAmbiguous},
		{"none", "Jane Doe", "Neal Stephenson", AuthorMatchNone},
		{"empty", "", "Jane Doe", AuthorMatchNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchAuthorName(tc.a, tc.b)
			if got.Kind != tc.kind {
				t.Fatalf("MatchAuthorName(%q,%q) kind=%d score=%.3f, want kind=%d", tc.a, tc.b, got.Kind, got.Score, tc.kind)
			}
			switch tc.kind {
			case AuthorMatchFuzzyAuto:
				if got.Score < AuthorMatchAutoThreshold {
					t.Fatalf("expected score >= %.2f, got %.3f", AuthorMatchAutoThreshold, got.Score)
				}
			case AuthorMatchFuzzyAmbiguous:
				if got.Score < AuthorMatchAmbiguousMinimum || got.Score >= AuthorMatchAutoThreshold {
					t.Fatalf("expected score in [%.2f,%.2f), got %.3f", AuthorMatchAmbiguousMinimum, AuthorMatchAutoThreshold, got.Score)
				}
			}
		})
	}
}

func TestMatchAuthorName_Symmetric(t *testing.T) {
	pairs := [][2]string{
		{"R.R. Haywood", "RR Haywood"},
		{"Brandon Sanderson", "Brandon Sandersen"},
		{"Weir, Andy", "Andy Weir"},
	}
	for _, pair := range pairs {
		fwd := MatchAuthorName(pair[0], pair[1])
		rev := MatchAuthorName(pair[1], pair[0])
		if fwd.Kind != rev.Kind {
			t.Fatalf("asymmetric match for %v: fwd=%d rev=%d", pair, fwd.Kind, rev.Kind)
		}
	}
}
