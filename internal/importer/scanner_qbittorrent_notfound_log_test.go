package importer

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

// TestCheckQbittorrentDownloads_NotFoundLogSkipsTerminalStates is the
// regression test for issue #1730.
//
// s.downloads.List has no status filter, so the qBittorrent poll also walks
// long-imported downloads. When such a download's torrent has been removed
// from qBittorrent after import, the not-found branch logged
// "qbittorrent: download not found in torrent list" at Debug — once per
// imported download, per 15s poll, forever (measured at 96% of all debug
// output on a 53-download library). The terminal-status skip sat AFTER the
// log; the no-hash branch above it checks BEFORE. This test pins the fixed
// ordering:
//
//   - StateImported / StateFailed rows whose torrent is gone emit NO line.
//   - StateImportFailed rows still emit the line AND still flow into
//     blockStaleImportFailures unseen, so a vanished source is terminally
//     blocked exactly as before (the #706 finding-4 behavior must not change).
func TestCheckQbittorrentDownloads_NotFoundLogSkipsTerminalStates(t *testing.T) {
	// qBittorrent holds no torrents at all: every tracked hash misses both the
	// category-filtered map and the unfiltered fallback, driving each download
	// into the not-found branch. The unfiltered listing succeeding is what
	// makes sourceListIsComplete=true for blockStaleImportFailures.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte("[]"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	database, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()
	dlRepo := db.NewDownloadRepo(database)
	clientRepo := db.NewDownloadClientRepo(database)
	bookRepo := db.NewBookRepo(database)
	authorRepo := db.NewAuthorRepo(database)
	histRepo := db.NewHistoryRepo(database)

	s := NewScanner(dlRepo, clientRepo, bookRepo, authorRepo, histRepo, t.TempDir(), "", "", "", "")

	host, port := scannerTestHostPort(t, srv.URL)
	client := &models.DownloadClient{
		Name:    "qbit-1730",
		Type:    "qbittorrent",
		Host:    host,
		Port:    port,
		Enabled: true,
	}
	if err := clientRepo.Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}

	mkDownload := func(guid, title, hash string, status models.DownloadState) {
		t.Helper()
		h := hash
		dl := &models.Download{
			GUID:             guid,
			Title:            title,
			NZBURL:           "magnet:?xt=urn:btih:" + hash,
			Status:           status,
			Protocol:         "torrent",
			TorrentID:        &h,
			DownloadClientID: &client.ID,
		}
		if err := dlRepo.Create(ctx, dl); err != nil {
			t.Fatalf("create download %s: %v", guid, err)
		}
	}
	mkDownload("guid-1730-imported", "long-imported-book", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", models.StateImported)
	mkDownload("guid-1730-failed", "failed-book", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", models.StateFailed)
	mkDownload("guid-1730-importfailed", "vanished-importfailed-book", "cccccccccccccccccccccccccccccccccccccccc", models.StateImportFailed)

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	s.checkQbittorrentDownloads(ctx, client)

	// Collect only the not-found lines so unrelated Debug output (e.g. the
	// per-poll summary line) can't mask a regression.
	var notFoundLines []string
	for _, line := range strings.Split(logBuf.String(), "\n") {
		if strings.Contains(line, "download not found in torrent list") {
			notFoundLines = append(notFoundLines, line)
		}
	}
	joined := strings.Join(notFoundLines, "\n")

	// Match on the full title attribute: "failed-book" alone is a substring of
	// "vanished-importfailed-book" and would false-positive.
	if strings.Contains(joined, "title=long-imported-book") {
		t.Errorf("#1730 regression: StateImported download logged the not-found Debug line:\n%s", joined)
	}
	if strings.Contains(joined, "title=failed-book") {
		t.Errorf("#1730 regression: StateFailed download logged the not-found Debug line:\n%s", joined)
	}
	if !strings.Contains(joined, "title=vanished-importfailed-book") {
		t.Errorf("StateImportFailed must keep emitting the not-found Debug line (operator trace for blockStaleImportFailures), got:\n%s", logBuf.String())
	}

	// Behavioral invariant: the skip must not change what reaches
	// blockStaleImportFailures. Terminal rows stay terminal, and the
	// StateImportFailed row — absent from a complete source listing — is
	// terminally blocked exactly as before the fix.
	assertState := func(guid string, want models.DownloadState) {
		t.Helper()
		got, err := dlRepo.GetByGUID(ctx, guid)
		if err != nil {
			t.Fatalf("get %s: %v", guid, err)
		}
		if got.Status != want {
			t.Errorf("%s: status = %s, want %s", guid, got.Status, want)
		}
	}
	assertState("guid-1730-imported", models.StateImported)
	assertState("guid-1730-failed", models.StateFailed)
	assertState("guid-1730-importfailed", models.StateImportBlocked)
}
