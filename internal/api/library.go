package api

import (
	"context"
	"net/http"

	"github.com/vavallee/bindery/internal/importer"
)

// libraryScanner is the subset of importer.Scanner used by LibraryHandler.
type libraryScanner interface {
	ScanLibrary(ctx context.Context)
}

type LibraryHandler struct {
	scanner libraryScanner
}

func NewLibraryHandler(scanner *importer.Scanner) *LibraryHandler {
	return &LibraryHandler{scanner: scanner}
}

// Scan triggers an immediate library reconciliation in the background and
// returns 202 Accepted. The scan runs asynchronously; clients can monitor
// progress via the book list.
func (h *LibraryHandler) Scan(w http.ResponseWriter, r *http.Request) {
	// context.WithoutCancel so the goroutine isn't killed when the HTTP
	// response is sent and the request context is cancelled.
	go h.scanner.ScanLibrary(context.WithoutCancel(r.Context()))
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "library scan started"})
}
