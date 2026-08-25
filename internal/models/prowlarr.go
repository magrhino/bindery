package models

import "time"

// ProwlarrInstance holds the connection config for a Prowlarr server.
type ProwlarrInstance struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
	// APIKeyConfigured is a response-only field: the API blanks APIKey on the
	// way out and sets this instead, so the client can render "a key is set"
	// without ever receiving the credential. It is never persisted (the
	// prowlarr repo enumerates its columns) and anything a client sends in it
	// is ignored.
	APIKeyConfigured bool       `json:"apiKeyConfigured"`
	SyncOnStartup    bool       `json:"syncOnStartup"`
	Enabled          bool       `json:"enabled"`
	LastSyncAt       *time.Time `json:"lastSyncAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}
