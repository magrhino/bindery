package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

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

func TestIndexerSearchQuery_MissingQ(t *testing.T) {
	h := indexerFixture(t)
	rec := httptest.NewRecorder()
	h.SearchQuery(rec, httptest.NewRequest(http.MethodGet, "/indexer/search", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing q param, got %d", rec.Code)
	}
}
