package db

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// The live specimen: a book declared 'audiobook' that also holds an epub in
// book_files. Before the widening, media_type stayed 'audiobook', so the UI
// badged the epub as an audiobook, the formatless download served the epub,
// and the delete dialog named one path while the handler removed both.
func TestRefreshBookStatus_WidensAudiobookBookHoldingAnEbook(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	book.MediaType = models.MediaTypeAudiobook
	if err := repo.Update(ctx, book); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeAudiobook, "/ab/redshirts"); err != nil {
		t.Fatalf("AddBookFile audiobook: %v", err)
	}
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, "/lib/redshirts.epub"); err != nil {
		t.Fatalf("AddBookFile ebook: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.MediaType != models.MediaTypeBoth {
		t.Errorf("MediaType = %q, want %q — a registered file must widen the declared intent",
			got.MediaType, models.MediaTypeBoth)
	}
	if got.EbookFilePath != "/lib/redshirts.epub" {
		t.Errorf("EbookFilePath = %q, want /lib/redshirts.epub", got.EbookFilePath)
	}
	if got.AudiobookFilePath != "/ab/redshirts" {
		t.Errorf("AudiobookFilePath = %q, want /ab/redshirts", got.AudiobookFilePath)
	}
	if got.Status != models.BookStatusImported {
		t.Errorf("Status = %q, want imported — both formats are on disk", got.Status)
	}
}

// The mirror case: an 'ebook' book that acquires an audiobook file.
func TestRefreshBookStatus_WidensEbookBookHoldingAnAudiobook(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	// openTestDB seeds MediaTypeEbook.
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, "/lib/x.epub"); err != nil {
		t.Fatalf("AddBookFile ebook: %v", err)
	}
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeAudiobook, "/ab/x"); err != nil {
		t.Fatalf("AddBookFile audiobook: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.MediaType != models.MediaTypeBoth {
		t.Errorf("MediaType = %q, want %q", got.MediaType, models.MediaTypeBoth)
	}
	if got.Status != models.BookStatusImported {
		t.Errorf("Status = %q, want imported", got.Status)
	}
}

// Widening is driven by inventory, so a book holding one format keeps the
// media type it was given — including the 'both' book that is still waiting
// on its second file, which must not be narrowed.
func TestRefreshBookStatus_SingleFormatDoesNotWiden(t *testing.T) {
	cases := []struct {
		name      string
		mediaType string
		format    string
		path      string
	}{
		{"ebook book, ebook file", models.MediaTypeEbook, models.MediaTypeEbook, "/lib/only.epub"},
		{"audiobook book, audiobook file", models.MediaTypeAudiobook, models.MediaTypeAudiobook, "/ab/only"},
		{"audiobook book, stray ebook file", models.MediaTypeAudiobook, models.MediaTypeEbook, "/lib/stray.epub"},
		{"both book, one file only", models.MediaTypeBoth, models.MediaTypeEbook, "/lib/half.epub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, _, book := openTestDB(t)
			ctx := context.Background()
			repo := NewBookRepo(database)

			book.MediaType = tc.mediaType
			if err := repo.Update(ctx, book); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if err := repo.AddBookFile(ctx, book.ID, tc.format, tc.path); err != nil {
				t.Fatalf("AddBookFile: %v", err)
			}

			got, err := repo.GetByID(ctx, book.ID)
			if err != nil || got == nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.MediaType != tc.mediaType {
				t.Errorf("MediaType = %q, want %q — one format on disk must not change the media type",
					got.MediaType, tc.mediaType)
			}
		})
	}
}

// Removing one format's file leaves the widened media type in place: the
// widening records that the book turned out to be dual-format, and narrowing
// it again on delete would silently re-hide whatever is still registered.
func TestRefreshBookStatus_WideningSurvivesFileRemoval(t *testing.T) {
	database, _, book := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeEbook, "/lib/y.epub"); err != nil {
		t.Fatalf("AddBookFile ebook: %v", err)
	}
	if err := repo.AddBookFile(ctx, book.ID, models.MediaTypeAudiobook, "/ab/y"); err != nil {
		t.Fatalf("AddBookFile audiobook: %v", err)
	}
	if _, err := repo.RemoveBookFile(ctx, "/ab/y"); err != nil {
		t.Fatalf("RemoveBookFile: %v", err)
	}

	got, err := repo.GetByID(ctx, book.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.MediaType != models.MediaTypeBoth {
		t.Errorf("MediaType = %q, want %q", got.MediaType, models.MediaTypeBoth)
	}
	// The audiobook is now missing, so the book owes a file again.
	if got.Status != models.BookStatusWanted {
		t.Errorf("Status = %q, want wanted — the audiobook is gone", got.Status)
	}
}

func TestLegacyFilePathFor(t *testing.T) {
	const (
		ebook = "/lib/a.epub"
		audio = "/ab/a"
	)
	cases := []struct {
		name      string
		mediaType string
		ebookPath string
		audioPath string
		want      string
	}{
		// The arm refreshBookStatus can no longer reach: an audiobook book
		// holding both paths must not advertise the ebook as its file_path.
		{"audiobook book prefers its audiobook", models.MediaTypeAudiobook, ebook, audio, audio},
		{"audiobook book with only an ebook falls back", models.MediaTypeAudiobook, ebook, "", ebook},
		{"audiobook book with only an audiobook", models.MediaTypeAudiobook, "", audio, audio},
		{"ebook book prefers the ebook", models.MediaTypeEbook, ebook, audio, ebook},
		{"both book is ebook-first", models.MediaTypeBoth, ebook, audio, ebook},
		{"both book with only an audiobook", models.MediaTypeBoth, "", audio, audio},
		{"no files clears the column", models.MediaTypeBoth, "", "", ""},
		{"empty media type is ebook-first", "", ebook, audio, ebook},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := legacyFilePathFor(tc.mediaType, tc.ebookPath, tc.audioPath); got != tc.want {
				t.Errorf("legacyFilePathFor(%q, %q, %q) = %q, want %q",
					tc.mediaType, tc.ebookPath, tc.audioPath, got, tc.want)
			}
		})
	}
}

// The 'both' list filter must isolate dual-format books. The ebook and
// audiobook filters deliberately include them ("everything with an ebook"),
// so before this case existed ?mediaType=both fell through the switch and
// returned the entire library.
func TestListBooks_MediaTypeBothFilter(t *testing.T) {
	database, author, ebookBook := openTestDB(t)
	ctx := context.Background()
	repo := NewBookRepo(database)

	dual := &models.Book{
		ForeignID: "OL-DUAL", AuthorID: author.ID, Title: "Dual", SortTitle: "Dual",
		Monitored: true, Status: models.BookStatusWanted, MediaType: models.MediaTypeBoth,
	}
	if err := repo.Create(ctx, dual); err != nil {
		t.Fatalf("create dual book: %v", err)
	}
	audio := &models.Book{
		ForeignID: "OL-AUDIO", AuthorID: author.ID, Title: "Audio", SortTitle: "Audio",
		Monitored: true, Status: models.BookStatusWanted, MediaType: models.MediaTypeAudiobook,
	}
	if err := repo.Create(ctx, audio); err != nil {
		t.Fatalf("create audiobook book: %v", err)
	}

	ids := func(t *testing.T, mediaType string) map[int64]bool {
		t.Helper()
		books, _, err := repo.ListPageFiltered(ctx, BookListFilter{MediaType: mediaType}, 100, 0)
		if err != nil {
			t.Fatalf("ListPageFiltered(%q): %v", mediaType, err)
		}
		out := make(map[int64]bool, len(books))
		for _, b := range books {
			out[b.ID] = true
		}
		return out
	}

	got := ids(t, "both")
	if !got[dual.ID] {
		t.Error("?mediaType=both must return the dual-format book")
	}
	if got[ebookBook.ID] || got[audio.ID] {
		t.Errorf("?mediaType=both returned single-format books: %v", got)
	}

	// The existing inclusive semantics must survive.
	got = ids(t, "ebook")
	if !got[dual.ID] || !got[ebookBook.ID] {
		t.Errorf("?mediaType=ebook must still include dual-format books: %v", got)
	}
	if got[audio.ID] {
		t.Error("?mediaType=ebook must not include audiobook-only books")
	}

	got = ids(t, "audiobook")
	if !got[dual.ID] || !got[audio.ID] {
		t.Errorf("?mediaType=audiobook must still include dual-format books: %v", got)
	}
	if got[ebookBook.ID] {
		t.Error("?mediaType=audiobook must not include ebook-only books")
	}
}
