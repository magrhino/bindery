package models

import "time"

type DelayProfile struct {
	ID                int64     `json:"id"`
	UsenetDelay       int       `json:"usenetDelay"`
	TorrentDelay      int       `json:"torrentDelay"`
	PreferredProtocol string    `json:"preferredProtocol"`
	EnableUsenet      bool      `json:"enableUsenet"`
	EnableTorrent     bool      `json:"enableTorrent"`
	Order             int       `json:"order"`
	CreatedAt         time.Time `json:"createdAt"`
}
