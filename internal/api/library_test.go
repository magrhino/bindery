package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/importer"
)

// fakeScanner records the context it received so the test can inspect it.
type fakeScanner struct {
	called chan context.Context
}

func (f *fakeScanner) ScanLibrary(ctx context.Context) { f.called <- ctx }

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

// TestLibraryScan_ContextOutlivesRequest is a regression test for issue #55.
// It verifies that cancelling the HTTP request context (which happens the
// instant the 202 response is written) does not cancel the scan goroutine.
func TestLibraryScan_ContextOutlivesRequest(t *testing.T) {
	fake := &fakeScanner{called: make(chan context.Context, 1)}
	h := &LibraryHandler{scanner: fake}

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/library/scan", nil).WithContext(reqCtx)
	h.Scan(httptest.NewRecorder(), req)

	// Simulate the net/http server cancelling the request context after the
	// handler returns (the 202 has been flushed to the client).
	cancel()

	// Wait for the goroutine to call ScanLibrary and deliver its context.
	scanCtx := <-fake.called

	// The scan context must still be live even though the request context was
	// cancelled.
	select {
	case <-scanCtx.Done():
		t.Fatal("scan context was cancelled when the request context was cancelled (issue #55 regression)")
	default:
		// pass — context.WithoutCancel keeps the scan alive
	}
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
