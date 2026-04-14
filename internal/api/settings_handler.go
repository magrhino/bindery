package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

type SettingsHandler struct {
	settings *db.SettingsRepo
}

func NewSettingsHandler(settings *db.SettingsRepo) *SettingsHandler {
	return &SettingsHandler{settings: settings}
}

// isSecretSetting reports whether a settings key holds sensitive material
// that must not leak through the generic settings endpoints. The auth.*
// values are surfaced through the dedicated /auth/* endpoints instead.
func isSecretSetting(key string) bool {
	switch key {
	case "auth.api_key", "auth.session_secret", "auth.mode":
		return true
	}
	return false
}

func (h *SettingsHandler) List(w http.ResponseWriter, r *http.Request) {
	settings, err := h.settings.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	filtered := make([]models.Setting, 0, len(settings))
	for _, s := range settings {
		if !isSecretSetting(s.Key) {
			filtered = append(filtered, s)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if isSecretSetting(key) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "setting not found"})
		return
	}
	s, err := h.settings.Get(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if s == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "setting not found"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *SettingsHandler) Set(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if isSecretSetting(key) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "use /auth/* endpoints for auth settings"})
		return
	}
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.settings.Set(r.Context(), key, req.Value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s, err := h.settings.Get(r.Context(), key)
	if err != nil || s == nil {
		writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": req.Value})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *SettingsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if isSecretSetting(key) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "use /auth/* endpoints for auth settings"})
		return
	}
	if err := h.settings.Delete(r.Context(), key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
