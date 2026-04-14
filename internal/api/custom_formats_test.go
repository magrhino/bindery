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

func customFormatFixture(t *testing.T) *CustomFormatHandler {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return NewCustomFormatHandler(db.NewCustomFormatRepo(database))
}

func TestCustomFormatList_Empty(t *testing.T) {
	h := customFormatFixture(t)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/custom-format", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if bytes.TrimSpace(rec.Body.Bytes())[0] != '[' {
		t.Errorf("expected JSON array, got %s", rec.Body.String())
	}
}

func TestCustomFormatCRUD(t *testing.T) {
	h := customFormatFixture(t)

	// Create
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/custom-format",
		bytes.NewBufferString(`{"name":"prefer-epub","conditions":[{"type":"releaseTitle","pattern":"epub"}]}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created models.CustomFormat
	json.NewDecoder(rec.Body).Decode(&created)
	if created.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Get
	rec = httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/custom-format/1", nil), "id", "1"))
	if rec.Code != http.StatusOK {
		t.Errorf("get: expected 200, got %d", rec.Code)
	}

	// Get — bad id
	rec = httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/custom-format/abc", nil), "id", "abc"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("get bad id: expected 400, got %d", rec.Code)
	}

	// Get — missing
	rec = httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/custom-format/999", nil), "id", "999"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("get missing: expected 404, got %d", rec.Code)
	}

	// Update (omit conditions — handler should default to [])
	rec = httptest.NewRecorder()
	h.Update(rec, withURLParam(
		httptest.NewRequest(http.MethodPut, "/api/v1/custom-format/1",
			bytes.NewBufferString(`{"name":"renamed"}`)),
		"id", "1"))
	if rec.Code != http.StatusOK {
		t.Errorf("update: expected 200, got %d", rec.Code)
	}

	// Update — bad id
	rec = httptest.NewRecorder()
	h.Update(rec, withURLParam(
		httptest.NewRequest(http.MethodPut, "/api/v1/custom-format/abc", bytes.NewBufferString(`{}`)),
		"id", "abc"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("update bad id: expected 400, got %d", rec.Code)
	}

	// Update — missing
	rec = httptest.NewRecorder()
	h.Update(rec, withURLParam(
		httptest.NewRequest(http.MethodPut, "/api/v1/custom-format/999", bytes.NewBufferString(`{"name":"x"}`)),
		"id", "999"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("update missing: expected 404, got %d", rec.Code)
	}

	// Update — bad body
	rec = httptest.NewRecorder()
	h.Update(rec, withURLParam(
		httptest.NewRequest(http.MethodPut, "/api/v1/custom-format/1", bytes.NewBufferString(`not-json`)),
		"id", "1"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("update bad body: expected 400, got %d", rec.Code)
	}

	// Delete
	rec = httptest.NewRecorder()
	h.Delete(rec, withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/custom-format/1", nil), "id", "1"))
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete: expected 204, got %d", rec.Code)
	}

	// Delete — bad id
	rec = httptest.NewRecorder()
	h.Delete(rec, withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/custom-format/abc", nil), "id", "abc"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("delete bad id: expected 400, got %d", rec.Code)
	}
}

func TestCustomFormatCreate_Validation(t *testing.T) {
	h := customFormatFixture(t)
	for _, tc := range []struct {
		body, desc string
	}{
		{`not-json`, "invalid json"},
		{`{}`, "missing name"},
	} {
		rec := httptest.NewRecorder()
		h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/custom-format", bytes.NewBufferString(tc.body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", tc.desc, rec.Code)
		}
	}
}
