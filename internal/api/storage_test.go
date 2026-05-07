package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/config"
)

func TestStorageHandler_Get(t *testing.T) {
	cfg := &config.Config{
		DownloadDir:          "/downloads",
		AudiobookDownloadDir: "/audiobook-downloads",
		LibraryDir:           "/books",
		AudiobookDir:         "/audiobooks",
	}
	h := NewStorageHandler(cfg)

	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/system/storage", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got storageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.DownloadDir != "/downloads" || got.AudiobookDownloadDir != "/audiobook-downloads" ||
		got.LibraryDir != "/books" || got.AudiobookDir != "/audiobooks" {
		t.Errorf("unexpected payload: %+v", got)
	}
}

func TestStorageHandler_EmptyAudiobookDirPassesThrough(t *testing.T) {
	cfg := &config.Config{DownloadDir: "/d", LibraryDir: "/l", AudiobookDir: ""}
	h := NewStorageHandler(cfg)

	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/system/storage", nil))

	var got storageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AudiobookDir != "" {
		t.Errorf("AudiobookDir = %q, want empty so the UI can fall back to LibraryDir", got.AudiobookDir)
	}
}

func TestStorageHandler_EmptyAudiobookDownloadDirPassesThrough(t *testing.T) {
	cfg := &config.Config{DownloadDir: "/d", AudiobookDownloadDir: "", LibraryDir: "/l", AudiobookDir: ""}
	h := NewStorageHandler(cfg)

	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest(http.MethodGet, "/system/storage", nil))

	var got storageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AudiobookDownloadDir != "" {
		t.Errorf("AudiobookDownloadDir = %q, want empty so the UI can fall back to DownloadDir", got.AudiobookDownloadDir)
	}
}
