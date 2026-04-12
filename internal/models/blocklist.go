package models

import "time"

type BlocklistEntry struct {
	ID        int64     `json:"id"`
	BookID    *int64    `json:"bookId"`
	GUID      string    `json:"guid"`
	Title     string    `json:"title"`
	IndexerID *int64    `json:"indexerId"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}
