package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

type QualityProfileHandler struct {
	profiles *db.QualityProfileRepo
}

func NewQualityProfileHandler(profiles *db.QualityProfileRepo) *QualityProfileHandler {
	return &QualityProfileHandler{profiles: profiles}
}

func (h *QualityProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.profiles.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if profiles == nil {
		profiles = []models.QualityProfile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (h *QualityProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	p, err := h.profiles.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "quality profile not found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}
