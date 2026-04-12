package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

type HistoryHandler struct {
	history *db.HistoryRepo
}

func NewHistoryHandler(history *db.HistoryRepo) *HistoryHandler {
	return &HistoryHandler{history: history}
}

func (h *HistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	var events []models.HistoryEvent
	var err error

	bookIDStr := r.URL.Query().Get("bookId")
	eventType := r.URL.Query().Get("eventType")

	switch {
	case bookIDStr != "":
		id, _ := strconv.ParseInt(bookIDStr, 10, 64)
		events, err = h.history.ListByBook(r.Context(), id)
	case eventType != "":
		events, err = h.history.ListByType(r.Context(), eventType)
	default:
		events, err = h.history.List(r.Context())
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if events == nil {
		events = []models.HistoryEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *HistoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := h.history.Delete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
