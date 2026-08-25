package openlibrary

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

// editionCorpus builds n edition entries, none of which carries an ISBN or a
// page count unless its index appears in withData. That mirrors the shape of a
// heavily reprinted OpenLibrary work: most entries are thin stubs and the ones
// carrying real identifiers sit wherever OL happens to place them.
func editionCorpus(n int, withData ...int) []editionEntry {
	rich := make(map[int]bool, len(withData))
	for _, i := range withData {
		rich[i] = true
	}
	entries := make([]editionEntry, 0, n)
	for i := range n {
		e := editionEntry{
			Key:   fmt.Sprintf("/books/OL%04dM", i),
			Title: fmt.Sprintf("Edition %d", i),
		}
		if rich[i] {
			e.ISBN13 = []string{fmt.Sprintf("978000000%04d", i)}
			e.NumberOfPages = 320
		}
		entries = append(entries, e)
	}
	return entries
}

// pagedEditionsHandler serves a corpus the way OpenLibrary does: `size` is the
// work's total edition count and `entries` is the limit/offset window into it.
// requestedOffsets records every offset asked for, in order.
func pagedEditionsHandler(entries []editionEntry, requestedOffsets *[]int) func(*http.Request) string {
	return func(r *http.Request) string {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		if requestedOffsets != nil {
			*requestedOffsets = append(*requestedOffsets, offset)
		}
		page := []editionEntry{}
		if offset < len(entries) {
			end := min(offset+limit, len(entries))
			page = entries[offset:end]
		}
		return jsonStr(editionsResponse{Size: len(entries), Entries: page})
	}
}

// A work with more editions than one old page held used to come back truncated
// to the first 50, in an order OpenLibrary does not sort. That is not just a
// short list: the MinPages and SkipMissingISBN metadata-profile filters ask
// whether ANY edition carries a page count or an ISBN, so a work whose only
// qualifying edition sat past position 50 was skipped on author sync and never
// entered the library (#1779).
func TestGetEditions_HTTP_ReturnsEveryEdition(t *testing.T) {
	const total = 139 // Fantastic Mr Fox (OL45804W), the work in the report.
	const isbnAt = 120

	entries := editionCorpus(total, isbnAt)
	c := newClientWithPaths(t, map[string]interface{}{
		"/works/OL45804W/editions.json": pagedEditionsHandler(entries, nil),
	})

	editions, err := c.GetEditions(context.Background(), "OL45804W")
	if err != nil {
		t.Fatalf("GetEditions: %v", err)
	}
	if len(editions) != total {
		t.Fatalf("got %d editions, want all %d", len(editions), total)
	}

	// The filters downstream are "does any edition qualify" predicates, so
	// what matters is that the qualifying edition is reachable at all.
	var sawISBN, sawPages bool
	for _, e := range editions {
		if e.ISBN13 != nil && *e.ISBN13 != "" {
			sawISBN = true
		}
		if e.NumPages != nil && *e.NumPages > 0 {
			sawPages = true
		}
	}
	if !sawISBN {
		t.Errorf("the work's only ISBN-bearing edition (index %d) never reached the caller", isbnAt)
	}
	if !sawPages {
		t.Errorf("the work's only page-count-bearing edition (index %d) never reached the caller", isbnAt)
	}

	want := fmt.Sprintf("OL%04dM", isbnAt)
	if editions[isbnAt].ForeignID != want {
		t.Errorf("edition %d: ForeignID = %q, want %q", isbnAt, editions[isbnAt].ForeignID, want)
	}
}

// A work large enough to need several requests is paged through rather than
// cut off at the first response.
func TestGetEditions_HTTP_PagesBeyondOneRequest(t *testing.T) {
	const total = editionsPageSize*2 + 50

	var offsets []int
	entries := editionCorpus(total, total-1)
	c := newClientWithPaths(t, map[string]interface{}{
		"/works/OL456W/editions.json": pagedEditionsHandler(entries, &offsets),
	})

	editions, err := c.GetEditions(context.Background(), "OL456W")
	if err != nil {
		t.Fatalf("GetEditions: %v", err)
	}
	if len(editions) != total {
		t.Fatalf("got %d editions, want %d", len(editions), total)
	}
	wantOffsets := []int{0, editionsPageSize, editionsPageSize * 2}
	if len(offsets) != len(wantOffsets) {
		t.Fatalf("requested offsets %v, want %v", offsets, wantOffsets)
	}
	for i, want := range wantOffsets {
		if offsets[i] != want {
			t.Fatalf("requested offsets %v, want %v", offsets, wantOffsets)
		}
	}
	if editions[total-1].ISBN13 == nil {
		t.Error("the last edition's ISBN did not survive pagination")
	}
}

// A work whose edition count fits in one response costs exactly one request.
// Paging must not turn the common case into a second round trip against an
// endpoint OpenLibrary throttles.
func TestGetEditions_HTTP_SmallWorkCostsOneRequest(t *testing.T) {
	var offsets []int
	c := newClientWithPaths(t, map[string]interface{}{
		"/works/OL456W/editions.json": pagedEditionsHandler(editionCorpus(12), &offsets),
	})

	editions, err := c.GetEditions(context.Background(), "OL456W")
	if err != nil {
		t.Fatalf("GetEditions: %v", err)
	}
	if len(editions) != 12 {
		t.Fatalf("got %d editions, want 12", len(editions))
	}
	if len(offsets) != 1 {
		t.Fatalf("made %d requests (offsets %v), want 1", len(offsets), offsets)
	}
}

// An upstream that never runs out of entries must not loop forever.
func TestGetEditions_HTTP_StopsAtPaginationCap(t *testing.T) {
	var requests int
	c := newClientWithPaths(t, map[string]interface{}{
		"/works/OL456W/editions.json": func(r *http.Request) string {
			requests++
			if requests > 100 {
				t.Fatal("GetEditions did not stop paginating")
			}
			return jsonStr(editionsResponse{
				Size:    1 << 20,
				Entries: editionCorpus(editionsPageSize),
			})
		},
	})

	editions, err := c.GetEditions(context.Background(), "OL456W")
	if err != nil {
		t.Fatalf("GetEditions: %v", err)
	}
	if len(editions) != editionsMaxFetch {
		t.Fatalf("got %d editions, want the cap of %d", len(editions), editionsMaxFetch)
	}
}

// A failure partway through keeps the editions already collected instead of
// discarding them, matching how the author-works pagination behaves.
func TestGetEditions_HTTP_LaterPageFailureKeepsWhatWeHave(t *testing.T) {
	// The transport's status override is per path, not per call, so the later
	// pages fail by returning a body the JSON decoder rejects instead.
	var requests int
	c := newClientWithPaths(t, map[string]interface{}{
		"/works/OL456W/editions.json": func(r *http.Request) string {
			requests++
			if requests > 1 {
				return `{"entries": [`
			}
			return jsonStr(editionsResponse{
				Size:    editionsPageSize * 3,
				Entries: editionCorpus(editionsPageSize),
			})
		},
	})

	editions, err := c.GetEditions(context.Background(), "OL456W")
	if err != nil {
		t.Fatalf("a later page failing must not fail the lookup, got %v", err)
	}
	if len(editions) != editionsPageSize {
		t.Fatalf("got %d editions, want the %d collected before the failure", len(editions), editionsPageSize)
	}
}

// The first page failing is still a failed lookup: callers treat an error as
// "no edition data for this work" and skip the edition-gated filters, which is
// the safe reading. Returning an empty list instead would look like a work with
// zero editions, which SkipMissingISBN drops.
func TestGetEditions_HTTP_FirstPageFailureIsAnError(t *testing.T) {
	c := newClientWithStatus(t,
		map[string]interface{}{"/works/OL456W/editions.json": `{"error":"boom"}`},
		map[string]int{"/works/OL456W/editions.json": http.StatusInternalServerError},
	)

	if _, err := c.GetEditions(context.Background(), "OL456W"); err == nil {
		t.Fatal("expected an error when the first page fails")
	}
}
