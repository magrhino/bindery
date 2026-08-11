package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/httpsec"
	"github.com/vavallee/bindery/internal/indexer"
	"github.com/vavallee/bindery/internal/indexer/newznab"
	"github.com/vavallee/bindery/internal/models"
)

// mockIndexerSearcher implements indexerSearcher for unit tests.
type mockIndexerSearcher struct {
	ebookResults []newznab.SearchResult
	audioResults []newznab.SearchResult
}

func (m *mockIndexerSearcher) SearchBookWithDebug(_ context.Context, _ []models.Indexer, c indexer.MatchCriteria) ([]newznab.SearchResult, *indexer.SearchDebug) {
	switch c.MediaType {
	case models.MediaTypeEbook:
		return m.ebookResults, nil
	case models.MediaTypeAudiobook:
		return m.audioResults, nil
	default:
		return append(m.ebookResults, m.audioResults...), nil
	}
}

func (m *mockIndexerSearcher) SearchQuery(_ context.Context, _ []models.Indexer, _ string) []newznab.SearchResult {
	return nil
}

func indexerFixture(t *testing.T) *IndexerHandler {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return NewIndexerHandler(
		db.NewIndexerRepo(database),
		db.NewBookRepo(database),
		db.NewAuthorRepo(database),
		db.NewMetadataProfileRepo(database),
		nil, // searcher — not needed for CRUD tests
		db.NewSettingsRepo(database),
		db.NewBlocklistRepo(database),
	)
}

func TestIndexerList_Empty(t *testing.T) {
	h := indexerFixture(t)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/indexer", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var out []models.Indexer
	json.NewDecoder(rec.Body).Decode(&out)
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d items", len(out))
	}
}

func TestIndexerCRUD(t *testing.T) {
	h := indexerFixture(t)

	// Create
	body := `{"name":"NZBGeek","url":"https://api.nzbgeek.info","apiKey":"testkey","type":"newznab"}`
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/indexer", bytes.NewBufferString(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created models.Indexer
	json.NewDecoder(rec.Body).Decode(&created)
	if created.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	// Default categories should be set
	if len(created.Categories) == 0 {
		t.Error("expected default categories to be populated")
	}

	// List
	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/indexer", nil))
	var list []models.Indexer
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("expected 1 indexer, got %d", len(list))
	}

	// Get
	rec = httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/indexer/1", nil), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Errorf("get: expected 200, got %d", rec.Code)
	}

	// Get — not found
	rec = httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/indexer/999", nil), "id", "999"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("get missing: expected 404, got %d", rec.Code)
	}

	// Update
	update := `{"name":"NZBGeek Updated","url":"https://api.nzbgeek.info","apiKey":"newkey","type":"newznab","categories":[7000]}`
	rec = httptest.NewRecorder()
	h.Update(rec, withURLParam(httptest.NewRequest(http.MethodPut, "/indexer/1", bytes.NewBufferString(update)), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Errorf("update: expected 200, got %d", rec.Code)
	}

	// Update — not found
	rec = httptest.NewRecorder()
	h.Update(rec, withURLParam(httptest.NewRequest(http.MethodPut, "/indexer/999", bytes.NewBufferString(update)), "id", "999"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("update missing: expected 404, got %d", rec.Code)
	}

	// Delete
	rec = httptest.NewRecorder()
	h.Delete(rec, withURLParam(httptest.NewRequest(http.MethodDelete, "/indexer/1", nil), "id", "1"))
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete: expected 204, got %d", rec.Code)
	}
}

func TestIndexerCreate_Validation(t *testing.T) {
	h := indexerFixture(t)
	for _, tc := range []struct {
		body string
		desc string
	}{
		{`{}`, "empty body"},
		{`{"name":"x"}`, "missing url"},
		{`{"url":"https://example.com"}`, "missing name"},
		{`not-json`, "invalid json"},
	} {
		rec := httptest.NewRecorder()
		h.Create(rec, httptest.NewRequest(http.MethodPost, "/indexer", bytes.NewBufferString(tc.body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", tc.desc, rec.Code)
		}
	}
}

func TestIndexerCreate_DuplicateURL(t *testing.T) {
	h := indexerFixture(t)
	body := `{"name":"NZBGeek","url":"https://api.nzbgeek.info","apiKey":"k"}`
	// First create succeeds
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/indexer", bytes.NewBufferString(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", rec.Code)
	}
	// Second create with same URL should conflict
	rec = httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/indexer", bytes.NewBufferString(body)))
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate url: expected 409, got %d", rec.Code)
	}
}

func TestIndexerTest_NotFound(t *testing.T) {
	h := indexerFixture(t)
	rec := httptest.NewRecorder()
	h.Test(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/indexer/999/test", nil), "id", "999"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestIndexerTestConfig_MissingURL(t *testing.T) {
	h := indexerFixture(t)
	rec := httptest.NewRecorder()
	h.TestConfig(rec, httptest.NewRequest(http.MethodPost, "/indexer/test", bytes.NewBufferString(`{"apiKey":"k"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing url, got %d", rec.Code)
	}
}

func TestIndexerTestConfig_Reachable(t *testing.T) {
	// httptest binds 127.0.0.1; allow loopback through the SSRF guard.
	defer httpsec.AllowLoopbackForTests()()
	// A reachable newznab-style endpoint returning a caps document. The probe
	// reports ok=true; the unsaved body is never persisted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><caps><categories><category id="7020" name="Ebook"/></categories></caps>`))
	}))
	defer srv.Close()

	h := indexerFixture(t)
	rec := httptest.NewRecorder()
	body := `{"name":"X","type":"newznab","url":"` + srv.URL + `","apiKey":"k"}`
	h.TestConfig(rec, httptest.NewRequest(http.MethodPost, "/indexer/test", bytes.NewBufferString(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out IndexerTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Errorf("expected ok=true for reachable indexer, got error %q", out.Error)
	}
}

func TestIndexerTestConfig_Unreachable(t *testing.T) {
	defer httpsec.AllowLoopbackForTests()()
	// A reachable-but-failing probe returns HTTP 200 with an inline error so
	// the UI can render the actionable message instead of a generic toast.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad api key"))
	}))
	defer srv.Close()

	h := indexerFixture(t)
	rec := httptest.NewRecorder()
	body := `{"name":"X","type":"newznab","url":"` + srv.URL + `","apiKey":"wrong"}`
	h.TestConfig(rec, httptest.NewRequest(http.MethodPost, "/indexer/test", bytes.NewBufferString(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out IndexerTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.OK || out.Error == "" {
		t.Errorf("expected ok=false with an error, got ok=%v error=%q", out.OK, out.Error)
	}
}

func TestIndexerSearchQuery_MissingQ(t *testing.T) {
	h := indexerFixture(t)
	rec := httptest.NewRecorder()
	h.SearchQuery(rec, httptest.NewRequest(http.MethodGet, "/indexer/search", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing q param, got %d", rec.Code)
	}
}

func TestSearchBook_DualFormat_MediaTypeTagging(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()

	authorRepo := db.NewAuthorRepo(database)
	author := &models.Author{
		ForeignID: "OL1A", Name: "Jane Doe", SortName: "Doe, Jane",
		MetadataProvider: "openlibrary", Monitored: true,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}

	bookRepo := db.NewBookRepo(database)
	book := &models.Book{
		Title:     "Test Book",
		ForeignID: "OL1M",
		AuthorID:  author.ID,
		MediaType: models.MediaTypeBoth,
		Monitored: true,
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	mock := &mockIndexerSearcher{
		ebookResults: []newznab.SearchResult{{GUID: "eb1", Title: "Test Book epub"}},
		audioResults: []newznab.SearchResult{{GUID: "au1", Title: "Test Book mp3"}},
	}

	h := NewIndexerHandler(
		db.NewIndexerRepo(database),
		bookRepo,
		authorRepo,
		db.NewMetadataProfileRepo(database),
		mock,
		db.NewSettingsRepo(database),
		db.NewBlocklistRepo(database),
	)

	rec := httptest.NewRecorder()
	req := withURLParam(
		httptest.NewRequest(http.MethodGet, "/indexer/book/1/search", nil),
		"id", strconv.FormatInt(book.ID, 10),
	)
	h.SearchBook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []struct {
			GUID      string `json:"guid"`
			MediaType string `json:"mediaType"`
		} `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	byGUID := make(map[string]string, len(resp.Results))
	for _, r := range resp.Results {
		byGUID[r.GUID] = r.MediaType
	}
	if byGUID["eb1"] != "ebook" {
		t.Errorf("ebook result: got mediaType=%q, want %q", byGUID["eb1"], "ebook")
	}
	if byGUID["au1"] != "audiobook" {
		t.Errorf("audiobook result: got mediaType=%q, want %q", byGUID["au1"], "audiobook")
	}
}

// SearchBook must not return the indexer apikey the search path signs into the
// download URL: interactive search is available to non-admin users, so leaking
// the shared indexer credential in nzbUrl is a cross-user secret disclosure. The
// grab handler re-signs from the indexer id server-side.
func TestSearchBook_RedactsIndexerAPIKey(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()
	authorRepo := db.NewAuthorRepo(database)
	author := &models.Author{ForeignID: "OL1A", Name: "Jane Doe", SortName: "Doe, Jane", MetadataProvider: "openlibrary", Monitored: true}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}
	bookRepo := db.NewBookRepo(database)
	book := &models.Book{Title: "Test Book", ForeignID: "OL1M", AuthorID: author.ID, MediaType: models.MediaTypeEbook, Monitored: true}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	mock := &mockIndexerSearcher{
		ebookResults: []newznab.SearchResult{{
			GUID:   "eb1",
			Title:  "Test Book epub",
			NZBURL: "https://idx.example.com/dl?apikey=SUPERSECRET&id=eb1",
		}},
	}
	h := NewIndexerHandler(
		db.NewIndexerRepo(database), bookRepo, authorRepo,
		db.NewMetadataProfileRepo(database), mock,
		db.NewSettingsRepo(database), db.NewBlocklistRepo(database),
	)

	rec := httptest.NewRecorder()
	req := withURLParam(httptest.NewRequest(http.MethodGet, "/indexer/book/1/search", nil), "id", strconv.FormatInt(book.ID, 10))
	h.SearchBook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SUPERSECRET") {
		t.Fatalf("search response leaked the indexer apikey: %s", rec.Body.String())
	}
	var resp struct {
		Results []struct {
			NZBURL string `json:"nzbUrl"`
		} `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].NZBURL != "https://idx.example.com/dl?id=eb1" {
		t.Fatalf("nzbUrl not redacted as expected: %+v", resp.Results)
	}
}

// slowSearcher records peak concurrency to verify parallel dispatch.
type slowSearcher struct {
	mu           sync.Mutex
	inFlight     int
	peakFlight   int
	delay        time.Duration
	ebookResults []newznab.SearchResult
	audioResults []newznab.SearchResult
}

func (s *slowSearcher) SearchBookWithDebug(_ context.Context, _ []models.Indexer, c indexer.MatchCriteria) ([]newznab.SearchResult, *indexer.SearchDebug) {
	s.mu.Lock()
	s.inFlight++
	if s.inFlight > s.peakFlight {
		s.peakFlight = s.inFlight
	}
	s.mu.Unlock()

	time.Sleep(s.delay)

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()

	switch c.MediaType {
	case models.MediaTypeEbook:
		return s.ebookResults, nil
	case models.MediaTypeAudiobook:
		return s.audioResults, nil
	default:
		return nil, nil
	}
}

func (s *slowSearcher) SearchQuery(_ context.Context, _ []models.Indexer, _ string) []newznab.SearchResult {
	return nil
}

// TestSearchBook_DualFormat_ParallelDispatch verifies that the two
// per-format searches for a MediaTypeBoth book run concurrently rather
// than sequentially. The slowSearcher records peak in-flight count:
// parallel dispatch yields 2; sequential yields 1.
func TestSearchBook_DualFormat_ParallelDispatch(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()

	authorRepo := db.NewAuthorRepo(database)
	author := &models.Author{
		ForeignID: "OL2A", Name: "Test Author", SortName: "Author, Test",
		MetadataProvider: "openlibrary", Monitored: true,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}

	bookRepo := db.NewBookRepo(database)
	book := &models.Book{
		Title: "Parallel Book", ForeignID: "OL2M",
		AuthorID: author.ID, MediaType: models.MediaTypeBoth, Monitored: true,
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	slow := &slowSearcher{
		delay:        30 * time.Millisecond,
		ebookResults: []newznab.SearchResult{{GUID: "pe1", Title: "Parallel Ebook"}},
		audioResults: []newznab.SearchResult{{GUID: "pa1", Title: "Parallel Audio"}},
	}

	h := NewIndexerHandler(
		db.NewIndexerRepo(database),
		bookRepo,
		authorRepo,
		db.NewMetadataProfileRepo(database),
		slow,
		db.NewSettingsRepo(database),
		db.NewBlocklistRepo(database),
	)

	rec := httptest.NewRecorder()
	req := withURLParam(
		httptest.NewRequest(http.MethodGet, "/indexer/book/1/search", nil),
		"id", strconv.FormatInt(book.ID, 10),
	)
	h.SearchBook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if slow.peakFlight < 2 {
		t.Errorf("dual-format search ran sequentially: peak concurrent calls = %d, want ≥ 2", slow.peakFlight)
	}
}

// debugSearcher mirrors the real searcher's debug payload: each per-format
// leg echoes back the criteria it was called with, including the narrowed
// MediaType and the category set for that format.
type debugSearcher struct{}

func (debugSearcher) SearchBookWithDebug(_ context.Context, _ []models.Indexer, c indexer.MatchCriteria) ([]newznab.SearchResult, *indexer.SearchDebug) {
	cats := []int{7020}
	if c.MediaType == models.MediaTypeAudiobook {
		cats = []int{3030}
	}
	return nil, &indexer.SearchDebug{
		Query: indexer.SearchQueryDebug{
			Title:     c.Title,
			Author:    c.Author,
			MediaType: c.MediaType,
		},
		Indexers: []indexer.IndexerDebug{{
			IndexerName: "MyAnonamouse",
			Enabled:     true,
			Categories:  cats,
		}},
	}
}

func (debugSearcher) SearchQuery(_ context.Context, _ []models.Indexer, _ string) []newznab.SearchResult {
	return nil
}

// TestSearchBook_DualFormat_DebugReportsBothMediaType covers the reporting
// half of #1636: a media_type=both book is searched once per format, and the
// merged debug panel used to inherit whichever leg it merged from, so the
// Query summary read "ebook" for a search that also queried the audiobook
// categories listed in the very same panel.
func TestSearchBook_DualFormat_DebugReportsBothMediaType(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()

	authorRepo := db.NewAuthorRepo(database)
	author := &models.Author{
		ForeignID: "OL3A", Name: "Mark Manson", SortName: "Manson, Mark",
		MetadataProvider: "openlibrary", Monitored: true,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}

	bookRepo := db.NewBookRepo(database)
	book := &models.Book{
		Title: "The Subtle Art", ForeignID: "OL3M",
		AuthorID: author.ID, MediaType: models.MediaTypeBoth, Monitored: true,
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	h := NewIndexerHandler(
		db.NewIndexerRepo(database),
		bookRepo,
		authorRepo,
		db.NewMetadataProfileRepo(database),
		debugSearcher{},
		db.NewSettingsRepo(database),
		db.NewBlocklistRepo(database),
	)

	rec := httptest.NewRecorder()
	req := withURLParam(
		httptest.NewRequest(http.MethodGet, "/indexer/book/1/search", nil),
		"id", strconv.FormatInt(book.ID, 10),
	)
	h.SearchBook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Debug struct {
			Query struct {
				MediaType string `json:"mediaType"`
			} `json:"query"`
			Indexers []struct {
				Categories []int `json:"categories"`
			} `json:"indexers"`
		} `json:"debug"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got := resp.Debug.Query.MediaType; got != models.MediaTypeBoth {
		t.Errorf("debug query mediaType: got %q, want %q", got, models.MediaTypeBoth)
	}

	// Guard the premise: the summary may only say "both" because both
	// category trees were actually queried.
	var sawEbookCats, sawAudioCats bool
	for _, idx := range resp.Debug.Indexers {
		for _, c := range idx.Categories {
			switch c {
			case 7020:
				sawEbookCats = true
			case 3030:
				sawAudioCats = true
			}
		}
	}
	if !sawEbookCats || !sawAudioCats {
		t.Errorf("expected both ebook and audiobook categories in the merged debug, got %+v", resp.Debug.Indexers)
	}
}

// TestSearchBook_QualityProfileAnnotatesDisallowedFormat covers the
// interactive half of #1693.
//
// decision.QualityAllowed was never constructed anywhere, so a profile's
// "Allowed formats" checkboxes had no effect. Interactive search deliberately
// ANNOTATES rather than filters: the user is in the loop and may want a
// disallowed format for one specific book, so every result is still returned
// carrying approved=false plus the reason. The scheduler enforces the same spec
// for real, because auto-grab has nobody to ask.
func TestSearchBook_QualityProfileAnnotatesDisallowedFormat(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()
	qualityRepo := db.NewQualityProfileRepo(database)
	profile := &models.QualityProfile{Name: "EPUB only", Cutoff: "epub", Items: []models.QualityItem{
		{Quality: "pdf", Allowed: false},
		{Quality: "epub", Allowed: true},
	}}
	if err := qualityRepo.Create(ctx, profile); err != nil {
		t.Fatal(err)
	}

	authorRepo := db.NewAuthorRepo(database)
	author := &models.Author{
		ForeignID: "OL9A", Name: "Jane Doe", SortName: "Doe, Jane",
		MetadataProvider: "openlibrary", Monitored: true, QualityProfileID: &profile.ID,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}

	bookRepo := db.NewBookRepo(database)
	book := &models.Book{
		Title: "Test Book", ForeignID: "OL9M", AuthorID: author.ID,
		MediaType: models.MediaTypeEbook, Monitored: true,
	}
	if err := bookRepo.Create(ctx, book); err != nil {
		t.Fatal(err)
	}

	mock := &mockIndexerSearcher{ebookResults: []newznab.SearchResult{
		{GUID: "pdf1", Title: "Jane Doe - Test Book.pdf"},
		{GUID: "epub1", Title: "Jane Doe - Test Book.epub"},
		{GUID: "plain1", Title: "Jane Doe - Test Book (2024)"},
	}}

	h := NewIndexerHandler(
		db.NewIndexerRepo(database), bookRepo, authorRepo,
		db.NewMetadataProfileRepo(database), mock,
		db.NewSettingsRepo(database), db.NewBlocklistRepo(database),
	).WithQualityProfiles(qualityRepo)

	rec := httptest.NewRecorder()
	h.SearchBook(rec, withURLParam(
		httptest.NewRequest(http.MethodGet, "/indexer/book/1/search", nil),
		"id", strconv.FormatInt(book.ID, 10),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []struct {
			GUID      string `json:"guid"`
			Approved  bool   `json:"approved"`
			Rejection string `json:"rejection"`
		} `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	got := map[string]bool{}
	for _, r := range resp.Results {
		got[r.GUID] = r.Approved
	}
	// All three are still returned — annotation, not filtering.
	if len(resp.Results) != 3 {
		t.Fatalf("expected all 3 results returned, got %d: %+v", len(resp.Results), resp.Results)
	}
	if approved, ok := got["pdf1"]; !ok || approved {
		t.Errorf("pdf release should be present but not approved under an EPUB-only profile, got approved=%v present=%v", approved, ok)
	}
	if approved, ok := got["epub1"]; !ok || !approved {
		t.Errorf("epub release should be approved, got approved=%v present=%v", approved, ok)
	}
	// A title with no parseable format token must not be blocked (see
	// QualityAllowed's fail-open comment).
	if approved, ok := got["plain1"]; !ok || !approved {
		t.Errorf("release with no format token should be approved, got approved=%v present=%v", approved, ok)
	}
	for _, r := range resp.Results {
		if r.GUID == "pdf1" && r.Rejection == "" {
			t.Error("a disallowed format must carry a rejection reason the UI can show")
		}
	}
}
