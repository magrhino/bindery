package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/vavallee/bindery/internal/auth"
	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

// languageSkipFixture seeds an author on a profile that allows English only and
// rejects unknown languages, plus a provider catalogue that the filters chew
// through: one keeper, three language rejects (two French, one with no language
// at all), and one junk-titled work. Mirrors the reported shape in #1889, where
// an author refresh dropped 65 of one author's books.
type languageSkipFixture struct {
	handler *AuthorHandler
	author  *models.Author
	books   *db.BookRepo
}

func newLanguageSkipFixture(t *testing.T) languageSkipFixture {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()
	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	profileRepo := db.NewMetadataProfileRepo(database)

	profile := &models.MetadataProfile{
		Name:                    "English only",
		AllowedLanguages:        "eng",
		UnknownLanguageBehavior: models.UnknownLanguageFail,
	}
	if err := profileRepo.Create(ctx, profile); err != nil {
		t.Fatal(err)
	}

	author := &models.Author{
		ForeignID: "OL1889A", Name: "Terry Bisson", SortName: "Bisson, Terry",
		MetadataProvider: "openlibrary", MetadataProfileID: &profile.ID,
	}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}

	works := []models.Book{
		{ForeignID: "OL1W", Title: "Bears Discover Fire", SortTitle: "bears discover fire", Language: "eng",
			Status: models.BookStatusWanted, MetadataProvider: "openlibrary", Genres: []string{}},
		{ForeignID: "OL2W", Title: "Les Ours", SortTitle: "les ours", Language: "fre",
			Status: models.BookStatusWanted, MetadataProvider: "openlibrary", Genres: []string{}},
		{ForeignID: "OL3W", Title: "Le Feu", SortTitle: "le feu", Language: "fre",
			Status: models.BookStatusWanted, MetadataProvider: "openlibrary", Genres: []string{}},
		{ForeignID: "OL4W", Title: "Untitled Work", SortTitle: "untitled work",
			Status: models.BookStatusWanted, MetadataProvider: "openlibrary", Genres: []string{}},
		{ForeignID: "OL5W", Title: "Terry Bisson", SortTitle: "terry bisson", Language: "eng",
			Status: models.BookStatusWanted, MetadataProvider: "openlibrary", Genres: []string{}},
	}
	agg := metadata.NewAggregator(&stubMetaProvider{works: works})
	h := NewAuthorHandler(authorRepo, nil, bookRepo, nil, agg, nil, profileRepo, nil)
	return languageSkipFixture{handler: h, author: author, books: bookRepo}
}

// getAuthorJSON runs the detail handler as userID/role and returns the decoded
// body. Goes through the handler rather than the summary store directly so the
// ownership guard the summary rides on is part of what's asserted.
func getAuthorJSON(t *testing.T, h *AuthorHandler, id, userID int64, role string) (*httptest.ResponseRecorder, models.Author) {
	t.Helper()
	ctx := withAuthCtx(context.Background(), userID, role)
	req := newRequestForID(http.MethodGet, "/api/v1/author/"+strconv.FormatInt(id, 10), id, ctx)
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	var got models.Author
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode author body: %v (%s)", err, rec.Body.String())
		}
	}
	return rec, got
}

// #1889: the counts the sync already tallies must reach the author detail
// response. Before this, a refresh that dropped most of an author's catalogue
// left a Debug line per book and one Info summary — invisible to anyone who
// can't read the container's stdout.
func TestAuthorGet_ReportsLastSyncSkipCounts(t *testing.T) {
	f := newLanguageSkipFixture(t)
	f.handler.FetchAuthorBooks(f.author, false, "")

	rec, got := getAuthorJSON(t, f.handler, f.author.ID, 1, "admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("author Get = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.LastSync == nil {
		t.Fatal("author detail carries no lastSync: the catalogue sync's outcome is still invisible (#1889)")
	}
	sync := got.LastSync
	if sync.Total != 5 {
		t.Errorf("lastSync.total = %d, want 5 (works the provider returned)", sync.Total)
	}
	if sync.Added != 1 {
		t.Errorf("lastSync.added = %d, want 1", sync.Added)
	}
	// Two French works plus one the provider gave no language for, which the
	// profile's unknown_language_behavior=fail rejects.
	if sync.SkippedLanguage != 3 {
		t.Errorf("lastSync.skippedLanguage = %d, want 3", sync.SkippedLanguage)
	}
	if sync.SkippedJunk != 1 {
		t.Errorf("lastSync.skippedJunk = %d, want 1 (the author-name-titled work)", sync.SkippedJunk)
	}
	if sync.SkippedTotal() != 4 {
		t.Errorf("lastSync skipped total = %d, want 4", sync.SkippedTotal())
	}
	if !sync.UnknownLanguageFail {
		t.Error("lastSync.unknownLanguageFail = false, want true: the user can't tell which half of the filter dropped the untitled-language works")
	}
	if len(sync.AllowedLanguages) != 1 || sync.AllowedLanguages[0] != "eng" {
		t.Errorf("lastSync.allowedLanguages = %v, want [eng]", sync.AllowedLanguages)
	}
	if sync.CompletedAt.IsZero() {
		t.Error("lastSync.completedAt is zero; the UI can't tell a fresh summary from a stale one")
	}

	// The sample names what was dropped, which is what tells the user whether
	// the profile is set the way they meant.
	sampled := map[string]string{}
	for _, s := range sync.SkippedLanguageSample {
		sampled[s.Title] = s.Language
	}
	if len(sampled) != 3 {
		t.Fatalf("lastSync.skippedLanguageSample = %+v, want 3 entries", sync.SkippedLanguageSample)
	}
	if lang, ok := sampled["Les Ours"]; !ok || lang != "fre" {
		t.Errorf("sample missing the French reject with its language: %+v", sync.SkippedLanguageSample)
	}
	if lang, ok := sampled["Untitled Work"]; !ok || lang != "" {
		t.Errorf("sample missing the unknown-language reject: %+v", sync.SkippedLanguageSample)
	}
}

// The sample is capped so a prolific author's rejected tail can't bloat every
// author detail response — the counts stay exact, only the named titles are cut.
func TestAuthorGet_LastSyncSampleIsCapped(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	profileRepo := db.NewMetadataProfileRepo(database)

	profile := &models.MetadataProfile{Name: "English only", AllowedLanguages: "eng"}
	if err := profileRepo.Create(ctx, profile); err != nil {
		t.Fatal(err)
	}
	author := &models.Author{ForeignID: "OL1889B", Name: "Prolific", SortName: "Prolific",
		MetadataProvider: "openlibrary", MetadataProfileID: &profile.ID}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}

	works := make([]models.Book, 0, 20)
	for i := range 20 {
		works = append(works, models.Book{
			ForeignID: "OLX" + strconv.Itoa(i), Title: "Roman " + strconv.Itoa(i),
			SortTitle: "roman", Language: "fre", Status: models.BookStatusWanted,
			MetadataProvider: "openlibrary", Genres: []string{},
		})
	}
	h := NewAuthorHandler(authorRepo, nil, bookRepo, nil, metadata.NewAggregator(&stubMetaProvider{works: works}), nil, profileRepo, nil)
	h.FetchAuthorBooks(author, false, "")

	rec, got := getAuthorJSON(t, h, author.ID, 1, "admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("author Get = %d, want 200", rec.Code)
	}
	if got.LastSync == nil {
		t.Fatal("author detail carries no lastSync")
	}
	if got.LastSync.SkippedLanguage != 20 {
		t.Errorf("lastSync.skippedLanguage = %d, want 20 (the count is not sampled)", got.LastSync.SkippedLanguage)
	}
	if len(got.LastSync.SkippedLanguageSample) != authorSyncSkippedSampleLimit {
		t.Errorf("sample length = %d, want %d", len(got.LastSync.SkippedLanguageSample), authorSyncSkippedSampleLimit)
	}
}

// A sync that dropped nothing still reports, so "nothing was filtered" is an
// answer the user can get rather than an absence they have to interpret.
func TestAuthorGet_LastSyncReportsCleanRun(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	profileRepo := db.NewMetadataProfileRepo(database)

	author := &models.Author{ForeignID: "OL1889C", Name: "Clean Run", SortName: "Clean Run",
		MetadataProvider: "openlibrary"}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}
	works := []models.Book{{ForeignID: "OLC1", Title: "Only Book", SortTitle: "only book", Language: "eng",
		Status: models.BookStatusWanted, MetadataProvider: "openlibrary", Genres: []string{}}}
	h := NewAuthorHandler(authorRepo, nil, bookRepo, nil, metadata.NewAggregator(&stubMetaProvider{works: works}), nil, profileRepo, nil)
	h.FetchAuthorBooks(author, false, "")

	_, got := getAuthorJSON(t, h, author.ID, 1, "admin")
	if got.LastSync == nil {
		t.Fatal("author detail carries no lastSync after a clean sync")
	}
	if got.LastSync.SkippedTotal() != 0 || got.LastSync.Added != 1 {
		t.Errorf("clean sync summary = %+v, want added=1 and nothing skipped", got.LastSync)
	}
}

// The summary is diagnostic output about another user's library, so it must not
// escape the ownership guard the rest of the author payload sits behind.
func TestAuthorGet_LastSyncNotVisibleCrossUser(t *testing.T) {
	auth.SetEnforceTenancyForTests(t, true)
	f := seedTwoUserAuthors(t)
	h := NewAuthorHandler(f.authors, nil, f.books, nil, nil, nil, f.profiles, nil)
	h.syncSummaries.record(f.a1.ID, models.AuthorSyncSummary{
		Total: 66, Added: 1, SkippedLanguage: 65,
		SkippedLanguageSample: []models.AuthorSyncSkippedBook{{Title: "Les Ours", Language: "fre"}},
	})

	rec, _ := getAuthorJSON(t, h, f.a1.ID, f.u2, "user")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user author Get = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "skippedLanguage") || strings.Contains(rec.Body.String(), "Les Ours") {
		t.Errorf("cross-user 404 body leaked the sync summary: %s", rec.Body.String())
	}

	rec, got := getAuthorJSON(t, h, f.a1.ID, f.u1, "user")
	if rec.Code != http.StatusOK {
		t.Fatalf("owner author Get = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.LastSync == nil || got.LastSync.SkippedLanguage != 65 {
		t.Errorf("owner must still see the summary; got %+v", got.LastSync)
	}
}

// A sync that dropped books logs its summary at WARN, not INFO: INFO is the
// level the in-app log view captures by default, and the reporter in #1889 was
// running rootless with no way to reach the Debug per-book lines.
func TestFetchAuthorBooks_SkippedBooksLogAtWarn(t *testing.T) {
	f := newLanguageSkipFixture(t)
	logs := captureSlog(t)
	f.handler.FetchAuthorBooks(f.author, false, "")

	var summaryLine string
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "author books synced") {
			summaryLine = line
		}
	}
	if summaryLine == "" {
		t.Fatal("no 'author books synced' summary line was logged")
	}
	if !strings.Contains(summaryLine, "level=WARN") {
		t.Errorf("summary logged at the wrong level for a run that skipped books: %s", summaryLine)
	}
	if !strings.Contains(summaryLine, "skipped_language=3") {
		t.Errorf("summary line lost its breakdown: %s", summaryLine)
	}
}

// A run with nothing skipped stays at INFO — raising every sync to WARN would
// make the level meaningless.
func TestFetchAuthorBooks_CleanSyncLogsAtInfo(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	profileRepo := db.NewMetadataProfileRepo(database)
	author := &models.Author{ForeignID: "OL1889D", Name: "Quiet Author", SortName: "Quiet Author",
		MetadataProvider: "openlibrary"}
	if err := authorRepo.Create(ctx, author); err != nil {
		t.Fatal(err)
	}
	works := []models.Book{{ForeignID: "OLQ1", Title: "A Book", SortTitle: "a book", Language: "eng",
		Status: models.BookStatusWanted, MetadataProvider: "openlibrary", Genres: []string{}}}
	h := NewAuthorHandler(authorRepo, nil, bookRepo, nil, metadata.NewAggregator(&stubMetaProvider{works: works}), nil, profileRepo, nil)

	logs := captureSlog(t)
	h.FetchAuthorBooks(author, false, "")
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "author books synced") && !strings.Contains(line, "level=INFO") {
			t.Errorf("clean sync summary must stay INFO: %s", line)
		}
	}
}

// Author ids come from SQLite's rowid allocation and get reused, so a deleted
// author's counts must not reappear on whatever author lands on that id next.
func TestAuthorDelete_ForgetsSyncSummary(t *testing.T) {
	f := newLanguageSkipFixture(t)
	f.handler.FetchAuthorBooks(f.author, false, "")
	if f.handler.syncSummaries.get(f.author.ID) == nil {
		t.Fatal("precondition: sync should have recorded a summary")
	}

	req := newRequestForID(http.MethodDelete, "/api/v1/author/"+strconv.FormatInt(f.author.ID, 10), f.author.ID, context.Background())
	rec := httptest.NewRecorder()
	f.handler.Delete(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("author Delete = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if got := f.handler.syncSummaries.get(f.author.ID); got != nil {
		t.Errorf("summary survived the author delete: %+v", got)
	}
}
