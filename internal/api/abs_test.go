package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/abs"
	"github.com/vavallee/bindery/internal/db"
)

type stubABSClient struct {
	authorizeResp *abs.AuthorizeResponse
	authorizeErr  error
	librariesResp []abs.Library
	librariesErr  error
	libraryResp   *abs.Library
	libraryErr    error
	lastLibraryID string
}

func (s *stubABSClient) Authorize(context.Context) (*abs.AuthorizeResponse, error) {
	return s.authorizeResp, s.authorizeErr
}

func (s *stubABSClient) ListLibraries(context.Context) ([]abs.Library, error) {
	return s.librariesResp, s.librariesErr
}

func (s *stubABSClient) GetLibrary(_ context.Context, id string) (*abs.Library, error) {
	s.lastLibraryID = id
	return s.libraryResp, s.libraryErr
}

func absFixture(t *testing.T, client absClient) (*ABSHandler, *db.SettingsRepo, context.Context) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	repo := db.NewSettingsRepo(database)
	h := NewABSHandler(repo)
	h.newFn = func(baseURL, apiKey string) (absClient, error) {
		if client == nil {
			return nil, errors.New("no client")
		}
		return client, nil
	}
	return h, repo, context.Background()
}

func TestABSConfigGetRedactsAPIKey(t *testing.T) {
	h, repo, ctx := absFixture(t, &stubABSClient{})
	if err := repo.Set(ctx, SettingABSBaseURL, "https://abs.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, SettingABSAPIKey, "secret"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.GetConfig(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	if bytes.Contains([]byte(body), []byte("secret")) {
		t.Fatalf("api key leaked in response: %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"apiKeyConfigured":true`)) {
		t.Fatalf("expected apiKeyConfigured=true, got %s", body)
	}
}

func TestABSConfigGetIncludesFeatureFlag(t *testing.T) {
	h, _, _ := absFixture(t, &stubABSClient{})
	h.WithFeatureEnabled(false)

	rec := httptest.NewRecorder()
	h.GetConfig(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"featureEnabled":false`)) {
		t.Fatalf("expected featureEnabled=false, got %s", rec.Body.String())
	}
}

func TestABSSetConfigValidatesSelectedLibrary(t *testing.T) {
	client := &stubABSClient{
		libraryResp: &abs.Library{ID: "lib_books", Name: "Books", MediaType: "book"},
	}
	h, repo, _ := absFixture(t, client)

	body := bytes.NewBufferString(`{"baseUrl":"https://abs.example.com/","apiKey":"secret","libraryId":"lib_books","enabled":true,"label":"Shelf"}`)
	rec := httptest.NewRecorder()
	h.SetConfig(rec, httptest.NewRequest(http.MethodPut, "/api/v1/abs/config", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
	}
	if client.lastLibraryID != "lib_books" {
		t.Fatalf("validated library = %q", client.lastLibraryID)
	}
	got, _ := repo.Get(context.Background(), SettingABSAPIKey)
	if got == nil || got.Value != "secret" {
		t.Fatalf("api key not persisted: %+v", got)
	}
}

func TestABSSetConfig_OmittedBaseURLPreservesStoredValue(t *testing.T) {
	client := &stubABSClient{
		libraryResp: &abs.Library{ID: "lib_books", Name: "Books", MediaType: "book"},
	}
	h, repo, ctx := absFixture(t, client)
	if err := repo.Set(ctx, SettingABSBaseURL, "https://abs.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, SettingABSAPIKey, "old-secret"); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"apiKey":"new-secret","libraryId":"lib_books","enabled":true}`)
	rec := httptest.NewRecorder()
	h.SetConfig(rec, httptest.NewRequest(http.MethodPut, "/api/v1/abs/config", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
	}

	got, _ := repo.Get(ctx, SettingABSBaseURL)
	if got == nil || got.Value != "https://abs.example.com" {
		t.Fatalf("base URL = %+v, want preserved value", got)
	}
}

func TestABSSetConfig_PersistsPathRemap(t *testing.T) {
	client := &stubABSClient{
		libraryResp: &abs.Library{ID: "lib_books", Name: "Books", MediaType: "book"},
	}
	h, repo, _ := absFixture(t, client)

	body := bytes.NewBufferString(`{"baseUrl":"https://abs.example.com/","apiKey":"secret","libraryId":"lib_books","enabled":true,"label":"Shelf","pathRemap":"/audiobookshelf:/books/audiobookshelf"}`)
	rec := httptest.NewRecorder()
	h.SetConfig(rec, httptest.NewRequest(http.MethodPut, "/api/v1/abs/config", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := repo.Get(context.Background(), SettingABSPathRemap)
	if got == nil || got.Value != "/audiobookshelf:/books/audiobookshelf" {
		t.Fatalf("path remap not persisted: %+v", got)
	}

	rec = httptest.NewRecorder()
	h.GetConfig(rec, httptest.NewRequest(http.MethodGet, "/api/v1/abs/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GetConfig code = %d", rec.Code)
	}
	var cfg ABSConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.PathRemap != "/audiobookshelf:/books/audiobookshelf" {
		t.Fatalf("pathRemap = %q", cfg.PathRemap)
	}
}

func TestABSTestMapsUnauthorizedTo502(t *testing.T) {
	client := &stubABSClient{
		authorizeErr: &abs.APIError{StatusCode: http.StatusUnauthorized, Message: "bad key"},
	}
	h, repo, ctx := absFixture(t, client)
	if err := repo.Set(ctx, SettingABSBaseURL, "https://abs.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, SettingABSAPIKey, "secret"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.Test(rec, httptest.NewRequest(http.MethodPost, "/api/v1/abs/test", bytes.NewBufferString(`{}`)))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestABSLibrariesFiltersToBookLibraries(t *testing.T) {
	client := &stubABSClient{
		librariesResp: []abs.Library{
			{ID: "lib_books", Name: "Books", MediaType: "book"},
			{ID: "lib_podcasts", Name: "Podcasts", MediaType: "podcast"},
		},
	}
	h, repo, ctx := absFixture(t, client)
	if err := repo.Set(ctx, SettingABSBaseURL, "https://abs.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, SettingABSAPIKey, "secret"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.Libraries(rec, httptest.NewRequest(http.MethodPost, "/api/v1/abs/libraries", bytes.NewBufferString(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0]["id"] != "lib_books" {
		t.Fatalf("id = %v", got[0]["id"])
	}
}
