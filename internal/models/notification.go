package models

import "time"

type Notification struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	URL       string    `json:"url"`
	Method    string    `json:"method"`
	Headers   string    `json:"headers"`
	OnGrab    bool      `json:"onGrab"`
	OnImport  bool      `json:"onImport"`
	OnUpgrade bool      `json:"onUpgrade"`
	OnFailure bool      `json:"onFailure"`
	OnHealth  bool      `json:"onHealth"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
