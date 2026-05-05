package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata/hardcover"
	"github.com/vavallee/bindery/internal/models"
)

type ImportListHandler struct {
	repo *db.ImportListRepo
}

func NewImportListHandler(repo *db.ImportListRepo) *ImportListHandler {
	return &ImportListHandler{repo: repo}
}

func (h *ImportListHandler) List(w http.ResponseWriter, r *http.Request) {
	lists, err := h.repo.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if lists == nil {
		lists = []models.ImportList{}
	}
	writeJSON(w, http.StatusOK, lists)
}

func (h *ImportListHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	il, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if il == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "import list not found"})
		return
	}
	writeJSON(w, http.StatusOK, il)
}

func (h *ImportListHandler) Create(w http.ResponseWriter, r *http.Request) {
	var il models.ImportList
	if err := json.NewDecoder(r.Body).Decode(&il); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if il.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	if il.Type == "" {
		il.Type = "csv"
	}
	if err := h.repo.Create(r.Context(), &il); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, il)
}

func (h *ImportListHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	existing, err := h.repo.GetByID(r.Context(), id)
	if err != nil || existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "import list not found"})
		return
	}
	var il models.ImportList
	if err := json.NewDecoder(r.Body).Decode(&il); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	il.ID = id
	if err := h.repo.Update(r.Context(), &il); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, il)
}

func (h *ImportListHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HardcoverLists returns the authenticated user's Hardcover reading lists.
// GET /api/v1/importlist/hardcover/lists  (Authorization: Bearer <tok>)
// The Hardcover client normalizes the token before forwarding it upstream.
func (h *ImportListHandler) HardcoverLists(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	token := hardcover.NormalizeAPIToken(authHeader)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing authorization header"})
		return
	}
	client := hardcover.NewAuthenticated(token)
	lists, err := client.GetUserLists(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

// --- Exclusions ---

func (h *ImportListHandler) ListExclusions(w http.ResponseWriter, r *http.Request) {
	exclusions, err := h.repo.ListExclusions(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if exclusions == nil {
		exclusions = []models.ImportListExclusion{}
	}
	writeJSON(w, http.StatusOK, exclusions)
}

func (h *ImportListHandler) CreateExclusion(w http.ResponseWriter, r *http.Request) {
	var e models.ImportListExclusion
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if e.ForeignID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "foreignId required"})
		return
	}
	if err := h.repo.CreateExclusion(r.Context(), &e); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *ImportListHandler) DeleteExclusion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteExclusion(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
