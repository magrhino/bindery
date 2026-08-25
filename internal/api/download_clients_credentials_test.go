package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/downloader"
	"github.com/vavallee/bindery/internal/models"
)

// seedDownloadClient inserts a row straight through the repo so the test
// controls the stored credentials exactly, without going through Create.
func seedDownloadClient(t *testing.T, clients *db.DownloadClientRepo, c models.DownloadClient) models.DownloadClient {
	t.Helper()
	if err := clients.Create(context.Background(), &c); err != nil {
		t.Fatalf("seed download client: %v", err)
	}
	return c
}

// updateDownloadClient runs the Update handler against the given body and
// returns the recorder so the caller can assert on status and payload.
func updateDownloadClient(t *testing.T, h *DownloadClientHandler, id int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	idStr := strconv.FormatInt(id, 10)
	rec := httptest.NewRecorder()
	req := withURLParam(httptest.NewRequest(http.MethodPut, "/downloadclient/"+idStr, bytes.NewBufferString(body)), "id", idStr)
	h.Update(rec, req)
	return rec
}

func decodeDownloadClient(t *testing.T, rec *httptest.ResponseRecorder) models.DownloadClient {
	t.Helper()
	var out models.DownloadClient
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestDownloadClientList_OmitsCredentials(t *testing.T) {
	h, clients := downloadClientFixture(t)
	seedDownloadClient(t, clients, models.DownloadClient{
		Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
	})
	seedDownloadClient(t, clients, models.DownloadClient{
		Name: "qBit", Type: "qbittorrent", Host: "10.10.10.11", Port: 8080, Username: "admin", Password: "qbit-pass", Category: "books", Enabled: true,
	})

	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/downloadclient", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); bytes.Contains([]byte(body), []byte("sab-key")) || bytes.Contains([]byte(body), []byte("qbit-pass")) {
		t.Fatalf("list response leaked a credential: %s", body)
	}

	var list []models.DownloadClient
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(list))
	}
	byName := map[string]models.DownloadClient{}
	for _, c := range list {
		byName[c.Name] = c
	}
	sab := byName["SAB"]
	if sab.APIKey != "" || sab.Password != "" {
		t.Errorf("sabnzbd credentials not blanked: apiKey=%q password=%q", sab.APIKey, sab.Password)
	}
	if !sab.APIKeyConfigured {
		t.Error("sabnzbd apiKeyConfigured: want true")
	}
	if sab.PasswordConfigured {
		t.Error("sabnzbd passwordConfigured: want false")
	}
	qbit := byName["qBit"]
	if qbit.APIKey != "" || qbit.Password != "" {
		t.Errorf("qbittorrent credentials not blanked: apiKey=%q password=%q", qbit.APIKey, qbit.Password)
	}
	if !qbit.PasswordConfigured {
		t.Error("qbittorrent passwordConfigured: want true")
	}
	if qbit.Username != "admin" {
		t.Errorf("username should still be returned, got %q", qbit.Username)
	}
}

func TestDownloadClientGet_OmitsCredentials(t *testing.T) {
	h, clients := downloadClientFixture(t)
	seeded := seedDownloadClient(t, clients, models.DownloadClient{
		Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
	})

	idStr := strconv.FormatInt(seeded.ID, 10)
	rec := httptest.NewRecorder()
	h.Get(rec, withURLParam(httptest.NewRequest(http.MethodGet, "/downloadclient/"+idStr, nil), "id", idStr))
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sab-key")) {
		t.Fatalf("get response leaked the api key: %s", rec.Body.String())
	}
	got := decodeDownloadClient(t, rec)
	if got.APIKey != "" {
		t.Errorf("apiKey = %q, want empty", got.APIKey)
	}
	if !got.APIKeyConfigured {
		t.Error("apiKeyConfigured: want true")
	}
	if got.PasswordConfigured {
		t.Error("passwordConfigured: want false")
	}
}

func TestDownloadClientCreate_OmitsCredentialsInResponse(t *testing.T) {
	h, _ := downloadClientFixture(t)
	body := `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","apiKey":"created-key","enabled":true}`
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/downloadclient", bytes.NewBufferString(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("created-key")) {
		t.Fatalf("create response leaked the api key: %s", rec.Body.String())
	}
	got := decodeDownloadClient(t, rec)
	if !got.APIKeyConfigured {
		t.Error("apiKeyConfigured: want true")
	}
}

func TestDownloadClientUpdate_BlankCredentialKeepsStored(t *testing.T) {
	ctx := context.Background()

	t.Run("omitted apiKey", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB renamed","host":"10.10.10.10","type":"sabnzbd","category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.APIKey != "sab-key" {
			t.Errorf("stored apiKey = %q, want it kept", stored.APIKey)
		}
		if stored.Name != "SAB renamed" {
			t.Errorf("stored name = %q", stored.Name)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("sab-key")) {
			t.Errorf("update response leaked the api key: %s", rec.Body.String())
		}
	})

	t.Run("explicitly empty apiKey", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","apiKey":"","category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.APIKey != "sab-key" {
			t.Errorf("stored apiKey = %q, want it kept", stored.APIKey)
		}
	})

	t.Run("omitted password", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "qBit", Type: "qbittorrent", Host: "10.10.10.11", Port: 8080, Username: "admin", Password: "qbit-pass", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"qBit","host":"10.10.10.11","type":"qbittorrent","username":"admin","password":"","category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.Password != "qbit-pass" {
			t.Errorf("stored password = %q, want it kept", stored.Password)
		}
	})

	t.Run("new value replaces stored", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","apiKey":"rotated","category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.APIKey != "rotated" {
			t.Errorf("stored apiKey = %q, want rotated", stored.APIKey)
		}
	})
}

func TestDownloadClientUpdate_ClearFlags(t *testing.T) {
	ctx := context.Background()

	t.Run("clearApiKey", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","clearApiKey":true,"category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.APIKey != "" {
			t.Errorf("stored apiKey = %q, want cleared", stored.APIKey)
		}
		got := decodeDownloadClient(t, rec)
		if got.APIKeyConfigured {
			t.Error("apiKeyConfigured: want false after a clear")
		}
	})

	t.Run("clearPassword", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "NZBGet", Type: "nzbget", Host: "10.10.10.12", Port: 6789, Username: "nzbget", Password: "nzb-pass", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"NZBGet","host":"10.10.10.12","type":"nzbget","username":"nzbget","clearPassword":true,"category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.Password != "" {
			t.Errorf("stored password = %q, want cleared", stored.Password)
		}
		got := decodeDownloadClient(t, rec)
		if got.PasswordConfigured {
			t.Error("passwordConfigured: want false after a clear")
		}
	})

	t.Run("clearPassword drops the legacy api_key mirror", func(t *testing.T) {
		// A pre-#423 qBittorrent row kept the password in the api_key column.
		// The repo mirrors it back into Password on read, so a clear that left
		// api_key alone would be undone on the very next write.
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "qBit", Type: "qbittorrent", Host: "10.10.10.11", Port: 8080, Username: "admin", APIKey: "legacy-pass", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"qBit","host":"10.10.10.11","type":"qbittorrent","username":"admin","clearPassword":true,"category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.Password != "" || stored.APIKey != "" {
			t.Errorf("legacy mirror survived the clear: apiKey=%q password=%q", stored.APIKey, stored.Password)
		}
	})

	t.Run("a real api key survives clearPassword", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","clearPassword":true,"category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.APIKey != "sab-key" {
			t.Errorf("stored apiKey = %q, want it kept", stored.APIKey)
		}
	})
}

func TestDownloadClientUpdate_ValueAndClearFlagConflict(t *testing.T) {
	for name, body := range map[string]string{
		"apiKey":   `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","apiKey":"x","clearApiKey":true}`,
		"password": `{"name":"SAB","host":"10.10.10.10","type":"qbittorrent","password":"x","clearPassword":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			h, clients := downloadClientFixture(t)
			seeded := seedDownloadClient(t, clients, models.DownloadClient{
				Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
			})
			rec := updateDownloadClient(t, h, seeded.ID, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			stored, _ := clients.GetByID(context.Background(), seeded.ID)
			if stored.APIKey != "sab-key" {
				t.Errorf("stored apiKey = %q, want the rejected update to change nothing", stored.APIKey)
			}
		})
	}
}

func TestDownloadClientUpdate_NonBooleanClearFlag(t *testing.T) {
	h, clients := downloadClientFixture(t)
	seeded := seedDownloadClient(t, clients, models.DownloadClient{
		Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
	})
	rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","clearApiKey":"yes"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDownloadClientUpdate_TypeSwitchClearsAbandonedCredential is the case the
// clear flags exist for: the edit form moves a client from a password type to
// an API-key type, and the password it no longer uses has to go with it.
func TestDownloadClientUpdate_TypeSwitchClearsAbandonedCredential(t *testing.T) {
	ctx := context.Background()

	t.Run("password type to api-key type", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "qBit", Type: "qbittorrent", Host: "10.10.10.11", Port: 8080, Username: "admin", Password: "qbit-pass", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID,
			`{"name":"SAB","host":"10.10.10.11","type":"sabnzbd","username":"","apiKey":"sab-key","clearPassword":true,"category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.Password != "" {
			t.Errorf("stored password = %q, want the abandoned credential cleared", stored.Password)
		}
		if stored.APIKey != "sab-key" {
			t.Errorf("stored apiKey = %q, want sab-key", stored.APIKey)
		}
	})

	t.Run("api-key type to password type", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID,
			`{"name":"Deluge","host":"10.10.10.10","type":"deluge","password":"deluge-pass","clearApiKey":true,"category":"books"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.APIKey != "" {
			t.Errorf("stored apiKey = %q, want the abandoned credential cleared", stored.APIKey)
		}
		if stored.Password != "deluge-pass" {
			t.Errorf("stored password = %q, want deluge-pass", stored.Password)
		}
	})
}

// TestDownloadClientUpdate_OmittedFieldsSurvive covers the behaviour change
// that comes with decoding over the stored row: a key the caller left out is
// no longer reset to its zero value, while an explicitly sent false still
// turns a boolean off.
func TestDownloadClientUpdate_OmittedFieldsSurvive(t *testing.T) {
	ctx := context.Background()

	t.Run("omitted fields keep their stored values", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key",
			Category: "books", CategoryAudiobook: "audiobooks", PathRemap: "/remote:/local",
			Priority: 3, UseSSL: true, URLBase: "/sab", Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB renamed"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if !stored.Enabled {
			t.Error("enabled was wiped by an update that did not mention it")
		}
		if !stored.UseSSL {
			t.Error("useSsl was wiped by an update that did not mention it")
		}
		if stored.Category != "books" || stored.CategoryAudiobook != "audiobooks" {
			t.Errorf("categories wiped: %q / %q", stored.Category, stored.CategoryAudiobook)
		}
		if stored.PathRemap != "/remote:/local" {
			t.Errorf("pathRemap wiped: %q", stored.PathRemap)
		}
		if stored.Priority != 3 {
			t.Errorf("priority wiped: %d", stored.Priority)
		}
		if stored.URLBase != "/sab" {
			t.Errorf("urlBase wiped: %q", stored.URLBase)
		}
	})

	t.Run("an explicit false still disables", func(t *testing.T) {
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key",
			Category: "books", UseSSL: true, Enabled: true,
		})
		rec := updateDownloadClient(t, h, seeded.ID, `{"enabled":false,"useSsl":false}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		stored, _ := clients.GetByID(ctx, seeded.ID)
		if stored.Enabled {
			t.Error("an explicit enabled:false did not disable the client")
		}
		if stored.UseSSL {
			t.Error("an explicit useSsl:false did not clear the flag")
		}
	})
}

// TestDownloadClientUpdate_EvictsPooledClient guards the requirement that the
// pooled downloader client is dropped on every successful update, including
// one that leaves the credentials untouched.
func TestDownloadClientUpdate_EvictsPooledClient(t *testing.T) {
	cache := downloader.NewClientCache()
	prev := downloader.SetDefaultCache(cache)
	t.Cleanup(func() { downloader.SetDefaultCache(prev) })

	h, clients := downloadClientFixture(t)
	seeded := seedDownloadClient(t, clients, models.DownloadClient{
		Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
	})
	stored, _ := clients.GetByID(context.Background(), seeded.ID)
	downloader.SabnzbdFor(stored)
	if cache.Len() != 1 {
		t.Fatalf("expected the pooled client to be cached, len = %d", cache.Len())
	}

	// No credential in the body at all: the stored key is kept, and the pooled
	// client still has to go so a changed host or category takes effect.
	rec := updateDownloadClient(t, h, seeded.ID, `{"name":"SAB","host":"10.10.10.10","type":"sabnzbd","category":"ebooks"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if cache.Len() != 0 {
		t.Errorf("pooled client was not evicted, len = %d", cache.Len())
	}
}

func TestDownloadClientUpdate_NotFound(t *testing.T) {
	h, _ := downloadClientFixture(t)
	rec := updateDownloadClient(t, h, 999, `{"name":"nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDownloadClientUpdate_InvalidBody(t *testing.T) {
	h, clients := downloadClientFixture(t)
	seeded := seedDownloadClient(t, clients, models.DownloadClient{
		Name: "SAB", Type: "sabnzbd", Host: "10.10.10.10", Port: 8080, APIKey: "sab-key", Category: "books", Enabled: true,
	})
	rec := updateDownloadClient(t, h, seeded.ID, `not-json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
