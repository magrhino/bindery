package migrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/metadata"
)

// goodreadsPreviewTTL is how long a resolved preview is held in memory before
// it is evicted. A user inspects the preview and clicks Commit within a few
// minutes; an abandoned preview should not pin resolved books forever.
const goodreadsPreviewTTL = 30 * time.Minute

// Sentinel errors for the Goodreads import flow. The API layer maps these to
// HTTP status codes without string-matching.
var (
	// ErrGoodreadsPreviewNotFound is returned by Commit when the token is
	// unknown — either never issued, already committed, or expired.
	ErrGoodreadsPreviewNotFound = errors.New("goodreads import preview not found or expired")
	// ErrGoodreadsRunning is returned when a preview is already being
	// resolved; only one resolution pass runs at a time.
	ErrGoodreadsRunning = errors.New("a goodreads import is already being prepared")
)

// goodreadsPreviewEntry is a resolved preview held server-side between the
// dry-run and the commit.
type goodreadsPreviewEntry struct {
	preview   *GoodreadsPreview
	createdAt time.Time
}

// GoodreadsImporter coordinates the two-step Goodreads CSV import: a dry-run
// Preview (parse + resolve, no writes) followed by a Commit that persists the
// already-resolved books. Resolved previews are cached in memory keyed by a
// token so the expensive provider round-trips happen exactly once.
//
// A single instance is shared across HTTP handlers; the mutex guards the
// preview cache and the single-resolution guard.
type GoodreadsImporter struct {
	authors *db.AuthorRepo
	books   *db.BookRepo
	meta    *metadata.Aggregator

	mu        sync.Mutex
	resolving bool
	previews  map[string]goodreadsPreviewEntry

	// pacing is the gap between provider lookups. Defaults to
	// goodreadsResolvePacing; tests override it to 0.
	pacing time.Duration

	// resolveFn does the per-row resolution. Indirected so tests can swap in
	// a deterministic resolver without a live metadata provider.
	resolveFn func(ctx context.Context, rows []GoodreadsRow, opts GoodreadsImportOptions, pacing time.Duration) []GoodreadsResolvedRow
}

// NewGoodreadsImporter wires the repos and metadata aggregator the import
// needs. meta may be nil only in tests that inject resolveFn.
func NewGoodreadsImporter(authors *db.AuthorRepo, books *db.BookRepo, meta *metadata.Aggregator) *GoodreadsImporter {
	imp := &GoodreadsImporter{
		authors:  authors,
		books:    books,
		meta:     meta,
		previews: map[string]goodreadsPreviewEntry{},
		pacing:   goodreadsResolvePacing,
	}
	imp.resolveFn = func(ctx context.Context, rows []GoodreadsRow, opts GoodreadsImportOptions, pacing time.Duration) []GoodreadsResolvedRow {
		return ResolveGoodreadsRows(ctx, rows, opts, imp.meta, imp.books, pacing)
	}
	return imp
}

// WithPacing overrides the inter-lookup delay. Tests pass 0.
func (imp *GoodreadsImporter) WithPacing(d time.Duration) *GoodreadsImporter {
	imp.pacing = d
	return imp
}

// withResolveFn swaps the resolution function. Test-only.
func (imp *GoodreadsImporter) withResolveFn(fn func(ctx context.Context, rows []GoodreadsRow, opts GoodreadsImportOptions, pacing time.Duration) []GoodreadsResolvedRow) *GoodreadsImporter {
	imp.resolveFn = fn
	return imp
}

// Preview parses the CSV, resolves every in-scope row, caches the result, and
// returns the dry-run summary. No data is written. The returned token must be
// passed to Commit to persist the books.
//
// Resolution is synchronous: it can take a while for a large export, but the
// per-row pacing keeps it bounded and the HTTP layer runs it with a context
// detached from the request so a slow client does not abort it.
func (imp *GoodreadsImporter) Preview(ctx context.Context, rows []GoodreadsRow, opts GoodreadsImportOptions) (*GoodreadsPreview, error) {
	imp.mu.Lock()
	if imp.resolving {
		imp.mu.Unlock()
		return nil, ErrGoodreadsRunning
	}
	imp.resolving = true
	imp.evictExpiredLocked()
	imp.mu.Unlock()

	defer func() {
		imp.mu.Lock()
		imp.resolving = false
		imp.mu.Unlock()
	}()

	resolved := imp.resolveFn(ctx, rows, opts, imp.pacing)

	preview := &GoodreadsPreview{
		Token:       newGoodreadsToken(),
		TotalRows:   len(rows),
		ShelfFilter: opts.shelfList(),
		Rows:        resolved,
	}
	summarisePreview(preview)

	imp.mu.Lock()
	imp.previews[preview.Token] = goodreadsPreviewEntry{preview: preview, createdAt: time.Now()}
	imp.mu.Unlock()

	return preview, nil
}

// Commit persists the resolved books from a previously issued preview token.
// The preview is consumed on success (and on a not-found token) so a second
// commit cannot double-add. Returns ErrGoodreadsPreviewNotFound for an
// unknown or expired token.
func (imp *GoodreadsImporter) Commit(ctx context.Context, token string) (*GoodreadsCommitResult, error) {
	imp.mu.Lock()
	imp.evictExpiredLocked()
	entry, ok := imp.previews[token]
	if ok {
		delete(imp.previews, token) // consume — a token commits at most once
	}
	imp.mu.Unlock()

	if !ok {
		return nil, ErrGoodreadsPreviewNotFound
	}

	result := CommitGoodreadsImport(ctx, entry.preview.Rows, imp.authors, imp.books)
	return &result, nil
}

// evictExpiredLocked drops previews older than the TTL. Caller holds imp.mu.
func (imp *GoodreadsImporter) evictExpiredLocked() {
	cutoff := time.Now().Add(-goodreadsPreviewTTL)
	for token, entry := range imp.previews {
		if entry.createdAt.Before(cutoff) {
			delete(imp.previews, token)
		}
	}
}

// newGoodreadsToken returns a random hex token for a preview. Collision odds
// are negligible; a clash would only ever shadow one stale preview.
func newGoodreadsToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is effectively impossible on supported
		// platforms; fall back to a timestamp so the flow still works.
		return "gr-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

// compile-time assertion that the aggregator satisfies the resolver contract.
var _ goodreadsResolver = (*metadata.Aggregator)(nil)
