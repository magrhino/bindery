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

type stubABSReviewImporter struct {
	lastCfg  abs.ImportConfig
	lastItem abs.NormalizedLibraryItem
}

func (s *stubABSReviewImporter) ImportReview(_ context.Context, cfg abs.ImportConfig, item abs.NormalizedLibraryItem) (abs.ImportItemResult, error) {
	s.lastCfg = cfg
	s.lastItem = item
	return abs.ImportItemResult{ItemID: item.ItemID, Outcome: "created"}, nil
}

func (s *stubABSReviewImporter) ReviewFileMapping(context.Context, abs.ImportConfig, abs.NormalizedLibraryItem) abs.ReviewFileMapping {
	return abs.ReviewFileMapping{}
}

func TestABSReviewHandler_Approve(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	reviews := db.NewABSReviewItemRepo(database)
	payload, _ := json.Marshal(abs.NormalizedLibraryItem{
		ItemID:    "item-123",
		LibraryID: "lib-books",
		Title:     "Harry Potter and the Deathly Hallows",
		Authors:   []abs.NormalizedAuthor{{ID: "author-1", Name: "J.K. Rowling"}},
	})
	if err := reviews.UpsertPending(context.Background(), &models.ABSReviewItem{
		SourceID:      abs.DefaultSourceID,
		LibraryID:     "lib-books",
		ItemID:        "item-123",
		Title:         "Harry Potter and the Deathly Hallows",
		PrimaryAuthor: "J.K. Rowling",
		MediaType:     models.MediaTypeAudiobook,
		ReviewReason:  "unmatched_book",
		PayloadJSON:   string(payload),
		Status:        "pending",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := reviews.ListByStatus(context.Background(), "pending")
	if err != nil || len(items) != 1 {
		t.Fatalf("pending items = %d err=%v, want 1", len(items), err)
	}

	importer := &stubABSReviewImporter{}
	h := NewABSReviewHandler(reviews, importer, func() ABSStoredConfig {
		return ABSStoredConfig{
			BaseURL:   "https://abs.example.com",
			APIKey:    "secret",
			Label:     "Shelf",
			LibraryID: "lib-books",
			Enabled:   true,
		}
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/abs/review/1/approve", bytes.NewReader(nil))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.Approve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if importer.lastCfg.BaseURL != "https://abs.example.com" || importer.lastItem.ItemID != "item-123" {
		t.Fatalf("importer cfg=%+v item=%+v", importer.lastCfg, importer.lastItem)
	}
	updated, err := reviews.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Status != "approved" {
		t.Fatalf("updated review = %+v, want approved", updated)
	}
}

func TestABSReviewHandler_ResolveAuthorGroupsByPrimaryAuthor(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	reviews := db.NewABSReviewItemRepo(database)
	for _, item := range []models.ABSReviewItem{
		{SourceID: abs.DefaultSourceID, LibraryID: "lib-books", ItemID: "item-1", Title: "The Bands of Mourning", PrimaryAuthor: "Brandon Sanderson", MediaType: models.MediaTypeAudiobook, ReviewReason: "unmatched_author", PayloadJSON: `{"itemId":"item-1"}`, Status: "pending"},
		{SourceID: abs.DefaultSourceID, LibraryID: "lib-books", ItemID: "item-2", Title: "Mistborn", PrimaryAuthor: "brandon sanderson", MediaType: models.MediaTypeAudiobook, ReviewReason: "unmatched_author", PayloadJSON: `{"itemId":"item-2"}`, Status: "pending"},
		{SourceID: abs.DefaultSourceID, LibraryID: "lib-books", ItemID: "item-3", Title: "Onyx Storm", PrimaryAuthor: "Rebecca Yarros", MediaType: models.MediaTypeAudiobook, ReviewReason: "unmatched_author", PayloadJSON: `{"itemId":"item-3"}`, Status: "pending"},
	} {
		item := item
		if err := reviews.UpsertPending(context.Background(), &item); err != nil {
			t.Fatal(err)
		}
	}

	h := NewABSReviewHandler(reviews, &stubABSReviewImporter{}, func() ABSStoredConfig { return ABSStoredConfig{} })
	body := bytes.NewBufferString(`{"foreignAuthorId":"OL123A","authorName":"Brandon Sanderson","applyTo":"same_author"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/abs/review/1/resolve-author", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.ResolveAuthor(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	items, err := reviews.ListByStatus(context.Background(), "pending")
	if err != nil {
		t.Fatal(err)
	}
	resolved := 0
	unrelatedUntouched := false
	for _, item := range items {
		if item.PrimaryAuthor == "Rebecca Yarros" {
			unrelatedUntouched = item.ResolvedAuthorForeignID == ""
			continue
		}
		if item.ResolvedAuthorForeignID == "OL123A" && item.ResolvedAuthorName == "Brandon Sanderson" {
			resolved++
		}
		if item.Title == "" {
			t.Fatalf("title unexpectedly changed for %+v", item)
		}
	}
	if resolved != 2 || !unrelatedUntouched {
		t.Fatalf("items = %+v, want two Brandon rows resolved and Rebecca untouched", items)
	}
}

func TestABSReviewHandler_ResolveBookStoresEditedTitle(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	reviews := db.NewABSReviewItemRepo(database)
	if err := reviews.UpsertPending(context.Background(), &models.ABSReviewItem{
		SourceID:      abs.DefaultSourceID,
		LibraryID:     "lib-books",
		ItemID:        "item-1",
		Title:         "The Bands of Mourning (2 of 2)",
		PrimaryAuthor: "Brandon Sanderson",
		MediaType:     models.MediaTypeAudiobook,
		ReviewReason:  "unmatched_book",
		PayloadJSON:   `{"itemId":"item-1"}`,
		Status:        "pending",
	}); err != nil {
		t.Fatal(err)
	}
	h := NewABSReviewHandler(reviews, &stubABSReviewImporter{}, func() ABSStoredConfig { return ABSStoredConfig{} })
	body := bytes.NewBufferString(`{"foreignBookId":"OL456W","title":"The Bands of Mourning","editedTitle":"The Bands of Mourning"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/abs/review/1/resolve-book", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.ResolveBook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	item, err := reviews.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if item.ResolvedBookForeignID != "OL456W" || item.ResolvedBookTitle != "The Bands of Mourning" || item.EditedTitle != "The Bands of Mourning" {
		t.Fatalf("item = %+v, want resolved book fields", item)
	}
}

func TestABSReviewHandler_Dismiss(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	reviews := db.NewABSReviewItemRepo(database)
	if err := reviews.UpsertPending(context.Background(), &models.ABSReviewItem{
		SourceID:      abs.DefaultSourceID,
		LibraryID:     "lib-books",
		ItemID:        "item-456",
		Title:         "Unknown Title",
		PrimaryAuthor: "Unknown Author",
		MediaType:     models.MediaTypeEbook,
		ReviewReason:  "unmatched_author",
		PayloadJSON:   `{"itemId":"item-456","libraryId":"lib-books","title":"Unknown Title"}`,
		Status:        "pending",
	}); err != nil {
		t.Fatal(err)
	}

	h := NewABSReviewHandler(reviews, &stubABSReviewImporter{}, func() ABSStoredConfig { return ABSStoredConfig{} })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/abs/review/1/dismiss", bytes.NewReader(nil))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.Dismiss(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	updated, err := reviews.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Status != "dismissed" {
		t.Fatalf("updated review = %+v, want dismissed", updated)
	}
}
