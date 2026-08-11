package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vavallee/bindery/internal/models"
)

// LibrarySnapshot answers FindExisting queries from one walk of each library
// root instead of one walk per query.
//
// FindExisting is called once per book an author sync creates, and each call
// used to re-walk the whole library tree (#1888/#1929). On local disk with a
// warm dentry cache that hides; on an NFS/SMB mount with a large library a
// single walk can take seconds, and a 65-book sync paid it 65 times — which is
// the right order of magnitude for the reported "an hour to add 65 books". The
// walk also parses every filename per call, so the sync re-ran the same
// ParseFilename work per book.
//
// A snapshot walks a root at most once, on the first query that needs it, and
// serves every later query from the parsed entries. It deliberately holds only
// the two root paths, not a *Scanner, so tests and callers can build one over
// a bare directory.
//
// Staleness, stated: files that appear under a root after that root's walk are
// invisible to later queries on the same snapshot. Within an author sync that
// window is close to vacuous — auto-search for the books being created runs
// after the create loop, so any file that can match one of them was on disk
// before the sync started; only a file copied in by hand mid-sync is missed,
// and the next sync sees it. One-off callers get a fresh snapshot per call
// (Scanner.FindExisting), which is exactly the old semantics.
type LibrarySnapshot struct {
	libraryDir   string
	audiobookDir string

	mu    sync.Mutex
	roots map[string][]libraryEntry
}

// libraryEntry is one book file, pre-parsed at walk time so queries are pure
// in-memory comparisons.
type libraryEntry struct {
	path string
	// firstDir is the first path segment under the root — the author folder in
	// an Author/Title layout. Empty for files sitting directly under the root,
	// which the author pre-filter has always exempted.
	firstDir string
	title    string
	author   string
}

// NewLibrarySnapshot builds an empty snapshot over the given roots. Roots are
// walked lazily, each on the first query that selects it, so a snapshot that
// only ever sees ebook queries never touches the audiobook root.
func NewLibrarySnapshot(libraryDir, audiobookDir string) *LibrarySnapshot {
	return &LibrarySnapshot{libraryDir: libraryDir, audiobookDir: audiobookDir}
}

// FindExisting reports the first library file matching title/author, or "".
// Semantics are identical to the pre-snapshot Scanner walk: the same media-type
// root selection, the same author-folder pre-filter, the same title/author
// match, in the same walk order — only the walk count changes.
func (ls *LibrarySnapshot) FindExisting(ctx context.Context, title, authorName, mediaType string) string {
	if title == "" {
		return ""
	}
	for _, root := range ls.rootsForMediaType(mediaType) {
		entries, ok := ls.entriesFor(ctx, root)
		if !ok {
			// Cancelled mid-walk: match the old walk's behaviour of quietly
			// finding nothing, and leave the root uncached so a live caller
			// gets a real walk.
			return ""
		}
		for i := range entries {
			e := &entries[i]
			if authorName != "" && e.firstDir != "" && !authorMatch(authorName, e.firstDir) {
				continue
			}
			if titleMatch(e.title, title) && authorMatch(authorName, e.author) {
				return e.path
			}
		}
	}
	return ""
}

// rootsForMediaType selects which roots a query walks: ebook → libraryDir,
// audiobook → audiobookDir (falling back to libraryDir when unset), both or
// unknown → both with libraryDir first (#488).
func (ls *LibrarySnapshot) rootsForMediaType(mediaType string) []string {
	roots := make([]string, 0, 2)
	switch mediaType {
	case models.MediaTypeEbook:
		if ls.libraryDir != "" {
			roots = append(roots, ls.libraryDir)
		}
	case models.MediaTypeAudiobook:
		switch {
		case ls.audiobookDir != "":
			roots = append(roots, ls.audiobookDir)
		case ls.libraryDir != "":
			roots = append(roots, ls.libraryDir)
		}
	default:
		if ls.libraryDir != "" {
			roots = append(roots, ls.libraryDir)
		}
		if ls.audiobookDir != "" && ls.audiobookDir != ls.libraryDir {
			roots = append(roots, ls.audiobookDir)
		}
	}
	return roots
}

// entriesFor returns the cached entries for root, walking it on first use. The
// second return is false only when the walk was abandoned on a cancelled
// context — that result is not cached, so the cancellation of one sync cannot
// blind a later caller.
func (ls *LibrarySnapshot) entriesFor(ctx context.Context, root string) ([]libraryEntry, bool) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if entries, ok := ls.roots[root]; ok {
		return entries, true
	}
	entries, ok := walkLibraryEntries(ctx, root)
	if !ok {
		return nil, false
	}
	if ls.roots == nil {
		ls.roots = make(map[string][]libraryEntry)
	}
	ls.roots[root] = entries
	return entries, true
}

// walkLibraryEntries collects every book file under root. Unreadable entries
// are skipped rather than aborting the walk, matching the old per-query walk;
// a missing root simply yields no entries. Returns ok=false only on context
// cancellation, which the old walk ignored outright (#1929) — a cancelled sync
// kept walking the whole tree.
func walkLibraryEntries(ctx context.Context, root string) ([]libraryEntry, bool) {
	var entries []libraryEntry
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // best-effort walk: skip unreadable entries rather than abort
		}
		if !IsBookFile(path) {
			return nil
		}
		firstDir := ""
		if rel, relErr := filepath.Rel(root, path); relErr == nil {
			if parts := strings.SplitN(rel, string(filepath.Separator), 2); len(parts) >= 2 {
				firstDir = parts[0]
			}
		}
		parsed := ParseFilename(path)
		entries = append(entries, libraryEntry{
			path:     path,
			firstDir: firstDir,
			title:    parsed.Title,
			author:   parsed.Author,
		})
		return nil
	})
	if ctx.Err() != nil {
		return nil, false
	}
	return entries, true
}
