package importer

import (
	"testing"
)

func TestTitleMatch(t *testing.T) {
	tests := []struct {
		bookTitle   string
		parsedTitle string
		want        bool
	}{
		// Standard matches — parsed titles use spaces, not dots
		{"The Name of the Wind", "The Name of the Wind", true},
		{"Project Hail Mary", "Project Hail Mary", true},
		{"The Way of Kings", "Brandon Sanderson The Way of Kings", true},

		// Partial overlap — at least 2 significant words required
		{"Dune Messiah", "Frank Herbert Dune Messiah", true},
		{"The Road", "Cormac McCarthy The Road 2006", true},

		// Single significant book-title word: minOverlap follows ptWords length
		// "Dune" → btWords=["dune"]; ptWords=["frank","herbert","dune"] len=3 → minOverlap=2
		// overlap=1 → false (single-word titles need a 1-word parsed title to match)
		{"Dune", "Frank Herbert Dune", false},
		// When parsed title is also short, minOverlap=1 and overlap=1 → true
		{"Dune", "Dune 2021", true},
		{"The Sparrow", "The Sparrow Russell", true},

		// Empty / degenerate cases
		{"", "The Name of the Wind", false},
		{"The Name of the Wind", "", false},
		// Dots in parsed title are not split — "project.hail.mary" becomes one big token
		{"Project Hail Mary", "Project.Hail.Mary", false},

		// Noise titles with no overlap
		{"Project Hail Mary", "The Lord of the Rings", false},
		{"Dune", "Foundation Asimov", false},
	}

	for _, tt := range tests {
		got := titleMatch(tt.bookTitle, tt.parsedTitle)
		if got != tt.want {
			t.Errorf("titleMatch(%q, %q) = %v, want %v", tt.bookTitle, tt.parsedTitle, got, tt.want)
		}
	}
}
