package abs

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/textutil"
)

// TestImporter_UpstreamAuthorLookupMemoizedPerRun proves the per-run author
// lookup cache: several books by the same author trigger the upstream provider
// search only once (one author's worth of queries), not once per book. Without
// the memo a library where that search is slow or times out (the OpenLibrary
// author-search deadlines in the field reports) re-pays the full timeout for
// every book by that author.
func TestImporter_UpstreamAuthorLookupMemoizedPerRun(t *testing.T) {
	importer, authorRepo, bookRepo, _, _, _, _, _, _, _ := newABSImporterFixture(t)
	// Provider returns no author match, so lookupUpstreamAuthor exhausts every
	// query form once per uncached invocation — a stable, countable baseline.
	stub := &stubABSMetadataProvider{}
	importer.WithMetadata(metadata.NewAggregator(stub))

	const authorName = "Qian Shan Cha Ke"
	items := make([]NormalizedLibraryItem, 3)
	for idx := range items {
		item := sampleABSItem()
		item.ItemID = "li-memo-" + string(rune('a'+idx))
		item.Title = "Volume " + string(rune('1'+idx))
		item.ASIN = ""
		item.Series = nil
		item.AudioFiles = nil
		item.EbookPath = ""
		item.EbookINO = ""
		item.Authors = []NormalizedAuthor{{ID: "author-qsck", Name: authorName}}
		items[idx] = item
	}

	importer.enumerateFn = func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
		for i := range items {
			if err := fn(ctx, items[i]); err != nil {
				return EnumerationStats{}, err
			}
		}
		return EnumerationStats{PagesScanned: 1, ItemsSeen: len(items), ItemsNormalized: len(items)}, nil
	}
	if _, err := importer.Run(context.Background(), ImportConfig{
		SourceID:  DefaultSourceID,
		BaseURL:   "https://abs.example.com",
		APIKey:    "secret",
		LibraryID: "lib-books",
		Label:     "Shelf",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	authors, err := authorRepo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("expected exactly 1 author created, got %d", len(authors))
	}
	if got := countBooksForAuthor(t, bookRepo, authors[0].ID); got != 3 {
		t.Fatalf("expected 3 books imported (enrichAuthor runs per item), got %d", got)
	}

	wantMax := len(textutil.AuthorSearchQueries(authorName))
	if wantMax == 0 {
		t.Fatal("precondition: author name produced no search queries")
	}
	if stub.searchAuthorsCalls == 0 {
		t.Fatal("vacuous: upstream author search never ran")
	}
	if stub.searchAuthorsCalls > wantMax {
		t.Fatalf("upstream author search ran %d times for 3 books by one author; memo should cap it at one author's worth (%d)", stub.searchAuthorsCalls, wantMax)
	}
}
