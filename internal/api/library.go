package api

import (
	"net/http"

	"github.com/vavallee/bindery/internal/importer"
)

type LibraryHandler struct {
	scanner *importer.Scanner
}

func NewLibraryHandler(scanner *importer.Scanner) *LibraryHandler {
	return &LibraryHandler{scanner: scanner}
}

// Scan triggers an immediate library reconciliation in the background and
// returns 202 Accepted. The scan runs asynchronously; clients can monitor
// progress via the book list.
func (h *LibraryHandler) Scan(w http.ResponseWriter, r *http.Request) {
	go h.scanner.ScanLibrary(r.Context())
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "library scan started"})
}
