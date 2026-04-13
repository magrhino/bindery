package api

import (
	"io"
	"net/http"
	"os"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/migrate"
	"github.com/vavallee/bindery/internal/models"
)

// MigrateHandler exposes bulk-import endpoints under /api/v1/migrate.
type MigrateHandler struct {
	authors   *db.AuthorRepo
	indexers  *db.IndexerRepo
	clients   *db.DownloadClientRepo
	blocklist *db.BlocklistRepo
	books     *db.BookRepo
	meta      *metadata.Aggregator

	// onNewAuthor fires in a goroutine for each newly-imported author so
	// the book-fetch-on-add behaviour from the AddAuthor flow is preserved.
	onNewAuthor func(author *models.Author)
}

func NewMigrateHandler(
	authors *db.AuthorRepo,
	indexers *db.IndexerRepo,
	clients *db.DownloadClientRepo,
	blocklist *db.BlocklistRepo,
	books *db.BookRepo,
	meta *metadata.Aggregator,
	onNewAuthor func(author *models.Author),
) *MigrateHandler {
	return &MigrateHandler{
		authors: authors, indexers: indexers, clients: clients,
		blocklist: blocklist, books: books, meta: meta,
		onNewAuthor: onNewAuthor,
	}
}

// ImportCSV accepts a multipart form with a "file" field containing either
// a newline-separated list of author names or a CSV (name[,monitored
// [,searchOnAdd]]). Top OpenLibrary match is chosen for each name.
func (h *MigrateHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	file, err := acceptUpload(w, r, 5<<20) // 5 MB cap — CSV of names is tiny
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	defer file.Close()

	result, err := migrate.ImportCSVAuthors(r.Context(), file, h.authors, h.meta, h.onNewAuthor)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ImportReadarr accepts a multipart form with a "file" field containing
// readarr.db (SQLite). It's saved to a temp file (sqlite driver wants a
// path), imported, then deleted.
func (h *MigrateHandler) ImportReadarr(w http.ResponseWriter, r *http.Request) {
	file, err := acceptUpload(w, r, 1<<30) // 1 GB cap — readarr.db is usually < 100 MB
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "readarr-*.db")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create temp: " + err.Error()})
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write temp: " + err.Error()})
		return
	}
	tmp.Close()

	result, err := migrate.ImportReadarr(
		r.Context(), tmpPath,
		h.authors, h.indexers, h.clients, h.blocklist, h.meta, h.onNewAuthor,
	)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// acceptUpload reads a multipart "file" field with a size cap, returning
// the file reader. Caller must Close. Passing w to MaxBytesReader makes the
// server respond 413 Request Entity Too Large automatically when the body
// exceeds maxBytes, instead of the handler seeing a generic parse error.
func acceptUpload(w http.ResponseWriter, r *http.Request, maxBytes int64) (io.ReadCloser, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, err
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		return nil, err
	}
	return f, nil
}
