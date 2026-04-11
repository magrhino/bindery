package openlibrary

import (
	"context"
	"os"
	"testing"
)

func skipIfShort(t *testing.T) {
	if os.Getenv("BINDERY_INTEGRATION") == "" {
		t.Skip("skipping integration test; set BINDERY_INTEGRATION=1 to run")
	}
}

func TestSearchAuthors_Integration(t *testing.T) {
	skipIfShort(t)
	c := New()
	authors, err := c.SearchAuthors(context.Background(), "Stephen King")
	if err != nil {
		t.Fatalf("search authors: %v", err)
	}
	if len(authors) == 0 {
		t.Fatal("expected at least one author result")
	}
	found := false
	for _, a := range authors {
		if a.Name == "Stephen King" {
			found = true
			if a.ForeignID == "" {
				t.Error("expected non-empty foreign ID for Stephen King")
			}
		}
	}
	if !found {
		t.Error("expected 'Stephen King' in results")
	}
}

func TestSearchBooks_Integration(t *testing.T) {
	skipIfShort(t)
	c := New()
	books, err := c.SearchBooks(context.Background(), "Dark Matter")
	if err != nil {
		t.Fatalf("search books: %v", err)
	}
	if len(books) == 0 {
		t.Fatal("expected at least one book result")
	}
	t.Logf("found %d books for 'Dark Matter'", len(books))
	t.Logf("first: %s (ID: %s)", books[0].Title, books[0].ForeignID)
}

func TestGetBook_Integration(t *testing.T) {
	skipIfShort(t)
	c := New()
	// OL20617889W is "The Shining" by Stephen King
	book, err := c.GetBook(context.Background(), "OL20617889W")
	if err != nil {
		t.Fatalf("get book: %v", err)
	}
	if book.Title == "" {
		t.Error("expected non-empty title")
	}
	t.Logf("book: %s, description length: %d", book.Title, len(book.Description))
}

func TestGetEditions_Integration(t *testing.T) {
	skipIfShort(t)
	c := New()
	editions, err := c.GetEditions(context.Background(), "OL20617889W")
	if err != nil {
		t.Fatalf("get editions: %v", err)
	}
	if len(editions) == 0 {
		t.Fatal("expected at least one edition")
	}
	t.Logf("found %d editions", len(editions))
	for i, e := range editions {
		if i >= 3 {
			break
		}
		t.Logf("  edition: %s (ISBN13: %v, format: %s)", e.Title, e.ISBN13, e.Format)
	}
}

func TestSortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Stephen King", "King, Stephen"},
		{"J.R.R. Tolkien", "Tolkien, J.R.R."},
		{"Madonna", "Madonna"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sortName(tt.input)
		if got != tt.want {
			t.Errorf("sortName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		input interface{}
		want  string
	}{
		{"plain string", "plain string"},
		{map[string]interface{}{"type": "/type/text", "value": "rich text"}, "rich text"},
		{nil, ""},
		{42, ""},
	}
	for _, tt := range tests {
		got := extractText(tt.input)
		if got != tt.want {
			t.Errorf("extractText(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
