package abs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestClientAuthorize_DecodesPinnedContractV2332(t *testing.T) {
	t.Parallel()

	payload := loadFixture(t, "authorize_v2_33_2.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/authorize" {
			t.Fatalf("path = %s, want /api/authorize", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "fixture-key")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Authorize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.User.Username != "fixture-user" {
		t.Fatalf("username = %q", resp.User.Username)
	}
	if resp.UserDefaultLibraryID != "lib-books" {
		t.Fatalf("default library = %q", resp.UserDefaultLibraryID)
	}
	if resp.ServerSettings.Version != "2.33.2" {
		t.Fatalf("version = %q", resp.ServerSettings.Version)
	}
}

func TestClientListLibraries_DecodesPinnedContractV2332(t *testing.T) {
	t.Parallel()

	payload := loadFixture(t, "libraries_v2_33_2.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/libraries" {
			t.Fatalf("path = %s, want /api/libraries", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, "fixture-key")
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := client.ListLibraries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(libraries) != 2 {
		t.Fatalf("len = %d, want 2", len(libraries))
	}
	if libraries[0].MediaType != "book" {
		t.Fatalf("first library mediaType = %q", libraries[0].MediaType)
	}
	if libraries[0].Folders[0].FullPath != "/audiobooks" {
		t.Fatalf("folder path = %q", libraries[0].Folders[0].FullPath)
	}
	if libraries[1].MediaType != "podcast" {
		t.Fatalf("second library mediaType = %q", libraries[1].MediaType)
	}
}
