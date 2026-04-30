package qbittorrent

import (
	"path/filepath"
	"strings"
)

// Torrent represents a single torrent as returned by the qBittorrent WebUI API.
type Torrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	Size        int64   `json:"size"`
	Progress    float64 `json:"progress"`
	State       string  `json:"state"`
	Category    string  `json:"category"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
	ETA         int     `json:"eta"`
	AddedOn     int64   `json:"added_on"`
}

// PathCandidates returns the qBittorrent-reported paths that may contain the
// torrent payload, ordered from most to least specific.
func (t Torrent) PathCandidates() []string {
	var candidates []string
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	add(t.ContentPath)
	if strings.TrimSpace(t.SavePath) != "" && strings.TrimSpace(t.Name) != "" {
		add(filepath.Join(t.SavePath, t.Name))
	}
	add(t.SavePath)
	return candidates
}
