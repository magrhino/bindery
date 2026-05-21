package migrate

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vavallee/bindery/internal/isbnutil"
)

// GoodreadsShelf is one of the three Goodreads "Exclusive Shelf" values.
// Every Goodreads library export assigns each row exactly one of these.
const (
	GoodreadsShelfToRead           = "to-read"
	GoodreadsShelfRead             = "read"
	GoodreadsShelfCurrentlyReading = "currently-reading"
)

// GoodreadsRow is a single parsed line from a Goodreads library CSV export.
// Only the subset of columns Bindery uses is retained; Goodreads-internal
// fields (Book Id, ratings, dates) are dropped during parsing.
type GoodreadsRow struct {
	// RowNumber is the 1-based data-row index (header excluded). Used to
	// keep the failed-rows CSV in the same order the user uploaded.
	RowNumber int `json:"rowNumber"`

	Title             string `json:"title"`
	Author            string `json:"author"`
	AdditionalAuthors string `json:"additionalAuthors,omitempty"`
	ISBN              string `json:"isbn,omitempty"`   // ISBN-10, separators stripped
	ISBN13            string `json:"isbn13,omitempty"` // ISBN-13, separators stripped
	ExclusiveShelf    string `json:"exclusiveShelf"`   // to-read | read | currently-reading
	Bookshelves       string `json:"bookshelves,omitempty"`
}

// goodreadsHeaderAliases maps a normalized header cell to the canonical field
// name. Goodreads has shipped a few header spellings over the years and some
// third-party exporters reorder or rename columns, so resolution is by name,
// never by fixed position.
var goodreadsHeaderAliases = map[string]string{
	"title":              "title",
	"author":             "author",
	"author l-f":         "author_lf",
	"additional authors": "additional_authors",
	"isbn":               "isbn",
	"isbn13":             "isbn13",
	"exclusive shelf":    "exclusive_shelf",
	"bookshelves":        "bookshelves",
}

// ParseGoodreadsCSV reads a Goodreads library CSV export and returns one
// GoodreadsRow per data line. The header row is located by name so column
// order is irrelevant. Rows with no usable title are dropped (a title is the
// minimum needed to attempt resolution).
//
// Goodreads wraps ISBN cells as ="0312853238" to stop spreadsheet software
// mangling them into numbers; that Excel-formula wrapper is stripped here.
// Many real exports leave ISBN/ISBN13 empty for older or self-published
// titles — that is expected, and such rows fall through to the title+author
// resolution path.
func ParseGoodreadsCSV(reader io.Reader) ([]GoodreadsRow, error) {
	if reader == nil {
		return nil, errors.New("reader is nil")
	}

	cr := csv.NewReader(reader)
	cr.FieldsPerRecord = -1 // tolerate ragged rows; we index by header
	cr.LazyQuotes = true    // Goodreads descriptions occasionally contain stray quotes

	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse goodreads csv: %w", err)
	}
	if len(records) == 0 {
		return nil, errors.New("goodreads csv is empty")
	}

	colIndex, err := mapGoodreadsHeader(records[0])
	if err != nil {
		return nil, err
	}

	get := func(rec []string, field string) string {
		idx, ok := colIndex[field]
		if !ok || idx >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[idx])
	}

	rows := make([]GoodreadsRow, 0, len(records)-1)
	for i, rec := range records[1:] {
		title := get(rec, "title")
		if title == "" {
			continue // a row with no title cannot be resolved
		}
		row := GoodreadsRow{
			RowNumber:         i + 1,
			Title:             title,
			Author:            firstNonEmptyStr(get(rec, "author"), goodreadsAuthorFromLF(get(rec, "author_lf"))),
			AdditionalAuthors: get(rec, "additional_authors"),
			ISBN:              cleanGoodreadsISBN(get(rec, "isbn")),
			ISBN13:            cleanGoodreadsISBN(get(rec, "isbn13")),
			ExclusiveShelf:    normalizeGoodreadsShelf(get(rec, "exclusive_shelf")),
			Bookshelves:       get(rec, "bookshelves"),
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// mapGoodreadsHeader resolves the header row to a field→column-index map.
// A "Title" column is mandatory; everything else is optional.
func mapGoodreadsHeader(header []string) (map[string]int, error) {
	colIndex := make(map[string]int, len(header))
	for i, cell := range header {
		key := strings.ToLower(strings.TrimSpace(cell))
		if canonical, ok := goodreadsHeaderAliases[key]; ok {
			if _, exists := colIndex[canonical]; !exists {
				colIndex[canonical] = i
			}
		}
	}
	if _, ok := colIndex["title"]; !ok {
		return nil, errors.New("goodreads csv is missing a Title column — is this a Goodreads library export?")
	}
	return colIndex, nil
}

// cleanGoodreadsISBN unwraps the ="..." Excel-formula wrapper Goodreads puts
// around ISBN cells, then normalizes separators. Returns "" for empty cells
// (the common case for older titles).
func cleanGoodreadsISBN(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Strip the leading = and surrounding quotes: ="0312853238" → 0312853238.
	raw = strings.TrimPrefix(raw, "=")
	raw = strings.Trim(raw, "\"")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return isbnutil.Normalize(raw)
}

// goodreadsAuthorFromLF converts a "Last, First" cell to "First Last".
// Used only as a fallback when the plain Author column is empty.
func goodreadsAuthorFromLF(lf string) string {
	lf = strings.TrimSpace(lf)
	if lf == "" {
		return ""
	}
	comma := strings.IndexByte(lf, ',')
	if comma < 0 {
		return lf
	}
	last := strings.TrimSpace(lf[:comma])
	first := strings.TrimSpace(lf[comma+1:])
	if first == "" {
		return last
	}
	if last == "" {
		return first
	}
	return first + " " + last
}

// normalizeGoodreadsShelf lower-cases and trims an Exclusive Shelf value.
// Unknown values are returned as-is so the caller can decide how to treat
// them (they are filtered out by the shelf-filter, never silently retagged).
func normalizeGoodreadsShelf(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// firstNonEmptyStr returns the first non-empty string from the arguments.
func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
