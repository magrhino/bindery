package models

import "time"

type MetadataProfile struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	MinPopularity    int       `json:"minPopularity"`
	MinPages         int       `json:"minPages"`
	SkipMissingDate  bool      `json:"skipMissingDate"`
	SkipMissingISBN  bool      `json:"skipMissingIsbn"`
	SkipPartBooks    bool      `json:"skipPartBooks"`
	AllowedLanguages string    `json:"allowedLanguages"`
	CreatedAt        time.Time `json:"createdAt"`
}
