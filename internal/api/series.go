package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

type SeriesHandler struct {
	series *db.SeriesRepo
}

func NewSeriesHandler(series *db.SeriesRepo) *SeriesHandler {
	return &SeriesHandler{series: series}
}

func (h *SeriesHandler) List(w http.ResponseWriter, r *http.Request) {
	series, err := h.series.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if series == nil {
		series = []models.Series{}
	}
	writeJSON(w, http.StatusOK, series)
}

func (h *SeriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	s, err := h.series.GetByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if s == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "series not found"})
		return
	}
	if s.Books == nil {
		s.Books = []models.SeriesBook{}
	}
	writeJSON(w, http.StatusOK, s)
}
