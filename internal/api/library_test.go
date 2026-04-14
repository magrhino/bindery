package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/importer"
)

func newLibraryHandler(t *testing.T) *LibraryHandler {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	scanner := importer.NewScanner(
		db.NewDownloadRepo(database),
		db.NewDownloadClientRepo(database),
		db.NewBookRepo(database),
		db.NewAuthorRepo(database),
		db.NewHistoryRepo(database),
		t.TempDir(), "", "", "", "",
	)
	return NewLibraryHandler(scanner)
}

func TestLibraryScan_Returns202(t *testing.T) {
	h := newLibraryHandler(t)
	rec := httptest.NewRecorder()
	h.Scan(rec, httptest.NewRequest(http.MethodPost, "/library/scan", nil))
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202 Accepted, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["message"] == "" {
		t.Error("expected non-empty message in response body")
	}
}
