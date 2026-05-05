package abscontract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/vavallee/bindery/internal/abs"
	"github.com/vavallee/bindery/internal/db"
)

type fixtureABSHarness struct {
	t              *testing.T
	cfg            HarnessConfig
	manifest       *FixtureManifest
	server         *httptest.Server
	libraries      []abs.Library
	items          []abs.LibraryItem
	detailByItemID map[string]abs.LibraryItem
	pageSize       int

	mu          sync.Mutex
	failPageFor map[int]int
}

func newFixtureABSHarness(t *testing.T, cfg HarnessConfig, manifest *FixtureManifest) *fixtureABSHarness {
	t.Helper()

	h := &fixtureABSHarness{
		t:              t,
		cfg:            cfg,
		manifest:       manifest,
		libraries:      loadHarnessLibraries(t),
		items:          loadHarnessItems(t),
		detailByItemID: loadHarnessDetails(t),
		pageSize:       2,
		failPageFor:    map[int]int{},
	}
	h.server = httptest.NewServer(http.HandlerFunc(h.serveHTTP))
	t.Cleanup(h.server.Close)
	return h
}

func (h *fixtureABSHarness) BaseURL() string {
	return h.server.URL
}

func (h *fixtureABSHarness) FailPage(page, attempts int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failPageFor[page] = attempts
}

func (h *fixtureABSHarness) serveHTTP(w http.ResponseWriter, r *http.Request) {
	apiKey, ok := h.authorizeRequest(r)
	if !ok {
		writeHarnessJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/authorize":
		writeHarnessJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{
				"id":                  "fixture-user",
				"username":            h.usernameForAPIKey(apiKey),
				"type":                "admin",
				"librariesAccessible": h.accessibleLibraryIDs(apiKey),
				"permissions": map[string]bool{
					"accessAllLibraries": apiKey == h.cfg.APIKey,
				},
			},
			"userDefaultLibraryId": h.cfg.LibraryID,
			"serverSettings": map[string]string{
				"version": h.cfg.Baseline.Version,
			},
			"Source": "fixture-harness",
		})
		return

	case r.Method == http.MethodGet && r.URL.Path == "/api/libraries":
		writeHarnessJSON(w, http.StatusOK, map[string]any{
			"libraries": h.visibleLibraries(apiKey),
		})
		return

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/libraries/") && strings.HasSuffix(r.URL.Path, "/items"):
		libraryID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/libraries/"), "/items")
		h.serveLibraryItems(w, r, apiKey, libraryID)
		return

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/libraries/"):
		libraryID := strings.TrimPrefix(r.URL.Path, "/api/libraries/")
		libraryID = strings.Trim(libraryID, "/")
		if !h.canAccessLibrary(apiKey, libraryID) {
			writeHarnessJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		for _, library := range h.libraries {
			if library.ID == libraryID {
				writeHarnessJSON(w, http.StatusOK, library)
				return
			}
		}
		writeHarnessJSON(w, http.StatusNotFound, map[string]string{"error": "library not found"})
		return

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/items/"):
		itemID := strings.TrimPrefix(r.URL.Path, "/api/items/")
		itemID = strings.Trim(itemID, "/")
		if detail, ok := h.detailByItemID[itemID]; ok {
			writeHarnessJSON(w, http.StatusOK, detail)
			return
		}
		for _, item := range h.items {
			if item.ID == itemID {
				writeHarnessJSON(w, http.StatusOK, item)
				return
			}
		}
		writeHarnessJSON(w, http.StatusNotFound, map[string]string{"error": "item not found"})
		return
	}

	writeHarnessJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (h *fixtureABSHarness) serveLibraryItems(w http.ResponseWriter, r *http.Request, apiKey, libraryID string) {
	if !h.canAccessLibrary(apiKey, libraryID) {
		writeHarnessJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if libraryID != h.cfg.LibraryID {
		writeHarnessJSON(w, http.StatusNotFound, map[string]string{"error": "library items not configured"})
		return
	}

	page := parseIntDefault(r.URL.Query().Get("page"), 0)
	h.mu.Lock()
	if remaining := h.failPageFor[page]; remaining > 0 {
		if remaining == 1 {
			delete(h.failPageFor, page)
		} else {
			h.failPageFor[page] = remaining - 1
		}
		h.mu.Unlock()
		writeHarnessJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("fixture page %d failed", page)})
		return
	}
	h.mu.Unlock()

	start := page * h.pageSize
	if start > len(h.items) {
		start = len(h.items)
	}
	end := start + h.pageSize
	if end > len(h.items) {
		end = len(h.items)
	}
	results := []abs.LibraryItem{}
	if start < len(h.items) {
		results = append(results, h.items[start:end]...)
	}
	writeHarnessJSON(w, http.StatusOK, abs.LibraryItemsPage{
		Results:   results,
		Total:     len(h.items),
		Limit:     h.pageSize,
		Page:      page,
		MediaType: "book",
		Minified:  true,
	})
}

func (h *fixtureABSHarness) authorizeRequest(r *http.Request) (string, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	switch token {
	case h.cfg.APIKey, h.cfg.LimitedAPIKey:
		return token, true
	default:
		return "", false
	}
}

func (h *fixtureABSHarness) usernameForAPIKey(apiKey string) string {
	if apiKey == h.cfg.LimitedAPIKey {
		return "fixture-limited"
	}
	return "fixture-admin"
}

func (h *fixtureABSHarness) visibleLibraries(apiKey string) []abs.Library {
	if apiKey == h.cfg.LimitedAPIKey {
		out := make([]abs.Library, 0, 1)
		for _, library := range h.libraries {
			if library.ID == h.cfg.LibraryID {
				out = append(out, library)
			}
		}
		return out
	}
	return append([]abs.Library(nil), h.libraries...)
}

func (h *fixtureABSHarness) accessibleLibraryIDs(apiKey string) []string {
	libraries := h.visibleLibraries(apiKey)
	out := make([]string, 0, len(libraries))
	for _, library := range libraries {
		out = append(out, library.ID)
	}
	return out
}

func (h *fixtureABSHarness) canAccessLibrary(apiKey, libraryID string) bool {
	for _, id := range h.accessibleLibraryIDs(apiKey) {
		if id == libraryID {
			return true
		}
	}
	return false
}

func newContractImporterFixture(t *testing.T) (*abs.Importer, *db.AuthorRepo, *db.BookRepo, *db.SettingsRepo, *db.ABSImportRunRepo) {
	t.Helper()

	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	authorRepo := db.NewAuthorRepo(database)
	bookRepo := db.NewBookRepo(database)
	settingsRepo := db.NewSettingsRepo(database)
	runRepo := db.NewABSImportRunRepo(database)
	importer := abs.NewImporter(
		authorRepo,
		db.NewAuthorAliasRepo(database),
		bookRepo,
		db.NewEditionRepo(database),
		db.NewSeriesRepo(database),
		settingsRepo,
		runRepo,
		db.NewABSImportRunEntityRepo(database),
		db.NewABSProvenanceRepo(database),
		db.NewABSReviewItemRepo(database),
		db.NewABSMetadataConflictRepo(database),
	).WithStoragePaths(t.TempDir(), t.TempDir(), nil)
	return importer, authorRepo, bookRepo, settingsRepo, runRepo
}

func loadHarnessLibraries(t *testing.T) []abs.Library {
	t.Helper()

	var payload struct {
		Libraries []abs.Library `json:"libraries"`
	}
	loadJSONFixture(t, repoFixturePath("internal", "abs", "testdata", "libraries_v2_33_2.json"), &payload)
	return payload.Libraries
}

func loadHarnessItems(t *testing.T) []abs.LibraryItem {
	t.Helper()

	items := []abs.LibraryItem{
		loadSingleListItem(t, repoFixturePath("internal", "abs", "testdata", "library_items_single_file_page_v2_33_2.json")),
		loadSingleListItem(t, repoFixturePath("internal", "abs", "testdata", "library_items_folder_multi_file_page_v2_33_2.json")),
		loadSingleListItem(t, repoFixturePath("internal", "abs", "testdata", "library_items_missing_size_duration_page_v2_33_2.json")),
		loadLibraryItemFixture(t, filepath.Join("testdata", "fixtures", "responses", "library_item_ebook_only_v2_33_2.json")),
		loadLibraryItemFixture(t, filepath.Join("testdata", "fixtures", "responses", "library_item_series_linked_v2_33_2.json")),
	}
	return items
}

func loadHarnessDetails(t *testing.T) map[string]abs.LibraryItem {
	t.Helper()

	return map[string]abs.LibraryItem{
		"li_folder_multi":  loadLibraryItemFixture(t, repoFixturePath("internal", "abs", "testdata", "library_item_folder_multi_file_detail_v2_33_2.json")),
		"li_missing_stats": loadLibraryItemFixture(t, repoFixturePath("internal", "abs", "testdata", "library_item_missing_size_duration_detail_v2_33_2.json")),
	}
}

func loadSingleListItem(t *testing.T, path string) abs.LibraryItem {
	t.Helper()

	var page abs.LibraryItemsPage
	loadJSONFixture(t, path, &page)
	if len(page.Results) != 1 {
		t.Fatalf("fixture %s yielded %d results, want 1", path, len(page.Results))
	}
	return page.Results[0]
}

func loadLibraryItemFixture(t *testing.T, path string) abs.LibraryItem {
	t.Helper()

	var item abs.LibraryItem
	loadJSONFixture(t, path, &item)
	return item
}

func loadJSONFixture(t *testing.T, path string, out any) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode fixture %s: %v", path, err)
	}
}

func repoFixturePath(parts ...string) string {
	all := append([]string{"..", ".."}, parts...)
	return filepath.Join(all...)
}

func parseIntDefault(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func writeHarnessJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
