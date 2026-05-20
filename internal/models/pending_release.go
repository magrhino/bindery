package models

import "time"

// PendingRelease is an indexer result that was found but not yet grabbed,
// typically because a delay profile timer has not expired. The full release
// JSON is stored so it can be re-evaluated on subsequent scheduler sweeps
// without another network round-trip to the indexer.
type PendingRelease struct {
	ID     int64 `json:"id"`
	BookID int64 `json:"bookId"`
	// MediaType is "ebook" or "audiobook" — it scopes the entry to the format
	// being searched so a dual-format book's ebook and audiobook pending
	// entries can be managed independently (see #707).
	MediaType   string    `json:"mediaType"`
	Title       string    `json:"title"`
	IndexerID   *int64    `json:"indexerId,omitempty"`
	GUID        string    `json:"guid"`
	Protocol    string    `json:"protocol"`
	Size        int64     `json:"size"`
	AgeMinutes  int       `json:"ageMinutes"`
	Quality     string    `json:"quality,omitempty"`
	CustomScore int       `json:"customScore"`
	Reason      string    `json:"reason"`
	FirstSeen   time.Time `json:"firstSeen"`
	ReleaseJSON string    `json:"releaseJson"`
}
