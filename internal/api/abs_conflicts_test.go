package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/abs"
	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

func absConflictFixture(t *testing.T) (*sql.DB, *ABSConflictHandler, *db.ABSMetadataConflictRepo, *db.AuthorRepo, *db.BookRepo) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	conflicts := db.NewABSMetadataConflictRepo(database)
	authors := db.NewAuthorRepo(database)
	books := db.NewBookRepo(database)
	return database, NewABSConflictHandler(conflicts, authors, books), conflicts, authors, books
}

func seedABSConflicts(t *testing.T, database *sql.DB, conflicts *db.ABSMetadataConflictRepo, count int) {
	t.Helper()
	for i := 1; i <= count; i++ {
		conflict := &models.ABSMetadataConflict{
			SourceID:         "default",
			LibraryID:        "lib-books",
			ItemID:           "item-" + strconv.Itoa(i),
			EntityType:       "book",
			LocalID:          int64(i),
			FieldName:        "description",
			ABSValue:         "ABS value " + strconv.Itoa(i),
			UpstreamValue:    "Upstream value " + strconv.Itoa(i),
			AppliedSource:    abs.MetadataSourceUpstream,
			ResolutionStatus: "unresolved",
		}
		if err := conflicts.Upsert(context.Background(), conflict); err != nil {
			t.Fatalf("Upsert conflict %d: %v", i, err)
		}
	}
	ts := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(context.Background(), `UPDATE abs_metadata_conflicts SET updated_at = ?`, ts); err != nil {
		t.Fatalf("normalize conflict timestamps: %v", err)
	}
}

func TestABSConflictHandler_ListDefaultPagination(t *testing.T) {
	database, h, conflicts, _, _ := absConflictFixture(t)
	seedABSConflicts(t, database, conflicts, 2)

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/conflicts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out absConflictListResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if out.Total != 2 || out.Limit != 50 || out.Offset != 0 || len(out.Items) != 2 {
		t.Fatalf("out = %+v, want default pagination metadata and two items", out)
	}
}

func TestABSConflictHandler_ListCustomLimitOffset(t *testing.T) {
	database, h, conflicts, _, _ := absConflictFixture(t)
	seedABSConflicts(t, database, conflicts, 3)

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/conflicts?limit=1&offset=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out absConflictListResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if out.Total != 3 || out.Limit != 1 || out.Offset != 1 || len(out.Items) != 1 || out.Items[0].ItemID != "item-2" {
		t.Fatalf("out = %+v, want second item in stable order", out)
	}
}

func TestABSConflictHandler_ListMaxLimitClamping(t *testing.T) {
	database, h, conflicts, _, _ := absConflictFixture(t)
	seedABSConflicts(t, database, conflicts, 105)

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/conflicts?limit=1000", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out absConflictListResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if out.Total != 105 || out.Limit != 100 || out.Offset != 0 || len(out.Items) != 100 {
		t.Fatalf("out = total %d limit %d offset %d len %d, want clamped page", out.Total, out.Limit, out.Offset, len(out.Items))
	}
}

func TestABSConflictHandler_ListStableOrdering(t *testing.T) {
	database, h, conflicts, _, _ := absConflictFixture(t)
	seedABSConflicts(t, database, conflicts, 3)

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/conflicts?limit=3", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out absConflictListResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	got := []string{}
	for _, item := range out.Items {
		got = append(got, item.ItemID)
	}
	want := []string{"item-3", "item-2", "item-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestABSConflictHandler_ListAndResolveBookConflict(t *testing.T) {
	t.Parallel()

	_, h, conflicts, authors, books := absConflictFixture(t)
	author := &models.Author{ForeignID: "OL-ANDY", Name: "Andy Weir", SortName: "Weir, Andy", MetadataProvider: "openlibrary", Monitored: true}
	if err := authors.Create(context.Background(), author); err != nil {
		t.Fatalf("Create author: %v", err)
	}
	book := &models.Book{
		ForeignID:        "OL-PHM",
		AuthorID:         author.ID,
		Title:            "Project Hail Mary",
		SortTitle:        "Project Hail Mary",
		Description:      "Upstream value",
		Status:           models.BookStatusWanted,
		Monitored:        true,
		AnyEditionOK:     true,
		MetadataProvider: "openlibrary",
	}
	if err := books.Create(context.Background(), book); err != nil {
		t.Fatalf("Create book: %v", err)
	}
	conflict := &models.ABSMetadataConflict{
		SourceID:         "default",
		LibraryID:        "lib-books",
		ItemID:           "li-project-hail-mary",
		EntityType:       "book",
		LocalID:          book.ID,
		FieldName:        "description",
		ABSValue:         "ABS value",
		UpstreamValue:    "Upstream value",
		AppliedSource:    abs.MetadataSourceUpstream,
		ResolutionStatus: "unresolved",
	}
	if err := conflicts.Upsert(context.Background(), conflict); err != nil {
		t.Fatalf("Upsert conflict: %v", err)
	}

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/conflicts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d", rec.Code)
	}
	var listed absConflictListResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].EntityName != "Project Hail Mary" {
		t.Fatalf("listed = %+v, want one decorated conflict", listed)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/abs/conflicts/1/resolve", bytes.NewBufferString(`{"source":"abs"}`))
	req = req.WithContext(context.Background())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	h.Resolve(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Resolve status = %d body=%s", rec.Code, rec.Body.String())
	}

	updated, err := books.GetByID(context.Background(), book.ID)
	if err != nil || updated == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updated.Description != "ABS value" {
		t.Fatalf("book description = %q, want ABS value", updated.Description)
	}

	stored, err := conflicts.GetByID(context.Background(), conflict.ID)
	if err != nil || stored == nil {
		t.Fatalf("GetByID conflict: %v", err)
	}
	if stored.PreferredSource != abs.MetadataSourceABS || stored.ResolutionStatus != "resolved" {
		t.Fatalf("stored conflict = %+v, want resolved ABS preference", stored)
	}
}

func TestABSConflictHandler_ListMarksRelinkEligibleAuthors(t *testing.T) {
	t.Parallel()

	_, h, conflicts, authors, _ := absConflictFixture(t)
	author := &models.Author{
		ForeignID:        "abs:author:lib-books:author-tolkien",
		Name:             "J. R. R. Tolkien",
		SortName:         "Tolkien, J. R. R.",
		MetadataProvider: "audiobookshelf",
		Monitored:        true,
	}
	if err := authors.Create(context.Background(), author); err != nil {
		t.Fatalf("Create author: %v", err)
	}
	conflict := &models.ABSMetadataConflict{
		SourceID:         "default",
		LibraryID:        "lib-books",
		ItemID:           "li-hobbit",
		EntityType:       "author",
		LocalID:          author.ID,
		FieldName:        "description",
		ABSValue:         "",
		UpstreamValue:    "Author of The Hobbit.",
		AppliedSource:    abs.MetadataSourceUpstream,
		ResolutionStatus: "unresolved",
	}
	if err := conflicts.Upsert(context.Background(), conflict); err != nil {
		t.Fatalf("Upsert conflict: %v", err)
	}

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/conflicts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status = %d", rec.Code)
	}
	var listed absConflictListResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Items) != 1 || !listed.Items[0].AuthorRelinkEligible {
		t.Fatalf("listed = %+v, want authorRelinkEligible=true", listed)
	}
}
