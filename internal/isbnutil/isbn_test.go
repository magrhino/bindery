package isbnutil

import "testing"

func TestNormalize(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "isbn10 lowercase x check digit", raw: "3-453-30523-x", want: "345330523X"},
		{name: "isbn13 hyphen separators", raw: "978-0-307-47472-8", want: "9780307474728"},
		{name: "isbn13 space separators", raw: "978 0 307 47472 8", want: "9780307474728"},
		{name: "interior x preserved", raw: "978X0307474728", want: "978X0307474728"},
		{name: "early x preserved", raw: "97X80307474728", want: "97X80307474728"},
		{name: "multiple x preserved", raw: "978X030747472X", want: "978X030747472X"},
		{name: "invalid letters preserved", raw: "ISBN 9780307474728", want: "ISBN9780307474728"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.raw); got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestToISBN13(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "isbn13 passthrough", raw: "9780441172719", want: "9780441172719"},
		{name: "isbn13 hyphenated", raw: "978-0-441-17271-9", want: "9780441172719"},
		{name: "isbn13 979 prefix", raw: "979-8-6027-9535-5", want: "9798602795355"},
		{name: "isbn10 converts", raw: "0441172717", want: "9780441172719"},
		{name: "isbn10 hyphenated", raw: "0-441-17271-7", want: "9780441172719"},
		{name: "isbn10 x check digit", raw: "3-453-30523-x", want: "9783453305236"},
		{name: "empty", raw: "", want: ""},
		{name: "not an isbn", raw: "not-an-isbn", want: ""},
		{name: "wrong length", raw: "12345", want: ""},
		{name: "13 digits without bookland prefix", raw: "1234567890123", want: ""},
		{name: "interior letter", raw: "978X0307474728", want: ""},
		{name: "isbn10 interior x", raw: "04411X2717", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToISBN13(tt.raw); got != tt.want {
				t.Fatalf("ToISBN13(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
