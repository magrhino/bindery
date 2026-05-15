package calibre

import (
	"reflect"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestNormalizeLanguageForCalibre(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"eng", "en"},
		{"fre", "fr"},
		{"fra", "fr"},
		{"ger", "de"},
		{"deu", "de"},
		{"EN", "en"},
		{"  spa  ", "es"},
		{"cat", "cat"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeLanguageForCalibre(tt.in); got != tt.want {
			t.Errorf("NormalizeLanguageForCalibre(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatCalibredbPubdate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"date only", "1965-08-01", "1965-08-01T00:00:00+00:00"},
		{"timestamp", "1965-08-01T00:00:00+00:00", "1965-08-01T00:00:00+00:00"},
		{"blank", "   ", ""},
		{"unexpected", "August 1965", "August 1965"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCalibredbPubdate(tt.in); got != tt.want {
				t.Fatalf("formatCalibredbPubdate(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIdentifiersForBook_UsesPresentBookAndEditionData(t *testing.T) {
	editionASIN := "B000FC1BN8"
	edition := &models.Edition{
		ForeignID: "/books/OL999M",
		ISBN13:    strPtr("9780441172719"),
		ISBN10:    strPtr("0441172717"),
		ASIN:      &editionASIN,
	}
	book := &models.Book{
		ID:               42,
		ForeignID:        "/works/OL123W",
		MetadataProvider: "openlibrary",
		ASIN:             "BOOKASIN",
	}

	got := IdentifiersForBook(book, edition)
	want := map[string]string{
		"asin":                "B000FC1BN8",
		"bindery":             "42",
		"isbn":                "9780441172719",
		"openlibrary":         "OL123W",
		"openlibrary_edition": "OL999M",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IdentifiersForBook = %#v, want %#v", got, want)
	}
}

func TestIdentifiersForBook_NormalizesProviderIdentifiers(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		foreignID string
		wantType  string
		wantValue string
	}{
		{"hardcover", "hardcover", "hc:dune", "hardcover", "dune"},
		{"googlebooks", "googlebooks", "gb:zyTCAlFPjgYC", "google", "zyTCAlFPjgYC"},
		{"google", "google", "gb:zyTCAlFPjgYC", "google", "zyTCAlFPjgYC"},
		{"dnb", "dnb", "dnb:123456789", "dnb", "123456789"},
		{"fallback", "Other.Provider", " provider:value ", "other_provider", "provider:value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IdentifiersForBook(&models.Book{
				ID:               7,
				ForeignID:        tt.foreignID,
				MetadataProvider: tt.provider,
			}, nil)
			if got[tt.wantType] != tt.wantValue {
				t.Fatalf("identifier %q = %q, want %q in %#v", tt.wantType, got[tt.wantType], tt.wantValue, got)
			}
			if got["bindery"] != "7" {
				t.Fatalf("bindery identifier = %q, want 7", got["bindery"])
			}
		})
	}
}

func TestIdentifierArgs_CleansAndSortsIdentifiers(t *testing.T) {
	got := identifierArgs(map[string]string{
		"Google Books": " gb:abc,123 ",
		"isbn":         " 9780441172719 ",
		"empty":        "",
	})
	want := []string{"google_books:gb:abc 123", "isbn:9780441172719"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identifierArgs = %#v, want %#v", got, want)
	}
}

func strPtr(s string) *string { return &s }
