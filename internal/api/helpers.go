package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func contextBackground() context.Context {
	return context.Background()
}

// parseID extracts the `{id}` URL parameter as an int64. If the value is
// missing or non-numeric it writes HTTP 400 and returns (0, false). Callers
// should check ok and bail out on false.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

func sortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return name
	}
	last := parts[len(parts)-1]
	rest := strings.Join(parts[:len(parts)-1], " ")
	return last + ", " + rest
}
