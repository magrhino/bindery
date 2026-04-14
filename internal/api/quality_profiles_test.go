package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/db"
)

func qualityProfileFixture(t *testing.T) *QualityProfileHandler {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return NewQualityProfileHandler(db.NewQualityProfileRepo(database))
}

func TestQualityProfileList_Empty(t *testing.T) {
	h := qualityProfileFixture(t)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/quality-profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if bytes.TrimSpace(rec.Body.Bytes())[0] != '[' {
		t.Errorf("expected JSON array, got %s", rec.Body.String())
	}
}

func TestQualityProfileGet(t *testing.T) {
	h := qualityProfileFixture(t)

	// Bad id
	rec := httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/quality-profile/abc", nil), "id", "abc"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: expected 400, got %d", rec.Code)
	}

	// Missing
	rec = httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/quality-profile/999", nil), "id", "999"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing: expected 404, got %d", rec.Code)
	}
}
