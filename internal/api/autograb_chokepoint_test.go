package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/indexer"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/scheduler"
)

// These are the #2256 end-to-end guards for the three dispatch sites that used
// to grab with autoGrab.enabled off: the bulk fan-out, add-book, and
// add-from-recommendations.
//
// They deliberately wire a real *scheduler.Scheduler as the BookSearcher
// rather than the usual spy. The switch is enforced inside SearchAndGrabBook,
// so a spy would only prove the handler still calls the searcher, which it
// does and should. The observable that actually matters is whether an indexer
// request goes out: nothing can be grabbed without one, so zero requests is
// zero grabs. Each case asserts the book is still recorded as wanted, because
// the switch stops the download, not the bookkeeping.

// emptyNewznabFeed is a well-formed feed with no items, so every tier of the
// per-book query cascade falls through and the searcher returns nothing. The
// test counts requests, not results.
const emptyNewznabFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel><title>Empty</title><newznab:response offset="0" total="0"/></channel>
</rss>`

// countingIndexer is a fake newznab endpoint that records how many searches
// reached it.
type countingIndexer struct {
	mu   sync.Mutex
	hits int
	srv  *httptest.Server
}

func newCountingIndexer(t *testing.T) *countingIndexer {
	t.Helper()
	c := &countingIndexer{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.hits++
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(emptyNewznabFeed))
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *countingIndexer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits
}

// settleWindow is how long a "no grabs happened" assertion waits before it
// believes the dispatch really was suppressed. The paths under test all hand
// off to a goroutine, so the negative case cannot be observed by waiting on a
// channel. The paired switch-on subtest calibrates it: with the switch on the
// same path reaches the indexer well inside waitForHits' budget.
const settleWindow = 750 * time.Millisecond

// waitForHits blocks until the fake indexer has been called at least once, or
// fails the test.
func waitForHits(t *testing.T, idx *countingIndexer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if idx.count() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the fake indexer was never called with autoGrab.enabled=true; the control case is not wired up, so the switch-off assertion would pass for the wrong reason")
}

// chokepointFixture is a live-ish stack: real repos, a real Scheduler as the
// BookSearcher, and one enabled indexer pointing at a counting fake.
type chokepointFixture struct {
	ctx       context.Context
	authors   *db.AuthorRepo
	books     *db.BookRepo
	series    *db.SeriesRepo
	blocklist *db.BlocklistRepo
	recs      *db.RecommendationRepo
	profiles  *db.MetadataProfileRepo
	settings  *db.SettingsRepo
	sched     *scheduler.Scheduler
	idx       *countingIndexer
}

// newChokepointFixture seeds autoGrab.enabled to the given value ("true" or
// "false") and returns the wired stack.
func newChokepointFixture(t *testing.T, autoGrab string) *chokepointFixture {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	f := &chokepointFixture{
		ctx:       ctx,
		authors:   db.NewAuthorRepo(database),
		books:     db.NewBookRepo(database),
		series:    db.NewSeriesRepo(database),
		blocklist: db.NewBlocklistRepo(database),
		recs:      db.NewRecommendationRepo(database),
		profiles:  db.NewMetadataProfileRepo(database),
		settings:  db.NewSettingsRepo(database),
		idx:       newCountingIndexer(t),
	}
	if err := f.settings.Set(ctx, "autoGrab.enabled", autoGrab); err != nil {
		t.Fatalf("seed autoGrab.enabled=%q: %v", autoGrab, err)
	}

	indexers := db.NewIndexerRepo(database)
	if err := indexers.Create(ctx, &models.Indexer{
		Name: "Fake", Type: "newznab", URL: f.idx.srv.URL, APIKey: "key",
		Categories: []int{7020, 3030}, Enabled: true, SupportsSearch: true,
	}); err != nil {
		t.Fatalf("seed indexer: %v", err)
	}

	f.sched = scheduler.New(ctx, nil, indexer.NewSearcher(), nil,
		f.authors, f.books, indexers,
		db.NewDownloadRepo(database), db.NewDownloadClientRepo(database),
		f.settings, f.blocklist)
	return f
}

// seedAuthor inserts a monitored author for the paths that need one.
func (f *chokepointFixture) seedAuthor(t *testing.T, foreignID, name string) *models.Author {
	t.Helper()
	a := &models.Author{
		ForeignID: foreignID, Name: name, SortName: name,
		MetadataProvider: "openlibrary", Monitored: true,
	}
	if err := f.authors.Create(f.ctx, a); err != nil {
		t.Fatalf("seed author: %v", err)
	}
	return a
}

// assertWanted fails unless the book is on record as wanted. The kill switch
// stops the grab, never the bookkeeping.
func (f *chokepointFixture) assertWanted(t *testing.T, foreignID string) {
	t.Helper()
	b, err := f.books.GetByForeignID(f.ctx, foreignID)
	if err != nil || b == nil {
		t.Fatalf("book %q after the request = %+v err=%v, want it recorded", foreignID, b, err)
	}
	if b.Status != models.BookStatusWanted {
		t.Fatalf("book %q status = %q, want %q: the switch must stop the grab, not the bookkeeping",
			foreignID, b.Status, models.BookStatusWanted)
	}
}

// ---------------------------------------------------------------------------
// Bulk fan-out (was internal/api/bulk.go fanOutSearches, unguarded)
// ---------------------------------------------------------------------------

func TestBulkSearch_HonoursAutoGrabSwitch(t *testing.T) {
	// The bulk pool paces launches three seconds apart in production; the
	// switch, not the pacing, is what is under test here.
	searchPaceInterval = 0
	t.Cleanup(func() { searchPaceInterval = 3 * time.Second })

	for _, tc := range []struct {
		name     string
		autoGrab string
	}{{"switch on reaches the indexer", "true"}, {"switch off grabs nothing", "false"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newChokepointFixture(t, tc.autoGrab)
			author := f.seedAuthor(t, "OL_CHOKE_BULK", "Bulk Author")
			book := &models.Book{
				ForeignID: "OL_CHOKE_BULK_B1", AuthorID: author.ID,
				Title: "Wanted One", SortTitle: "wanted one",
				Status: models.BookStatusWanted, Monitored: true,
				MetadataProvider: "openlibrary", Genres: []string{},
			}
			if err := f.books.Create(f.ctx, book); err != nil {
				t.Fatalf("seed book: %v", err)
			}

			h := NewBulkHandler(f.authors, f.books, f.blocklist, f.sched).
				WithSeriesRepo(f.series).
				WithLifetimeCtx(f.ctx)

			rec := postBulk(t, h.BooksBulk, fmt.Sprintf(`{"ids":[%d],"action":"search"}`, book.ID))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}

			if tc.autoGrab == "true" {
				waitForHits(t, f.idx)
				return
			}
			time.Sleep(settleWindow)
			if got := f.idx.count(); got != 0 {
				t.Fatalf("bulk search issued %d indexer requests with autoGrab.enabled=false, want 0 (#2256)", got)
			}
			f.assertWanted(t, book.ForeignID)
		})
	}
}

// ---------------------------------------------------------------------------
// Add book (was internal/api/authors.go AddBook, unguarded)
// ---------------------------------------------------------------------------

func TestAddBook_HonoursAutoGrabSwitch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		autoGrab string
	}{{"switch on reaches the indexer", "true"}, {"switch off grabs nothing", "false"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newChokepointFixture(t, tc.autoGrab)
			author := f.seedAuthor(t, "OL-CHOKE-ADD", "Add Author")
			if err := f.authors.UpsertAuthorIdentifier(f.ctx, author.ID, "hc:choke-add"); err != nil {
				t.Fatalf("seed author identifier: %v", err)
			}

			provider := &stubMetaProvider{
				name: "hardcover",
				getBookByID: map[string]*models.Book{
					"hc:choke-book": {
						ForeignID: "hc:choke-book", Title: "Chokepoint Book",
						SortTitle: "Chokepoint Book", Status: models.BookStatusWanted,
						Genres: []string{}, MetadataProvider: "hardcover",
					},
				},
			}
			h := NewAuthorHandler(f.authors, nil, f.books, f.series,
				metadata.NewAggregator(provider), f.settings, f.profiles, f.sched)

			body, _ := json.Marshal(map[string]any{
				"foreignBookId":   "hc:choke-book",
				"foreignAuthorId": "hc:choke-add",
				"authorName":      "Add Author",
				"searchOnAdd":     true,
			})
			parent, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/author/book", bytes.NewReader(body)).WithContext(parent)
			rec := httptest.NewRecorder()
			h.AddBook(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
			}

			if tc.autoGrab == "true" {
				waitForHits(t, f.idx)
				return
			}
			time.Sleep(settleWindow)
			if got := f.idx.count(); got != 0 {
				t.Fatalf("add-book issued %d indexer requests with autoGrab.enabled=false, want 0 (#2256)", got)
			}
			f.assertWanted(t, "hc:choke-book")
		})
	}
}

// ---------------------------------------------------------------------------
// Add from recommendations (was internal/api/recommendations.go Add, unguarded)
// ---------------------------------------------------------------------------

func TestRecommendationAdd_HonoursAutoGrabSwitch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		autoGrab string
	}{{"switch on reaches the indexer", "true"}, {"switch off grabs nothing", "false"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newChokepointFixture(t, tc.autoGrab)
			author := f.seedAuthor(t, "hc:choke-rec-author", "Rec Author")
			if err := f.recs.ReplaceBatch(f.ctx, 1, []models.RecommendationCandidate{{
				ForeignID:  "hc:choke-rec-book",
				RecType:    models.RecTypeListCross,
				Title:      "Recommended Chokepoint",
				AuthorName: author.Name,
				AuthorID:   &author.ID,
				MediaType:  models.MediaTypeEbook,
				Genres:     []string{},
				Score:      1,
			}}); err != nil {
				t.Fatalf("seed recommendation: %v", err)
			}

			h := NewRecommendationHandler(f.recs, fakeRecommendationEngine{}, f.authors, f.books, f.sched).
				WithFinder(f.series, nil).
				WithAppContext(f.ctx)

			rec := httptest.NewRecorder()
			h.Add(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/1/add", nil), "id", "1"))
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
			}

			if tc.autoGrab == "true" {
				waitForHits(t, f.idx)
				return
			}
			time.Sleep(settleWindow)
			if got := f.idx.count(); got != 0 {
				t.Fatalf("add-from-recommendations issued %d indexer requests with autoGrab.enabled=false, want 0 (#2256)", got)
			}
			f.assertWanted(t, "hc:choke-rec-book")
		})
	}
}
