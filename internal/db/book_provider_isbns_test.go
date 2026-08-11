package db

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// TestBookRepo_ProviderISBNsAreNotPersisted pins the lifetime of
// models.Book.ProviderISBNs (#1893): metadata providers fill it in during a
// search, and the books table drops it on write. There is no isbns column, so
// any feature that reads a book's ISBN back out of the catalogue must go
// through the editions table (see indexer.CriteriaISBN, the #1724 near-miss).
//
// If this test starts failing because book ISBNs became persisted, that is a
// deliberate design change: hydrate the field everywhere books are loaded (not
// just the one query you touched), then rename it back to something that no
// longer claims to be provider-only, and update the field's doc comment.
func TestBookRepo_ProviderISBNsAreNotPersisted(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	ctx := context.Background()

	authors := NewAuthorRepo(database)
	books := NewBookRepo(database)

	a := &models.Author{
		ForeignID: "OL-HERBERT", Name: "Frank Herbert", SortName: "Herbert, Frank",
		MetadataProvider: "openlibrary", Monitored: true,
	}
	if err := authors.Create(ctx, a); err != nil {
		t.Fatalf("create author: %v", err)
	}

	b := &models.Book{
		ForeignID: "OL-DUNE", AuthorID: a.ID, Title: "Dune", SortTitle: "dune",
		Genres: []string{}, MetadataProvider: "openlibrary", Monitored: true,
		ProviderISBNs: []string{"9780441172719", "0441172717"},
	}
	if err := books.Create(ctx, b); err != nil {
		t.Fatalf("create book: %v", err)
	}

	got, err := books.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("get book: %v", err)
	}
	if got == nil {
		t.Fatal("book not found after create")
	}
	if len(got.ProviderISBNs) != 0 {
		t.Errorf("GetByID ProviderISBNs = %v, want empty: the field is transport-only and has no column (#1893)", got.ProviderISBNs)
	}

	batch, err := books.GetByIDs(ctx, []int64{b.ID})
	if err != nil {
		t.Fatalf("get books by ids: %v", err)
	}
	if loaded := batch[b.ID]; loaded == nil {
		t.Fatal("book missing from GetByIDs result")
	} else if len(loaded.ProviderISBNs) != 0 {
		t.Errorf("GetByIDs ProviderISBNs = %v, want empty: the field is transport-only and has no column (#1893)", loaded.ProviderISBNs)
	}
}
