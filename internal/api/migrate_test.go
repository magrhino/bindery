package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

func migrateFixture(t *testing.T, primary metadata.Provider) *MigrateHandler {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return NewMigrateHandler(
		db.NewAuthorRepo(database),
		db.NewIndexerRepo(database),
		db.NewDownloadClientRepo(database),
		db.NewBlocklistRepo(database),
		db.NewBookRepo(database),
		metadata.NewAggregator(primary),
		nil,
	)
}

// multipartBody builds a multipart/form-data body with a single "file" field.
func multipartBody(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return body, w.FormDataContentType()
}

func TestMigrate_ImportCSV_BadMultipart(t *testing.T) {
	h := migrateFixture(t, &stubProvider{})

	// Not multipart at all
	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrate/csv", bytes.NewBufferString("junk"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ImportCSV(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-multipart: expected 400, got %d", rec.Code)
	}
}

func TestMigrate_ImportCSV_MissingFileField(t *testing.T) {
	h := migrateFixture(t, &stubProvider{})

	// Multipart with wrong field name
	body, ct := multipartBody(t, "notfile", "x.csv", "Andy Weir\n")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrate/csv", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ImportCSV(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no file field: expected 400, got %d", rec.Code)
	}
}

func TestMigrate_ImportCSV_Success(t *testing.T) {
	p := &stubProvider{
		authors: []models.Author{{Name: "Andy Weir", ForeignID: "OL1A"}},
	}
	h := migrateFixture(t, p)

	body, ct := multipartBody(t, "file", "authors.csv", "Andy Weir\n")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrate/csv", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ImportCSV(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Payload should be JSON with "requested" field.
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["requested"]; !ok {
		t.Errorf("expected requested field in result, got %+v", got)
	}
}

func TestMigrate_ImportReadarr_BadMultipart(t *testing.T) {
	h := migrateFixture(t, &stubProvider{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrate/readarr", bytes.NewBufferString("junk"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ImportReadarr(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-multipart: expected 400, got %d", rec.Code)
	}
}

func TestMigrate_ImportReadarr_InvalidDB(t *testing.T) {
	h := migrateFixture(t, &stubProvider{})

	// Valid multipart with garbage bytes — the SQLite driver should reject it.
	body, ct := multipartBody(t, "file", "readarr.db", "not a real sqlite file")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/migrate/readarr", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ImportReadarr(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad sqlite: expected 400, got %d", rec.Code)
	}
}
