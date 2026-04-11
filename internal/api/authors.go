package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
)

type AuthorHandler struct {
	authors *db.AuthorRepo
	books   *db.BookRepo
	meta    *metadata.Aggregator
}

func NewAuthorHandler(authors *db.AuthorRepo, books *db.BookRepo, meta *metadata.Aggregator) *AuthorHandler {
	return &AuthorHandler{authors: authors, books: books, meta: meta}
}

func (h *AuthorHandler) List(w http.ResponseWriter, r *http.Request) {
	authors, err := h.authors.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if authors == nil {
		authors = []models.Author{}
	}
	writeJSON(w, http.StatusOK, authors)
}

func (h *AuthorHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	author, err := h.authors.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if author == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "author not found"})
		return
	}

	// Attach books
	books, err := h.books.ListByAuthor(r.Context(), id)
	if err == nil {
		author.Books = books
	}

	writeJSON(w, http.StatusOK, author)
}

func (h *AuthorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ForeignID        string `json:"foreignAuthorId"`
		Name             string `json:"authorName"`
		QualityProfileID *int64 `json:"qualityProfileId"`
		RootFolderID     *int64 `json:"rootFolderId"`
		Monitored        bool   `json:"monitored"`
		SearchOnAdd      bool   `json:"searchOnAdd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ForeignID == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "foreignAuthorId and authorName required"})
		return
	}

	// Check if already exists
	existing, _ := h.authors.GetByForeignID(r.Context(), req.ForeignID)
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "author already exists"})
		return
	}

	// Fetch full author metadata
	author, err := h.meta.GetAuthor(r.Context(), req.ForeignID)
	if err != nil {
		slog.Warn("metadata lookup failed, using provided name", "error", err)
		author = &models.Author{
			ForeignID:        req.ForeignID,
			Name:             req.Name,
			SortName:         sortName(req.Name),
			MetadataProvider: "openlibrary",
		}
	}
	author.Monitored = req.Monitored
	author.QualityProfileID = req.QualityProfileID
	author.RootFolderID = req.RootFolderID

	if err := h.authors.Create(r.Context(), author); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Fetch and store books for this author
	if req.SearchOnAdd {
		go h.fetchAuthorBooks(author)
	}

	writeJSON(w, http.StatusCreated, author)
}

func (h *AuthorHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	author, err := h.authors.GetByID(r.Context(), id)
	if err != nil || author == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "author not found"})
		return
	}

	var req struct {
		Monitored        *bool  `json:"monitored"`
		QualityProfileID *int64 `json:"qualityProfileId"`
		RootFolderID     *int64 `json:"rootFolderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Monitored != nil {
		author.Monitored = *req.Monitored
	}
	if req.QualityProfileID != nil {
		author.QualityProfileID = req.QualityProfileID
	}
	if req.RootFolderID != nil {
		author.RootFolderID = req.RootFolderID
	}

	if err := h.authors.Update(r.Context(), author); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, author)
}

func (h *AuthorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.authors.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthorHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	author, err := h.authors.GetByID(r.Context(), id)
	if err != nil || author == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "author not found"})
		return
	}

	go h.fetchAuthorBooks(author)
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "refresh started"})
}

func (h *AuthorHandler) fetchAuthorBooks(author *models.Author) {
	ctx := contextBackground()
	slog.Info("fetching books for author", "author", author.Name, "foreignId", author.ForeignID)

	books, err := h.meta.SearchBooks(ctx, author.Name)
	if err != nil {
		slog.Error("failed to fetch books", "author", author.Name, "error", err)
		return
	}

	// Track titles we've already added (case-insensitive) to avoid OL duplicates
	existingBooks, _ := h.books.ListByAuthor(ctx, author.ID)
	seenTitles := make(map[string]bool)
	for _, eb := range existingBooks {
		seenTitles[strings.ToLower(eb.Title)] = true
	}

	var added int
	for _, b := range books {
		// Only add books by this author
		if b.Author == nil || b.Author.ForeignID != author.ForeignID {
			continue
		}
		b.AuthorID = author.ID
		b.Monitored = author.Monitored

		// Skip if foreign ID already exists
		existing, _ := h.books.GetByForeignID(ctx, b.ForeignID)
		if existing != nil {
			continue
		}

		// Skip duplicate titles (OpenLibrary often has multiple works for the same book)
		normalizedTitle := strings.ToLower(b.Title)
		if seenTitles[normalizedTitle] {
			continue
		}
		seenTitles[normalizedTitle] = true

		if err := h.books.Create(ctx, &b); err != nil {
			slog.Warn("failed to create book", "title", b.Title, "error", err)
			continue
		}
		added++
	}
	slog.Info("author books synced", "author", author.Name, "added", added, "total", len(books))
}
