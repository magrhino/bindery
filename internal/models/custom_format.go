package models

import "time"

type CustomFormat struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"`
	Conditions []CustomCondition `json:"conditions"`
	CreatedAt  time.Time         `json:"createdAt"`
}

type CustomCondition struct {
	Type     string `json:"type"`    // "releaseTitle", "releaseGroup", "size", "indexerFlag"
	Pattern  string `json:"pattern"`
	Negate   bool   `json:"negate"`
	Required bool   `json:"required"`
}
