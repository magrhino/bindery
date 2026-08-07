package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// callCreateWithBody invokes Create with a JSON request body (the optional
// {"label": "..."}).
func callCreateWithBody(t *testing.T, h *BackupHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/backup", strings.NewReader(body))
	h.Create(rec, req)
	return rec
}

// withFilenameParam attaches a chi route context carrying the {filename} URL
// parameter that Restore/Delete read.
func withFilenameParam(req *http.Request, filename string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("filename", filename)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// TestBackup_Create_WithLabel is the #1790 feature test: an optional label is
// folded into the backup filename, and the labeled file round-trips through
// List, Restore, and Delete (which previously rejected any name that was not a
// bare timestamp).
func TestBackup_Create_WithLabel(t *testing.T) {
	database, dbPath, dataDir := backupTestDB(t)
	h := NewBackupHandler(database, dbPath, dataDir)

	rec := callCreateWithBody(t, h, `{"label":"pre-import cleanup!"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rec.Code, rec.Body.String())
	}

	entries, err := os.ReadDir(filepath.Join(dataDir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup file, got %d", len(entries))
	}
	name := entries[0].Name()
	// "pre-import cleanup!" -> "pre-import-cleanup" (space and '!' become one '-',
	// collapsed, trailing separator trimmed).
	if !strings.HasSuffix(name, "_pre-import-cleanup.db") {
		t.Fatalf("filename %q does not carry the sanitized label", name)
	}
	if !backupFilenameRe.MatchString(name) {
		t.Fatalf("labeled filename %q rejected by backupFilenameRe", name)
	}

	// Restore accepts the labeled name (bug before the fix: strict regex 400'd it).
	restoreRec := httptest.NewRecorder()
	restoreReq := withFilenameParam(
		httptest.NewRequest(http.MethodPost, "/backup/"+name+"/restore", nil), name)
	restoreReq.Header.Set("X-Confirm-Restore", "true")
	h.Restore(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("restore labeled backup: status=%d body=%s", restoreRec.Code, restoreRec.Body.String())
	}

	// Delete accepts it too.
	delRec := httptest.NewRecorder()
	delReq := withFilenameParam(httptest.NewRequest(http.MethodDelete, "/backup/"+name, nil), name)
	h.Delete(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete labeled backup: status=%d body=%s", delRec.Code, delRec.Body.String())
	}
}

// TestBackup_Create_NoLabel keeps the plain timestamp name when the body is
// empty or omits a usable label.
func TestBackup_Create_NoLabel(t *testing.T) {
	database, dbPath, dataDir := backupTestDB(t)
	h := NewBackupHandler(database, dbPath, dataDir)

	for _, body := range []string{"", `{}`, `{"label":"   "}`, `{"label":"!!!"}`} {
		rec := callCreateWithBody(t, h, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create body=%q: status=%d", body, rec.Code)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		// bindery_YYYYMMDD_HHMMSS.db — 15 chars of timestamp between prefix and ext.
		mid := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "bindery_"), ".db")
		if strings.ContainsAny(mid, "abcdefghijklmnopqrstuvwxyz") {
			t.Errorf("expected a bare timestamp name, got %q", e.Name())
		}
	}
}

func TestSanitizeBackupLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"pre-import", "pre-import"},
		{"just_settings", "just_settings"},
		{"pre-import cleanup!", "pre-import-cleanup"},
		{"../../etc/passwd", "etc-passwd"},
		{"a/b\\c", "a-b-c"},
		{"..", ""},
		{"   ", ""},
		{"日本語", ""},
		{"v1.2.3", "v1-2-3"},
		{strings.Repeat("x", 100), strings.Repeat("x", 40)},
	}
	for _, tc := range cases {
		got := sanitizeBackupLabel(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeBackupLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// The output must always be safe to splice into a filename.
		if strings.ContainsAny(got, `/\.`) {
			t.Errorf("sanitizeBackupLabel(%q) = %q contains a path/extension character", tc.in, got)
		}
	}
}

func TestBackupFilenameRe_RejectsTraversal(t *testing.T) {
	ok := []string{
		"bindery_20260726_181731.db",
		"bindery_20260726_181731_just_settings.db",
		"bindery_20260726_181731_pre-import-cleanup.db",
	}
	for _, name := range ok {
		if !backupFilenameRe.MatchString(name) {
			t.Errorf("expected %q to be accepted", name)
		}
	}
	bad := []string{
		"evil.db",
		"bindery_../x.db",
		"bindery_a/b.db",
		"bindery_20260726_181731.db.bak",
		"../bindery_20260726_181731.db",
		"bindery_20260726_181731.sqlite",
	}
	for _, name := range bad {
		if backupFilenameRe.MatchString(name) {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}
