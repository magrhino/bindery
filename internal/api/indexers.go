package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/indexer"
	"github.com/vavallee/bindery/internal/indexer/newznab"
	"github.com/vavallee/bindery/internal/models"
)

type IndexerHandler struct {
	indexers *db.IndexerRepo
	books    *db.BookRepo
	authors  *db.AuthorRepo
	searcher *indexer.Searcher
	settings *db.SettingsRepo
}

func NewIndexerHandler(indexers *db.IndexerRepo, books *db.BookRepo, authors *db.AuthorRepo, searcher *indexer.Searcher, settings *db.SettingsRepo) *IndexerHandler {
	return &IndexerHandler{indexers: indexers, books: books, authors: authors, searcher: searcher, settings: settings}
}

func (h *IndexerHandler) List(w http.ResponseWriter, r *http.Request) {
	idxs, err := h.indexers.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if idxs == nil {
		idxs = []models.Indexer{}
	}
	writeJSON(w, http.StatusOK, idxs)
}

func (h *IndexerHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	idx, err := h.indexers.GetByID(r.Context(), id)
	if err != nil || idx == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "indexer not found"})
		return
	}
	writeJSON(w, http.StatusOK, idx)
}

func (h *IndexerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var idx models.Indexer
	if err := json.NewDecoder(r.Body).Decode(&idx); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if idx.Name == "" || idx.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url required"})
		return
	}
	if idx.Type == "" {
		idx.Type = "newznab"
	}
	if len(idx.Categories) == 0 {
		idx.Categories = []int{7000, 7020}
	}

	// Check for duplicate URL
	existing, _ := h.indexers.List(r.Context())
	for _, e := range existing {
		if e.URL == idx.URL {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "indexer with this URL already exists"})
			return
		}
	}

	if err := h.indexers.Create(r.Context(), &idx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, idx)
}

func (h *IndexerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	existing, err := h.indexers.GetByID(r.Context(), id)
	if err != nil || existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "indexer not found"})
		return
	}

	var idx models.Indexer
	if err := json.NewDecoder(r.Body).Decode(&idx); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	idx.ID = id
	if err := h.indexers.Update(r.Context(), &idx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, idx)
}

func (h *IndexerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.indexers.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IndexerHandler) Test(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	idx, err := h.indexers.GetByID(r.Context(), id)
	if err != nil || idx == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "indexer not found"})
		return
	}

	client := newznab.New(idx.URL, idx.APIKey)
	if err := client.Test(r.Context()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// SearchBook searches all enabled indexers for a specific book.
func (h *IndexerHandler) SearchBook(w http.ResponseWriter, r *http.Request) {
	bookID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	book, err := h.books.GetByID(r.Context(), bookID)
	if err != nil || book == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "book not found"})
		return
	}

	idxs, err := h.indexers.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Resolve author name for better search results
	authorName := ""
	if author, err := h.authors.GetByID(r.Context(), book.AuthorID); err == nil && author != nil {
		authorName = author.Name
	}

	results := h.searcher.SearchBook(r.Context(), idxs, book.Title, authorName)

	// Apply language filter
	lang := "en"
	if s, _ := h.settings.Get(r.Context(), "search.preferredLanguage"); s != nil {
		lang = s.Value
	}
	results = indexer.FilterByLanguage(results, lang)

	writeJSON(w, http.StatusOK, results)
}

// SearchQuery performs a freeform search across all indexers.
func (h *IndexerHandler) SearchQuery(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q parameter required"})
		return
	}

	idxs, err := h.indexers.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	results := h.searcher.SearchQuery(r.Context(), idxs, query)
	writeJSON(w, http.StatusOK, results)
}
