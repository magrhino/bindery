package api

import (
	"encoding/json"
	"net/http"

	"github.com/vavallee/bindery/internal/metadata"
)

type SearchHandler struct {
	meta *metadata.Aggregator
}

func NewSearchHandler(meta *metadata.Aggregator) *SearchHandler {
	return &SearchHandler{meta: meta}
}

func (h *SearchHandler) SearchAuthors(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "term parameter required"})
		return
	}

	authors, err := h.meta.SearchAuthors(r.Context(), term)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, authors)
}

func (h *SearchHandler) SearchBooks(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "term parameter required"})
		return
	}

	books, err := h.meta.SearchBooks(r.Context(), term)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, books)
}

func (h *SearchHandler) LookupByISBN(w http.ResponseWriter, r *http.Request) {
	isbn := r.URL.Query().Get("isbn")
	if isbn == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "isbn parameter required"})
		return
	}

	book, err := h.meta.GetBookByISBN(r.Context(), isbn)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if book == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no book found for ISBN"})
		return
	}

	writeJSON(w, http.StatusOK, book)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
