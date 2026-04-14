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

func tagFixture(t *testing.T) *TagHandler {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return NewTagHandler(db.NewTagRepo(database))
}

func TestTagList_Empty(t *testing.T) {
	h := tagFixture(t)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/tag", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var out []models.Tag
	json.NewDecoder(rec.Body).Decode(&out)
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d items", len(out))
	}
}

func TestTagCRUD(t *testing.T) {
	h := tagFixture(t)

	// Create
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/tag", bytes.NewBufferString(`{"name":"sci-fi"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rec.Code)
	}
	var created models.Tag
	json.NewDecoder(rec.Body).Decode(&created)
	if created.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// List
	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/tag", nil))
	var list []models.Tag
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 || list[0].Name != "sci-fi" {
		t.Errorf("unexpected list: %v", list)
	}

	// Delete
	rec = httptest.NewRecorder()
	h.Delete(rec, withURLParam(httptest.NewRequest(http.MethodDelete, "/tag/1", nil), "id", "1"))
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete: expected 204, got %d", rec.Code)
	}

	// List after delete
	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/tag", nil))
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("expected empty after delete, got %d", len(list))
	}
}

func TestTagCreate_Validation(t *testing.T) {
	h := tagFixture(t)
	for _, body := range []string{`{}`, `{"name":""}`, `not-json`} {
		rec := httptest.NewRecorder()
		h.Create(rec, httptest.NewRequest(http.MethodPost, "/tag", bytes.NewBufferString(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, rec.Code)
		}
	}
}
