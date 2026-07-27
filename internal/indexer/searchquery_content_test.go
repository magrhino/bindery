package indexer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

// TestSearchQuery_AppliesContentGuards pins the fix for the freeform /search
// endpoint returning movie and usenet-junk results.
//
// SearchQuery backs GET /indexer/search, which the /search page renders with a
// per-result Grab button. It ran dedupe + rankResults only, so the #1591 video
// guard — added to SearchBook, then to SearchBookWithDebug in #1644 — was still
// reachable by hand from the UI. A grabbed movie imports as the wrong file.
func TestSearchQuery_AppliesContentGuards(t *testing.T) {
	const rssBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <newznab:response offset="0" total="6"/>
    <item>
      <title>Frank Herbert - Dune - retail epub</title>
      <guid isPermaLink="false">guid-keep-1</guid>
      <enclosure url="https://fake/dl/1" length="1000" type="application/x-nzb"/>
      <newznab:attr name="category" value="7020"/>
    </item>
    <item>
      <title>Dune 2021 1080p WEB-DL x264-GROUP</title>
      <guid isPermaLink="false">guid-video-res</guid>
      <enclosure url="https://fake/dl/2" length="1000" type="application/x-nzb"/>
    </item>
    <item>
      <title>Dune 2021 x265 BluRay REMUX</title>
      <guid isPermaLink="false">guid-video-codec</guid>
      <enclosure url="https://fake/dl/3" length="1000" type="application/x-nzb"/>
    </item>
    <item>
      <title>Dune S01E02 HDTV</title>
      <guid isPermaLink="false">guid-video-tv</guid>
      <enclosure url="https://fake/dl/4" length="1000" type="application/x-nzb"/>
    </item>
    <item>
      <title>Frank Herbert - Dune - Special Feature</title>
      <guid isPermaLink="false">guid-cat-movie</guid>
      <enclosure url="https://fake/dl/5" length="1000" type="application/x-nzb"/>
      <newznab:attr name="category" value="2000"/>
    </item>
    <item>
      <title>Frank Herbert - Dune.part03.rar</title>
      <guid isPermaLink="false">guid-junk</guid>
      <enclosure url="https://fake/dl/6" length="1000" type="application/x-nzb"/>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	idxs := []models.Indexer{{ID: 1, Name: "test", URL: srv.URL, Enabled: true, Categories: []int{7020}}}
	results := newTestSearcher().SearchQuery(context.Background(), idxs, "Dune")

	got := make(map[string]bool, len(results))
	for _, r := range results {
		got[r.GUID] = true
	}

	for _, guid := range []string{"guid-video-res", "guid-video-codec", "guid-video-tv", "guid-cat-movie", "guid-junk"} {
		if got[guid] {
			t.Errorf("expected %s to be filtered out of SearchQuery results", guid)
		}
	}
	if !got["guid-keep-1"] {
		t.Error("expected the legitimate ebook release to survive filtering")
	}
	if len(results) != 1 {
		t.Errorf("expected exactly 1 result, got %d", len(results))
		for _, r := range results {
			t.Logf("  kept: %s (%s)", r.Title, r.GUID)
		}
	}
}

// TestSearchQuery_KeepsResultsUnrelatedToQuery pins the deliberate absence of
// filterRelevant. A freeform query has no book identity to score against, so
// relevance is the user's call — only the query-independent content guards
// belong on this path. Without this test, "apply the same filters as
// SearchBook" is an easy over-correction that would silently empty the page.
func TestSearchQuery_KeepsResultsUnrelatedToQuery(t *testing.T) {
	const rssBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <newznab:response offset="0" total="1"/>
    <item>
      <title>Some Totally Unrelated Cookbook epub</title>
      <guid isPermaLink="false">guid-unrelated</guid>
      <enclosure url="https://fake/dl/1" length="1000" type="application/x-nzb"/>
      <newznab:attr name="category" value="7020"/>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	idxs := []models.Indexer{{ID: 1, Name: "test", URL: srv.URL, Enabled: true, Categories: []int{7020}}}
	results := newTestSearcher().SearchQuery(context.Background(), idxs, "Dune")

	if len(results) != 1 {
		t.Fatalf("freeform search must not apply relevance filtering; got %d results, want 1", len(results))
	}
}
