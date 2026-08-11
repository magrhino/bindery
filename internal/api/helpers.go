package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/importer"
	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/textutil"
)

// BookSearcher triggers an immediate indexer search and auto-grab for a
// single wanted book. Implemented by *scheduler.Scheduler.
type BookSearcher interface {
	SearchAndGrabBook(ctx context.Context, book models.Book)
}

// BookMetaLookup fetches a single book record from a named provider,
// bypassing the TTL cache. Implemented by *metadata.Aggregator; kept as an
// interface so the Rebind handler can be tested without a real HTTP client.
type BookMetaLookup interface {
	GetBookFromProvider(ctx context.Context, provider, foreignID string) (*models.Book, error)
}

// LibraryFinder checks whether a book already exists in the local library.
// Implemented by *importer.Scanner; a nil implementation is a no-op. The
// mediaType argument selects which library roots are searched (ebook vs
// audiobook vs both) so a same-titled file in the wrong root cannot be
// mis-attributed to a book of the opposite media type.
type LibraryFinder interface {
	FindExisting(ctx context.Context, title, authorName, mediaType string) string
}

// librarySnapshotter is the optional capability behind LibraryFinder: a finder
// that can hand out a snapshot answering many FindExisting queries from one
// walk of each library root. The author sync asserts for it before its create
// loop, because that loop calls FindExisting once per new book and each call
// used to re-walk the entire library — 65 full walks for a 65-book author,
// which on network storage is the right order of magnitude for the reported
// hour-long refresh (#1888, #1929). Implemented by *importer.Scanner; a finder
// without the capability keeps its per-call behaviour.
type librarySnapshotter interface {
	SnapshotFinder() *importer.LibrarySnapshot
}

// snapshotFinder returns a batch-friendly view of finder when it offers one,
// and finder itself otherwise (including nil, which callers already treat as a
// no-op).
func snapshotFinder(finder LibraryFinder) LibraryFinder {
	if sn, ok := finder.(librarySnapshotter); ok {
		return sn.SnapshotFinder()
	}
	return finder
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

func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit := defaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	if limit <= 0 {
		limit = defaultLimit
	}

	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

func sortName(name string) string {
	return textutil.SortName(name)
}
