package audible

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestSearchBooksByAuthor(t *testing.T) {
	var gotQuery, gotResponseGroups, gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/1.0/catalog/products", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("author")
		gotResponseGroups = r.URL.Query().Get("response_groups")
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"products": [
				{
					"asin": "B0036S4B2G",
					"title": "Dune",
					"subtitle": "Dune, Book 1",
					"language": "english",
					"runtime_length_min": 1290,
					"release_date": "2007-08-07",
					"product_images": {"500": "https://example.com/dune-500.jpg", "1024": "https://example.com/dune-1024.jpg"},
					"publisher_summary": "Desert planet epic.",
					"narrators": [{"name": "Scott Brick"}, {"name": "Simon Vance"}],
					"format_type": "unabridged"
				},
				{
					"asin": "B01GXP8A",
					"title": "Der Wüstenplanet",
					"language": "german",
					"release_date": "2019-01-15"
				},
				{
					"asin": "",
					"title": "Missing ASIN skipped"
				},
				{
					"asin": "B999",
					"title": ""
				}
			]
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	books, err := c.SearchBooksByAuthor(context.Background(), "Frank Herbert")
	if err != nil {
		t.Fatalf("SearchBooksByAuthor: %v", err)
	}
	if gotQuery != "Frank Herbert" {
		t.Errorf("author query = %q, want %q", gotQuery, "Frank Herbert")
	}
	if gotResponseGroups == "" {
		t.Error("response_groups query param missing")
	}
	if gotUA == "" {
		t.Error("User-Agent header missing")
	}
	if len(books) != 2 {
		t.Fatalf("got %d books, want 2 (third/fourth should be filtered)", len(books))
	}

	dune := books[0]
	if dune.ForeignID != "audible:B0036S4B2G" {
		t.Errorf("ForeignID = %q", dune.ForeignID)
	}
	if dune.Title != "Dune: Dune, Book 1" {
		t.Errorf("Title = %q (subtitle should be appended)", dune.Title)
	}
	if dune.ASIN != "B0036S4B2G" {
		t.Errorf("ASIN = %q", dune.ASIN)
	}
	if dune.MediaType != models.MediaTypeAudiobook {
		t.Errorf("MediaType = %q, want audiobook", dune.MediaType)
	}
	if dune.Language != "eng" {
		t.Errorf("Language = %q, want eng (normalized from 'english')", dune.Language)
	}
	if dune.Narrator != "Scott Brick, Simon Vance" {
		t.Errorf("Narrator = %q", dune.Narrator)
	}
	if dune.DurationSeconds != 1290*60 {
		t.Errorf("DurationSeconds = %d", dune.DurationSeconds)
	}
	if dune.ImageURL != "https://example.com/dune-1024.jpg" {
		t.Errorf("ImageURL = %q, want largest-size", dune.ImageURL)
	}
	if dune.ReleaseDate == nil || dune.ReleaseDate.Year() != 2007 {
		t.Errorf("ReleaseDate = %v", dune.ReleaseDate)
	}
	if dune.MetadataProvider != "audible" {
		t.Errorf("MetadataProvider = %q", dune.MetadataProvider)
	}

	german := books[1]
	if german.Language != "ger" {
		t.Errorf("German book language = %q, want ger", german.Language)
	}
}

func TestSearchBooksByAuthor_EmptyAuthor(t *testing.T) {
	c := New()
	// No HTTP server — an empty author must not hit the network.
	books, err := c.SearchBooksByAuthor(context.Background(), "   ")
	if err != nil {
		t.Fatalf("empty author: %v", err)
	}
	if books != nil {
		t.Errorf("want nil, got %v", books)
	}
}

func TestSearchBooksByAuthor_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/1.0/catalog/products", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	_, err := c.SearchBooksByAuthor(context.Background(), "Somebody")
	if err == nil {
		t.Fatal("expected error on HTTP 503")
	}
}

// TestSearchBooksByAuthor_MalformedBody verifies that a 200 carrying invalid
// JSON surfaces a decode error rather than panicking or silently returning an
// empty result set. The catalogue response is untrusted upstream input, so a
// truncated/garbage body must fail loudly.
func TestSearchBooksByAuthor_MalformedBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/1.0/catalog/products", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Truncated JSON object — a decoder must reject this.
		_, _ = w.Write([]byte(`{`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	books, err := c.SearchBooksByAuthor(context.Background(), "Somebody")
	if err == nil {
		t.Fatal("expected decode error on malformed JSON body, got nil")
	}
	if books != nil {
		t.Errorf("expected nil books on decode error, got %v", books)
	}
}

// TestSearchBooksByAuthor_ContextCanceled verifies that an already-cancelled
// context makes the call return promptly with a context error instead of
// hanging or returning a nil error. No metadata client had a context-cancel
// test before this.
func TestSearchBooksByAuthor_ContextCanceled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/1.0/catalog/products", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server must not be reached when the context is already cancelled")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	books, err := c.SearchBooksByAuthor(ctx, "Somebody")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if books != nil {
		t.Errorf("expected nil books on cancellation, got %v", books)
	}
}

func TestNormalizeLanguage(t *testing.T) {
	cases := map[string]string{
		"english":   "eng",
		"English":   "eng",
		"  GERMAN ": "ger",
		"french":    "fre",
		"":          "",
		"zulu":      "zulu", // unmapped falls through
		"eng":       "eng",  // already a code
	}
	for in, want := range cases {
		if got := normalizeLanguage(in); got != want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickLargestCover(t *testing.T) {
	if got := pickLargestCover(nil); got != "" {
		t.Errorf("nil map: got %q", got)
	}
	got := pickLargestCover(map[string]string{
		"300":  "small.jpg",
		"1024": "big.jpg",
		"500":  "med.jpg",
		"":     "empty-key",
		"bad":  "nonnumeric.jpg",
	})
	if got != "big.jpg" {
		t.Errorf("pickLargestCover = %q, want big.jpg", got)
	}
}

// TestSearchBooksByAuthor_Paginates verifies that an author whose catalogue
// exceeds the 50-item per-request cap is enumerated in full.
//
// The endpoint's page parameter is 0-indexed, so omitting it returns window 0
// and page=1 returns a DISJOINT second window. The client previously issued
// one unpaged request, which silently dropped every product past the first 50
// with no error and no log line (#1751). The fake below reproduces that exact
// shape: three windows of 50/50/5 over an advertised total_results of 105.
func TestSearchBooksByAuthor_Paginates(t *testing.T) {
	const total = 105
	var pagesSeen []string
	mux := http.NewServeMux()
	mux.HandleFunc("/1.0/catalog/products", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pagesSeen = append(pagesSeen, page)
		start, err := strconv.Atoi(page)
		if err != nil {
			t.Errorf("page parameter missing or non-numeric: %q", page)
			start = 0
		}
		w.Header().Set("Content-Type", "application/json")
		var products []string
		for i := start * maxResults; i < (start+1)*maxResults && i < total; i++ {
			products = append(products, fmt.Sprintf(`{"asin":"B%04d","title":"Title %d"}`, i, i))
		}
		fmt.Fprintf(w, `{"total_results":%d,"products":[%s]}`, total, strings.Join(products, ","))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	books, err := c.SearchBooksByAuthor(context.Background(), "Prolific Author")
	if err != nil {
		t.Fatalf("SearchBooksByAuthor: %v", err)
	}
	if len(books) != total {
		t.Fatalf("got %d books, want %d (catalogue truncated)", len(books), total)
	}
	// Every window must be distinct: a book from the second and third windows
	// proves the client did not simply repeat window 0.
	seen := make(map[string]bool, len(books))
	for _, b := range books {
		if seen[b.ASIN] {
			t.Errorf("duplicate ASIN %q in result set", b.ASIN)
		}
		seen[b.ASIN] = true
	}
	for _, want := range []string{"B0000", "B0050", "B0104"} {
		if !seen[want] {
			t.Errorf("ASIN %q missing — window containing it was never requested", want)
		}
	}
	// 105 products over a 50-item page size is exactly three pages; the
	// total_results short-circuit must stop there rather than probing further.
	if len(pagesSeen) != 3 {
		t.Errorf("requested pages %v, want exactly 3", pagesSeen)
	}
}

// TestSearchBooksByAuthor_StopsWithoutTotalResults verifies the fallback stop
// condition: a catalogue response that omits total_results must still
// terminate, on the first short page.
func TestSearchBooksByAuthor_StopsWithoutTotalResults(t *testing.T) {
	var calls int
	mux := http.NewServeMux()
	mux.HandleFunc("/1.0/catalog/products", func(w http.ResponseWriter, r *http.Request) {
		calls++
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		if page > 0 {
			// Short page: catalogue exhausted.
			fmt.Fprint(w, `{"products":[{"asin":"BLAST","title":"Last"}]}`)
			return
		}
		var products []string
		for i := 0; i < maxResults; i++ {
			products = append(products, fmt.Sprintf(`{"asin":"B%04d","title":"Title %d"}`, i, i))
		}
		fmt.Fprintf(w, `{"products":[%s]}`, strings.Join(products, ","))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	books, err := c.SearchBooksByAuthor(context.Background(), "Someone")
	if err != nil {
		t.Fatalf("SearchBooksByAuthor: %v", err)
	}
	if len(books) != maxResults+1 {
		t.Fatalf("got %d books, want %d", len(books), maxResults+1)
	}
	if calls != 2 {
		t.Errorf("made %d requests, want 2 (stop on the first short page)", calls)
	}
}
