package migrate

import (
	"strings"
	"testing"
)

// A representative Goodreads library export header. Real exports carry more
// columns; the parser must resolve by name and ignore the rest.
const goodreadsHeader = `Book Id,Title,Author,Author l-f,Additional Authors,ISBN,ISBN13,My Rating,Average Rating,Publisher,Binding,Number of Pages,Year Published,Original Publication Year,Date Read,Date Added,Bookshelves,Bookshelves with positions,Exclusive Shelf,My Review,Spoiler,Private Notes,Read Count,Owned Copies`

func TestParseGoodreadsCSV_HappyPath(t *testing.T) {
	csv := goodreadsHeader + "\n" +
		`1234,Project Hail Mary,Andy Weir,"Weir, Andy",,="0593135202",="9780593135204",5,4.52,Ballantine,Hardcover,476,2021,2021,,2024/01/02,to-read,"to-read (#1)",to-read,,false,,0,0` + "\n"

	rows, err := ParseGoodreadsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.Title != "Project Hail Mary" {
		t.Errorf("title = %q", r.Title)
	}
	if r.Author != "Andy Weir" {
		t.Errorf("author = %q", r.Author)
	}
	// The ="..." Excel-formula wrapper must be stripped.
	if r.ISBN != "0593135202" {
		t.Errorf("isbn = %q, want unwrapped 0593135202", r.ISBN)
	}
	if r.ISBN13 != "9780593135204" {
		t.Errorf("isbn13 = %q, want unwrapped 9780593135204", r.ISBN13)
	}
	if r.ExclusiveShelf != GoodreadsShelfToRead {
		t.Errorf("shelf = %q", r.ExclusiveShelf)
	}
	if r.RowNumber != 1 {
		t.Errorf("rowNumber = %d, want 1", r.RowNumber)
	}
}

// TestParseGoodreadsCSV_ISBNSparse covers the very common real-world case of a
// Goodreads export where ISBN/ISBN13 are empty for older or self-published
// titles. The parser must still produce a usable row so the title+author
// resolution path can run.
func TestParseGoodreadsCSV_ISBNSparse(t *testing.T) {
	csv := goodreadsHeader + "\n" +
		// empty ISBN cells, and Goodreads also emits ="" for "no ISBN"
		`9,The Old Book,Jane Austen,"Austen, Jane",,="",="",4,4.10,,Paperback,300,1850,1813,,2023/05/05,read,"read (#3)",read,,false,,1,1` + "\n" +
		`10,Self Published Thing,Indie Author,"Author, Indie",,,,3,3.5,,ebook,120,2022,2022,,2024/02/02,to-read,,to-read,,false,,0,0` + "\n"

	rows, err := ParseGoodreadsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.ISBN != "" || r.ISBN13 != "" {
			t.Errorf("row %q: expected empty ISBNs, got isbn=%q isbn13=%q", r.Title, r.ISBN, r.ISBN13)
		}
		if r.Title == "" || r.Author == "" {
			t.Errorf("ISBN-sparse row must still carry title+author: %+v", r)
		}
	}
	if rows[0].ExclusiveShelf != GoodreadsShelfRead {
		t.Errorf("row 0 shelf = %q", rows[0].ExclusiveShelf)
	}
}

// TestParseGoodreadsCSV_AuthorFromLF verifies the "Last, First" fallback is
// used only when the plain Author column is empty.
func TestParseGoodreadsCSV_AuthorFromLF(t *testing.T) {
	csv := goodreadsHeader + "\n" +
		`5,Some Title,,"Tolkien, J.R.R.",,,,0,0,,,0,0,0,,2024/01/01,to-read,,to-read,,false,,0,0` + "\n"
	rows, err := ParseGoodreadsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Author != "J.R.R. Tolkien" {
		t.Errorf("author from l-f = %q, want %q", rows[0].Author, "J.R.R. Tolkien")
	}
}

// TestParseGoodreadsCSV_DropsTitlelessRows ensures a row with no title is
// dropped — it cannot be resolved.
func TestParseGoodreadsCSV_DropsTitlelessRows(t *testing.T) {
	csv := goodreadsHeader + "\n" +
		`1,Real Book,Real Author,"Author, Real",,,,0,0,,,0,0,0,,2024/01/01,to-read,,to-read,,false,,0,0` + "\n" +
		`2,,Ghost Author,"Author, Ghost",,,,0,0,,,0,0,0,,2024/01/01,to-read,,to-read,,false,,0,0` + "\n"
	rows, err := ParseGoodreadsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "Real Book" {
		t.Fatalf("expected only the titled row, got %+v", rows)
	}
}

// TestParseGoodreadsCSV_ColumnReorder verifies header-name resolution works
// even when columns are reordered (third-party exporters do this).
func TestParseGoodreadsCSV_ColumnReorder(t *testing.T) {
	csv := "Exclusive Shelf,ISBN13,Author,Title\n" +
		`currently-reading,="9780000000001",Some Author,Some Title` + "\n"
	rows, err := ParseGoodreadsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.Title != "Some Title" || r.Author != "Some Author" {
		t.Errorf("reordered columns mis-mapped: %+v", r)
	}
	if r.ISBN13 != "9780000000001" {
		t.Errorf("isbn13 = %q", r.ISBN13)
	}
	if r.ExclusiveShelf != GoodreadsShelfCurrentlyReading {
		t.Errorf("shelf = %q", r.ExclusiveShelf)
	}
}

func TestParseGoodreadsCSV_MissingTitleColumn(t *testing.T) {
	csv := "Author,ISBN\nSome Author,123\n"
	_, err := ParseGoodreadsCSV(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected an error for a CSV with no Title column")
	}
}

func TestParseGoodreadsCSV_Empty(t *testing.T) {
	_, err := ParseGoodreadsCSV(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected an error for an empty CSV")
	}
}

func TestCleanGoodreadsISBN(t *testing.T) {
	cases := map[string]string{
		`="0593135202"`:    "0593135202",
		`="9780593135204"`: "9780593135204",
		`=""`:              "",
		"":                 "",
		"0-306-40615-2":    "0306406152", // separators stripped
		`"0306406152"`:     "0306406152",
	}
	for in, want := range cases {
		if got := cleanGoodreadsISBN(in); got != want {
			t.Errorf("cleanGoodreadsISBN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGoodreadsAuthorFromLF(t *testing.T) {
	cases := map[string]string{
		"Weir, Andy":        "Andy Weir",
		"Tolkien, J.R.R.":   "J.R.R. Tolkien",
		"Madonna":           "Madonna",
		"":                  "",
		"Stockett, Kathryn": "Kathryn Stockett",
	}
	for in, want := range cases {
		if got := goodreadsAuthorFromLF(in); got != want {
			t.Errorf("goodreadsAuthorFromLF(%q) = %q, want %q", in, got, want)
		}
	}
}
