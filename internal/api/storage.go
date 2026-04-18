package api

import (
	"net/http"

	"github.com/vavallee/bindery/internal/config"
)

// StorageHandler exposes the process-level storage paths loaded from
// environment / config file so the Settings UI can display them without
// asking the user to `docker exec` into the container.
//
// These values are intentionally read-only: the importer captures them at
// startup, so mutating them via the API would drift from the running
// process. Per-library root folders (the editable ones) live under
// /api/v1/rootfolder.
type StorageHandler struct {
	cfg *config.Config
}

func NewStorageHandler(cfg *config.Config) *StorageHandler {
	return &StorageHandler{cfg: cfg}
}

type storageResponse struct {
	DownloadDir  string `json:"downloadDir"`
	LibraryDir   string `json:"libraryDir"`
	AudiobookDir string `json:"audiobookDir"`
}

// Get handles GET /api/v1/system/storage.
func (h *StorageHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, storageResponse{
		DownloadDir:  h.cfg.DownloadDir,
		LibraryDir:   h.cfg.LibraryDir,
		AudiobookDir: h.cfg.AudiobookDir,
	})
}
