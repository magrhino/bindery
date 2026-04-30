package qbittorrent

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestTorrentPathCandidates(t *testing.T) {
	torrent := Torrent{
		ContentPath: "/downloads/content",
		SavePath:    "/downloads",
		Name:        "Book",
	}
	want := []string{"/downloads/content", filepath.Join("/downloads", "Book"), "/downloads"}

	if got := torrent.PathCandidates(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PathCandidates() = %#v, want %#v", got, want)
	}
}

func TestTorrentPathCandidatesDeduplicates(t *testing.T) {
	torrent := Torrent{
		ContentPath: filepath.Join("/downloads", "Book"),
		SavePath:    "/downloads",
		Name:        "Book",
	}
	want := []string{filepath.Join("/downloads", "Book"), "/downloads"}

	if got := torrent.PathCandidates(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PathCandidates() = %#v, want %#v", got, want)
	}
}
