package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/abs"
	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

func absConflictFixture(t *testing.T) (*ABSConflictHandler, *db.ABSMetadataConflictRepo, *db.AuthorRepo, *db.BookRepo) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	conflicts := db.NewABSMetadataConflictRepo(database)
	authors := db.NewAuthorRepo(database)
	books := db.NewBookRepo(database)
	return NewABSConflictHandler(conflicts, authors, books), conflicts, authors, books
}

func TestABSConflictHandler_ListAndResolveBookConflict(t *testing.T) {
	t.Parallel()

	h, conflicts, authors, books := absConflictFixture(t)
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
	var listed []absConflictResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 || listed[0].EntityName != "Project Hail Mary" {
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

	h, conflicts, authors, _ := absConflictFixture(t)
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
	var listed []absConflictResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 || !listed[0].AuthorRelinkEligible {
		t.Fatalf("listed = %+v, want authorRelinkEligible=true", listed)
	}
}
