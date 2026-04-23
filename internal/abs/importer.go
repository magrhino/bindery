// Package abs provides Audiobookshelf client, normalization, and import logic.
package abs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/vavallee/bindery/internal/db"
	bindimporter "github.com/vavallee/bindery/internal/importer"
	"github.com/vavallee/bindery/internal/indexer"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/textutil"
)

const (
	DefaultSourceID             = "default"
	SettingABSLastImportAt      = "abs.last_import_at"
	settingDefaultRootID        = "library.defaultRootFolderId"
	entityTypeAuthor            = "author"
	entityTypeBook              = "book"
	entityTypeSeries            = "series"
	entityTypeEdition           = "edition"
	providerAudiobookshelf      = "audiobookshelf"
	runStatusCompleted          = "completed"
	runStatusFailed             = "failed"
	runStatusRolledBack         = "rolled_back"
	itemOutcomeCreated          = "created"
	itemOutcomeLinked           = "linked"
	itemOutcomeUpdated          = "updated"
	itemOutcomeSkipped          = "skipped"
	itemOutcomeFailed           = "failed"
	reviewReasonUnmatchedAuthor = "unmatched_author"
	reviewReasonAmbiguousAuthor = "ambiguous_author"
	reviewReasonUnmatchedBook   = "unmatched_book"
	reviewReasonAmbiguousBook   = "ambiguous_book"
)

type importClientFactory func(baseURL, apiKey string) (enumerationClient, error)
type enumerateFunc func(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error)

type ImportConfig struct {
	SourceID  string
	BaseURL   string
	APIKey    string
	LibraryID string
	PathRemap string
	Label     string
	Enabled   bool
	DryRun    bool
}

func (c ImportConfig) normalized() ImportConfig {
	c.SourceID = strings.TrimSpace(c.SourceID)
	if c.SourceID == "" {
		c.SourceID = DefaultSourceID
	}
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.LibraryID = strings.TrimSpace(c.LibraryID)
	c.PathRemap = strings.TrimSpace(c.PathRemap)
	c.Label = strings.TrimSpace(c.Label)
	if c.Label == "" {
		c.Label = "Audiobookshelf"
	}
	return c
}

func (c ImportConfig) Validate() error {
	c = c.normalized()
	if !c.Enabled {
		return errors.New("abs source is disabled")
	}
	if c.BaseURL == "" {
		return errors.New("abs base_url is empty")
	}
	if c.APIKey == "" {
		return errors.New("abs api_key is empty")
	}
	if c.LibraryID == "" {
		return errors.New("abs library_id is empty")
	}
	return nil
}

type ImportStats struct {
	LibrariesScanned     int `json:"librariesScanned"`
	PagesScanned         int `json:"pagesScanned"`
	ItemsSeen            int `json:"itemsSeen"`
	ItemsNormalized      int `json:"itemsNormalized"`
	ItemsDetailFetched   int `json:"itemsDetailFetched"`
	AuthorsCreated       int `json:"authorsCreated"`
	AuthorsLinked        int `json:"authorsLinked"`
	BooksCreated         int `json:"booksCreated"`
	BooksLinked          int `json:"booksLinked"`
	BooksUpdated         int `json:"booksUpdated"`
	SeriesCreated        int `json:"seriesCreated"`
	SeriesLinked         int `json:"seriesLinked"`
	EditionsAdded        int `json:"editionsAdded"`
	OwnedMarked          int `json:"ownedMarked"`
	PendingManual        int `json:"pendingManual"`
	ReviewQueued         int `json:"reviewQueued"`
	MetadataMatched      int `json:"metadataMatched"`
	MetadataRelinked     int `json:"metadataRelinked"`
	MetadataConflicts    int `json:"metadataConflicts"`
	MetadataAutoResolved int `json:"metadataAutoResolved"`
	Skipped              int `json:"skipped"`
	Failed               int `json:"failed"`
}

type ImportSourceSnapshot struct {
	SourceID  string `json:"sourceId"`
	Label     string `json:"label"`
	BaseURL   string `json:"baseUrl"`
	LibraryID string `json:"libraryId"`
	PathRemap string `json:"pathRemap,omitempty"`
	Enabled   bool   `json:"enabled"`
	DryRun    bool   `json:"dryRun"`
}

type ImportSummary struct {
	DryRun                bool              `json:"dryRun"`
	ResumedFromCheckpoint bool              `json:"resumedFromCheckpoint"`
	Checkpoint            *ImportCheckpoint `json:"checkpoint,omitempty"`
	Stats                 ImportStats       `json:"stats"`
	Error                 string            `json:"error,omitempty"`
}

type ImportItemResult struct {
	ItemID      string `json:"itemId"`
	Title       string `json:"title"`
	Outcome     string `json:"outcome"`
	Message     string `json:"message,omitempty"`
	MatchedBy   string `json:"matchedBy,omitempty"`
	AuthorID    int64  `json:"authorId,omitempty"`
	BookID      int64  `json:"bookId,omitempty"`
	SeriesCount int    `json:"seriesCount,omitempty"`
}

type ReviewFileMapping struct {
	Found   bool   `json:"found"`
	Message string `json:"message,omitempty"`
}

type ImportProgress struct {
	Running               bool               `json:"running"`
	RunID                 int64              `json:"runId,omitempty"`
	DryRun                bool               `json:"dryRun"`
	StartedAt             time.Time          `json:"startedAt"`
	FinishedAt            *time.Time         `json:"finishedAt,omitempty"`
	Processed             int                `json:"processed"`
	Message               string             `json:"message,omitempty"`
	Error                 string             `json:"error,omitempty"`
	ResumedFromCheckpoint bool               `json:"resumedFromCheckpoint"`
	Checkpoint            *ImportCheckpoint  `json:"checkpoint,omitempty"`
	Stats                 *ImportStats       `json:"stats,omitempty"`
	Results               []ImportItemResult `json:"results,omitempty"`
}

type PersistedImportRun struct {
	ID          int64                `json:"id"`
	SourceID    string               `json:"sourceId"`
	SourceLabel string               `json:"sourceLabel"`
	BaseURL     string               `json:"baseUrl"`
	LibraryID   string               `json:"libraryId"`
	Status      string               `json:"status"`
	DryRun      bool                 `json:"dryRun"`
	StartedAt   time.Time            `json:"startedAt"`
	FinishedAt  *time.Time           `json:"finishedAt,omitempty"`
	Source      ImportSourceSnapshot `json:"source"`
	Checkpoint  *ImportCheckpoint    `json:"checkpoint,omitempty"`
	Summary     ImportSummary        `json:"summary"`
}

type metadataMergeResult struct {
	Matched      int
	Relinked     int
	Conflicts    int
	AutoResolved int
	Messages     []string
}

type Importer struct {
	authors      *db.AuthorRepo
	aliases      *db.AuthorAliasRepo
	books        *db.BookRepo
	editions     *db.EditionRepo
	series       *db.SeriesRepo
	settings     *db.SettingsRepo
	runs         *db.ABSImportRunRepo
	runEntities  *db.ABSImportRunEntityRepo
	provenance   *db.ABSProvenanceRepo
	reviews      *db.ABSReviewItemRepo
	conflicts    *db.ABSMetadataConflictRepo
	meta         *metadata.Aggregator
	newClient    importClientFactory
	enumerateFn  enumerateFunc
	rootFolders  *db.RootFolderRepo
	libraryDir   string
	audiobookDir string

	mu       sync.Mutex
	running  bool
	progress ImportProgress
}

var ErrAlreadyRunning = errors.New("abs import already running")

func NewImporter(
	authors *db.AuthorRepo,
	aliases *db.AuthorAliasRepo,
	books *db.BookRepo,
	editions *db.EditionRepo,
	series *db.SeriesRepo,
	settings *db.SettingsRepo,
	runs *db.ABSImportRunRepo,
	runEntities *db.ABSImportRunEntityRepo,
	provenance *db.ABSProvenanceRepo,
	reviews *db.ABSReviewItemRepo,
	conflicts *db.ABSMetadataConflictRepo,
) *Importer {
	return &Importer{
		authors:     authors,
		aliases:     aliases,
		books:       books,
		editions:    editions,
		series:      series,
		settings:    settings,
		runs:        runs,
		runEntities: runEntities,
		provenance:  provenance,
		reviews:     reviews,
		conflicts:   conflicts,
		newClient: func(baseURL, apiKey string) (enumerationClient, error) {
			return NewClient(baseURL, apiKey)
		},
	}
}

func (i *Importer) WithMetadata(meta *metadata.Aggregator) *Importer {
	i.meta = meta
	return i
}

func (i *Importer) WithStoragePaths(libraryDir, audiobookDir string, rootFolders *db.RootFolderRepo) *Importer {
	i.libraryDir = filepath.Clean(strings.TrimSpace(libraryDir))
	i.audiobookDir = filepath.Clean(strings.TrimSpace(audiobookDir))
	if i.audiobookDir == "." || i.audiobookDir == "" {
		i.audiobookDir = i.libraryDir
	}
	i.rootFolders = rootFolders
	return i
}

func (i *Importer) Progress() ImportProgress {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.progress
}

func (i *Importer) Running() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.running
}

type reviewRequiredError struct {
	Reason string
}

func (e reviewRequiredError) Error() string {
	switch e.Reason {
	case reviewReasonAmbiguousAuthor:
		return "author match is ambiguous"
	case reviewReasonUnmatchedAuthor:
		return "author match not found"
	case reviewReasonAmbiguousBook:
		return "book match is ambiguous"
	case reviewReasonUnmatchedBook:
		return "book match not found"
	default:
		return "review required"
	}
}

func (i *Importer) ImportReview(ctx context.Context, cfg ImportConfig, item NormalizedLibraryItem) (ImportItemResult, error) {
	stats := &ImportStats{}
	result := i.importOne(ctx, cfg, 0, item, stats, true)
	if result.Outcome == itemOutcomeFailed {
		if result.Message == "" {
			return result, errors.New("review import failed")
		}
		return result, errors.New(result.Message)
	}
	return result, nil
}

func (i *Importer) ReviewFileMapping(ctx context.Context, cfg ImportConfig, item NormalizedLibraryItem) ReviewFileMapping {
	var messages []string
	if path := strings.TrimSpace(item.EbookPath); path != "" {
		ok, message := i.inspectFormatPath(ctx, cfg, models.MediaTypeEbook, path)
		if ok {
			return ReviewFileMapping{Found: true, Message: "ebook file is visible to Bindery"}
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	if path := strings.TrimSpace(item.Path); path != "" && len(item.AudioFiles) > 0 {
		ok, message := i.inspectFormatPath(ctx, cfg, models.MediaTypeAudiobook, path)
		if ok {
			return ReviewFileMapping{Found: true, Message: "audiobook path is visible to Bindery"}
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return ReviewFileMapping{Message: "no ABS file paths available"}
	}
	return ReviewFileMapping{Message: strings.Join(messages, "; ")}
}

func (i *Importer) Start(ctx context.Context, cfg ImportConfig) error {
	i.mu.Lock()
	if i.running {
		i.mu.Unlock()
		return ErrAlreadyRunning
	}
	i.running = true
	i.progress = ImportProgress{
		Running:   true,
		DryRun:    cfg.DryRun,
		StartedAt: time.Now().UTC(),
		Message:   importStartMessage(cfg.DryRun),
	}
	i.mu.Unlock()

	go i.run(ctx, cfg)
	return nil
}

func (i *Importer) Run(ctx context.Context, cfg ImportConfig) (*ImportStats, error) {
	i.mu.Lock()
	if i.running {
		i.mu.Unlock()
		return nil, ErrAlreadyRunning
	}
	i.running = true
	i.progress = ImportProgress{
		Running:   true,
		DryRun:    cfg.DryRun,
		StartedAt: time.Now().UTC(),
		Message:   importStartMessage(cfg.DryRun),
	}
	i.mu.Unlock()

	stats := i.run(ctx, cfg)
	progress := i.Progress()
	if progress.Error != "" {
		return stats, errors.New(progress.Error)
	}
	return stats, nil
}

func (i *Importer) run(ctx context.Context, cfg ImportConfig) *ImportStats {
	stats := &ImportStats{}
	cfg = cfg.normalized()
	summary := ImportSummary{DryRun: cfg.DryRun, Stats: *stats}
	defer func() {
		now := time.Now().UTC()
		i.mu.Lock()
		i.running = false
		i.progress.Running = false
		i.progress.FinishedAt = &now
		i.progress.Stats = stats
		i.mu.Unlock()
	}()

	if err := cfg.Validate(); err != nil {
		i.fail(err)
		return stats
	}

	if checkpoint, err := loadImportCheckpoint(ctx, i.settings); err == nil && checkpoint != nil && checkpoint.LibraryID == cfg.LibraryID {
		i.setProgress(func(p *ImportProgress) {
			p.ResumedFromCheckpoint = true
			p.Checkpoint = checkpoint
			if checkpoint.LastItemID != "" {
				p.Message = fmt.Sprintf("resuming from page %d after %s", checkpoint.Page, checkpoint.LastItemID)
			} else {
				p.Message = fmt.Sprintf("resuming from page %d", checkpoint.Page)
			}
		})
		summary.ResumedFromCheckpoint = true
		summary.Checkpoint = checkpoint
	}

	run := &models.ABSImportRun{
		SourceID:         cfg.SourceID,
		SourceLabel:      cfg.Label,
		BaseURL:          cfg.BaseURL,
		LibraryID:        cfg.LibraryID,
		Status:           "running",
		DryRun:           cfg.DryRun,
		SourceConfigJSON: mustJSON(sourceSnapshot(cfg)),
		CheckpointJSON:   mustJSON(summary.Checkpoint),
		SummaryJSON:      "{}",
	}
	if i.runs != nil {
		if err := i.runs.Create(ctx, run); err != nil {
			i.fail(err)
			return stats
		}
		i.setProgress(func(p *ImportProgress) { p.RunID = run.ID })
	}

	enumFn, err := i.resolveEnumerator(cfg, run.ID)
	if err != nil {
		if run.ID != 0 && i.runs != nil {
			summary.Error = err.Error()
			_ = i.runs.Finish(ctx, run.ID, runStatusFailed, summary)
		}
		i.fail(err)
		return stats
	}

	enumStats, err := enumFn(ctx, cfg.LibraryID, func(ctx context.Context, item NormalizedLibraryItem) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		i.setProgress(func(p *ImportProgress) {
			p.Message = importItemMessage(cfg.DryRun, firstNonEmpty(item.Title, item.ItemID))
		})
		result := i.importOne(ctx, cfg, run.ID, item, stats, allowImmediateImport(item))
		i.setProgress(func(p *ImportProgress) {
			p.Processed++
			p.Results = append(p.Results, result)
		})
		return nil
	})
	if err != nil {
		if run.ID != 0 && i.runs != nil {
			if checkpoint, checkpointErr := loadImportCheckpoint(ctx, i.settings); checkpointErr == nil {
				summary.Checkpoint = checkpoint
			}
			summary.Stats = *stats
			summary.Error = err.Error()
			_ = i.runs.Finish(ctx, run.ID, runStatusFailed, summary)
		}
		i.fail(err)
		return stats
	}

	stats.LibrariesScanned = 1
	stats.PagesScanned = enumStats.PagesScanned
	stats.ItemsSeen = enumStats.ItemsSeen
	stats.ItemsNormalized = enumStats.ItemsNormalized
	stats.ItemsDetailFetched = enumStats.ItemsDetailFetched

	if i.settings != nil && !cfg.DryRun {
		if err := i.settings.Set(ctx, SettingABSLastImportAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
			slog.Warn("abs import: persist last_import_at failed", "error", err)
		}
	}
	summary.Checkpoint = nil
	summary.Stats = *stats
	if run.ID != 0 && i.runs != nil {
		if err := i.runs.Finish(ctx, run.ID, runStatusCompleted, summary); err != nil {
			slog.Warn("abs import: finish run failed", "runID", run.ID, "error", err)
		}
	}
	slog.Info("abs import complete",
		"libraryID", cfg.LibraryID,
		"dryRun", cfg.DryRun,
		"pagesScanned", stats.PagesScanned,
		"itemsSeen", stats.ItemsSeen,
		"authorsCreated", stats.AuthorsCreated,
		"booksCreated", stats.BooksCreated,
		"booksLinked", stats.BooksLinked,
		"booksUpdated", stats.BooksUpdated,
		"seriesCreated", stats.SeriesCreated,
		"seriesLinked", stats.SeriesLinked,
		"editionsAdded", stats.EditionsAdded,
		"skipped", stats.Skipped,
		"failed", stats.Failed)
	i.setProgress(func(p *ImportProgress) {
		p.Checkpoint = nil
		p.Message = importDoneMessage(cfg.DryRun)
	})
	return stats
}

func (i *Importer) resolveEnumerator(cfg ImportConfig, runID int64) (enumerateFunc, error) {
	if i.enumerateFn != nil {
		return i.enumerateFn, nil
	}
	client, err := i.newClient(cfg.BaseURL, cfg.APIKey)
	if err != nil {
		return nil, err
	}
	enumerator := NewEnumerator(client, i.settings, 50).WithCheckpointObserver(func(checkpoint ImportCheckpoint) {
		cp := checkpoint
		i.setProgress(func(p *ImportProgress) {
			p.Checkpoint = &cp
		})
		if i.runs != nil && runID != 0 {
			if err := i.runs.UpdateCheckpoint(context.Background(), runID, checkpoint); err != nil {
				slog.Warn("abs import: persist checkpoint failed", "runID", runID, "error", err)
			}
		}
	})
	return enumerator.Enumerate, nil
}

func (i *Importer) importOne(ctx context.Context, cfg ImportConfig, runID int64, item NormalizedLibraryItem, stats *ImportStats, allowCreate bool) ImportItemResult {
	result := ImportItemResult{
		ItemID:  item.ItemID,
		Title:   item.Title,
		Outcome: itemOutcomeUpdated,
	}
	author, authorCreated, authorMatchedBy, authorMeta, err := i.resolveAuthor(ctx, cfg, runID, item, allowCreate)
	if err != nil {
		var reviewErr reviewRequiredError
		if errors.As(err, &reviewErr) {
			if queueErr := i.queueReviewItem(ctx, runID, cfg, item, reviewErr.Reason); queueErr != nil {
				stats.Failed++
				result.Outcome = itemOutcomeFailed
				result.Message = queueErr.Error()
				return result
			}
			stats.ReviewQueued++
			result.Outcome = itemOutcomeSkipped
			result.Message = reviewQueueMessage(reviewErr.Reason, item)
			return result
		}
		stats.Failed++
		result.Outcome = itemOutcomeFailed
		result.Message = err.Error()
		return result
	}
	stats.MetadataMatched += authorMeta.Matched
	stats.MetadataRelinked += authorMeta.Relinked
	stats.MetadataConflicts += authorMeta.Conflicts
	stats.MetadataAutoResolved += authorMeta.AutoResolved
	result.AuthorID = author.ID
	if authorCreated {
		stats.AuthorsCreated++
	} else {
		stats.AuthorsLinked++
	}
	result.MatchedBy = authorMatchedBy

	if !cfg.DryRun {
		i.recordSecondaryAuthors(ctx, author.ID, item.Authors[1:])
	}

	bookResult, created, linked, bookMeta, err := i.upsertBook(ctx, cfg, runID, author, item, allowCreate)
	if err != nil {
		var reviewErr reviewRequiredError
		if errors.As(err, &reviewErr) {
			if queueErr := i.queueReviewItem(ctx, runID, cfg, item, reviewErr.Reason); queueErr != nil {
				stats.Failed++
				result.Outcome = itemOutcomeFailed
				result.Message = queueErr.Error()
				return result
			}
			stats.ReviewQueued++
			result.Outcome = itemOutcomeSkipped
			result.Message = reviewQueueMessage(reviewErr.Reason, item)
			return result
		}
		stats.Failed++
		result.Outcome = itemOutcomeFailed
		result.Message = err.Error()
		return result
	}
	stats.MetadataMatched += bookMeta.Matched
	stats.MetadataRelinked += bookMeta.Relinked
	stats.MetadataConflicts += bookMeta.Conflicts
	stats.MetadataAutoResolved += bookMeta.AutoResolved
	result.BookID = bookResult.row.ID
	if created {
		stats.BooksCreated++
		result.Outcome = itemOutcomeCreated
	} else if linked {
		stats.BooksLinked++
		result.Outcome = itemOutcomeLinked
	} else {
		stats.BooksUpdated++
		result.Outcome = itemOutcomeUpdated
	}
	if result.MatchedBy == "" {
		result.MatchedBy = bookResult.matchedBy
	}
	if !cfg.DryRun {
		i.enrichAudiobookFromASIN(ctx, bookResult.row)
	}

	seriesCount := 0
	for _, series := range item.Series {
		created, matchedBy, err := i.upsertSeries(ctx, cfg, runID, bookResult.row.ID, series)
		if err != nil {
			slog.Warn("abs import: series upsert failed", "itemID", item.ItemID, "series", series.Name, "error", err)
			continue
		}
		seriesCount++
		if created {
			stats.SeriesCreated++
		} else if matchedBy != "" {
			stats.SeriesLinked++
		}
	}
	result.SeriesCount = seriesCount

	addedEditions, err := i.upsertEditions(ctx, cfg, runID, bookResult.row.ID, item)
	if err != nil {
		slog.Warn("abs import: edition upsert failed", "itemID", item.ItemID, "error", err)
	} else {
		stats.EditionsAdded += addedEditions
	}

	reconcile := i.reconcileOwnedState(ctx, cfg, author, bookResult.row, item)
	stats.OwnedMarked += reconcile.OwnedMarked
	stats.PendingManual += reconcile.PendingManual
	messages := append([]string{}, authorMeta.Messages...)
	messages = append(messages, bookMeta.Messages...)
	if reconcile.Message != "" {
		messages = append(messages, reconcile.Message)
	}
	if len(messages) > 0 {
		result.Message = strings.Join(messages, "; ")
	}

	return result
}

func reviewQueueMessage(reason string, item NormalizedLibraryItem) string {
	switch reason {
	case reviewReasonUnmatchedAuthor:
		return fmt.Sprintf("queued for review: no confident author match for %q", primaryAuthorName(item))
	case reviewReasonAmbiguousAuthor:
		return fmt.Sprintf("queued for review: multiple author matches for %q", primaryAuthorName(item))
	case reviewReasonUnmatchedBook:
		return fmt.Sprintf("queued for review: no confident book match for %q", strings.TrimSpace(item.Title))
	case reviewReasonAmbiguousBook:
		return fmt.Sprintf("queued for review: multiple book matches for %q", strings.TrimSpace(item.Title))
	default:
		return "queued for review"
	}
}

func (i *Importer) enrichAudiobookFromASIN(ctx context.Context, book *models.Book) {
	if i.meta == nil || i.books == nil || book == nil {
		return
	}
	if strings.TrimSpace(book.ASIN) == "" {
		return
	}
	if book.MediaType != models.MediaTypeAudiobook && book.MediaType != models.MediaTypeBoth {
		return
	}
	if err := i.meta.EnrichAudiobook(ctx, book); err != nil {
		slog.Debug("abs import: audnex enrichment skipped", "bookID", book.ID, "asin", book.ASIN, "error", err)
		return
	}
	if err := i.books.Update(ctx, book); err != nil {
		slog.Warn("abs import: persisting audnex enrichment failed", "bookID", book.ID, "asin", book.ASIN, "error", err)
	}
}

func (i *Importer) queueReviewItem(ctx context.Context, runID int64, cfg ImportConfig, item NormalizedLibraryItem, reason string) error {
	if i.reviews == nil {
		return reviewRequiredError{Reason: reason}
	}
	review := &models.ABSReviewItem{
		SourceID:      cfg.SourceID,
		LibraryID:     item.LibraryID,
		ItemID:        item.ItemID,
		Title:         strings.TrimSpace(item.Title),
		PrimaryAuthor: primaryAuthorName(item),
		ASIN:          strings.TrimSpace(item.ASIN),
		MediaType:     deriveMediaType(item),
		ReviewReason:  reason,
		PayloadJSON:   mustJSON(item),
		Status:        "pending",
	}
	if runID != 0 {
		review.LatestRunID = ptrInt64(runID)
	}
	return i.reviews.UpsertPending(ctx, review)
}

type ownershipReconcileResult struct {
	OwnedMarked   int
	PendingManual int
	Message       string
}

func (i *Importer) reconcileOwnedState(ctx context.Context, cfg ImportConfig, author *models.Author, book *models.Book, item NormalizedLibraryItem) ownershipReconcileResult {
	if i.books == nil || book == nil {
		return ownershipReconcileResult{}
	}

	var (
		reconcileMessages []string
		ownedMarked       int
		pendingManual     int
	)

	if ebookPath := strings.TrimSpace(item.EbookPath); ebookPath != "" {
		ok, changed, message := i.reconcileFormatPath(ctx, cfg, author, book, models.MediaTypeEbook, ebookPath)
		if ok {
			if changed {
				ownedMarked++
			}
			reconcileMessages = append(reconcileMessages, "ebook verified")
		} else {
			pendingManual++
			reconcileMessages = append(reconcileMessages, message)
		}
	}

	if audiobookPath := strings.TrimSpace(item.Path); audiobookPath != "" && len(item.AudioFiles) > 0 {
		ok, changed, message := i.reconcileFormatPath(ctx, cfg, author, book, models.MediaTypeAudiobook, audiobookPath)
		if ok {
			if changed {
				ownedMarked++
			}
			reconcileMessages = append(reconcileMessages, "audiobook verified")
		} else {
			pendingManual++
			reconcileMessages = append(reconcileMessages, message)
		}
	}

	if ownedMarked == 0 && pendingManual == 0 {
		return ownershipReconcileResult{}
	}

	return ownershipReconcileResult{
		OwnedMarked:   ownedMarked,
		PendingManual: pendingManual,
		Message:       strings.Join(reconcileMessages, "; "),
	}
}

func (i *Importer) reconcileFormatPath(ctx context.Context, cfg ImportConfig, author *models.Author, book *models.Book, format, candidatePath string) (bool, bool, string) {
	remappedPath := i.remapABSPath(cfg, candidatePath)
	cleanPath := filepath.Clean(remappedPath)
	if cleanPath == "." || cleanPath == "" {
		return false, false, fmt.Sprintf("%s path missing from ABS metadata; imported metadata only", format)
	}
	if !i.pathAllowedForBook(ctx, author, format, cleanPath) {
		if remappedPath != strings.TrimSpace(candidatePath) {
			return false, false, fmt.Sprintf("%s path %q remapped to %q but is still outside Bindery storage; imported metadata only", format, strings.TrimSpace(candidatePath), cleanPath)
		}
		return false, false, fmt.Sprintf("%s path %q is outside Bindery storage; imported metadata only", format, cleanPath)
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		if remappedPath != strings.TrimSpace(candidatePath) {
			return false, false, fmt.Sprintf("%s path %q remapped to %q is not visible to Bindery; imported metadata only", format, strings.TrimSpace(candidatePath), cleanPath)
		}
		return false, false, fmt.Sprintf("%s path %q is not visible to Bindery; imported metadata only", format, cleanPath)
	}
	if format == models.MediaTypeEbook && info.IsDir() {
		return false, false, fmt.Sprintf("%s path %q is a directory; imported metadata only", format, cleanPath)
	}

	alreadyTracked, err := i.bookAlreadyTracksPath(ctx, book.ID, format, cleanPath)
	if err != nil {
		slog.Warn("abs import: file reconciliation lookup failed", "bookID", book.ID, "format", format, "path", cleanPath, "error", err)
		return false, false, fmt.Sprintf("%s verification could not inspect existing Bindery files; imported metadata only", format)
	}
	if cfg.DryRun {
		return true, !alreadyTracked, ""
	}
	if err := i.books.SetFormatFilePath(ctx, book.ID, format, cleanPath); err != nil {
		slog.Warn("abs import: file reconciliation failed", "bookID", book.ID, "format", format, "path", cleanPath, "error", err)
		return false, false, fmt.Sprintf("%s path %q could not be registered in Bindery; imported metadata only", format, cleanPath)
	}
	return true, !alreadyTracked, ""
}

func (i *Importer) inspectFormatPath(ctx context.Context, cfg ImportConfig, format, candidatePath string) (bool, string) {
	remappedPath := i.remapABSPath(cfg, candidatePath)
	cleanPath := filepath.Clean(remappedPath)
	if cleanPath == "." || cleanPath == "" {
		return false, fmt.Sprintf("%s path missing from ABS metadata", format)
	}
	if !i.pathAllowedForBook(ctx, nil, format, cleanPath) {
		if remappedPath != strings.TrimSpace(candidatePath) {
			return false, fmt.Sprintf("%s path %q remapped to %q but is outside Bindery storage", format, strings.TrimSpace(candidatePath), cleanPath)
		}
		return false, fmt.Sprintf("%s path %q is outside Bindery storage", format, cleanPath)
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		if remappedPath != strings.TrimSpace(candidatePath) {
			return false, fmt.Sprintf("%s path %q remapped to %q is not visible to Bindery", format, strings.TrimSpace(candidatePath), cleanPath)
		}
		return false, fmt.Sprintf("%s path %q is not visible to Bindery", format, cleanPath)
	}
	if format == models.MediaTypeEbook && info.IsDir() {
		return false, fmt.Sprintf("%s path %q is a directory", format, cleanPath)
	}
	return true, ""
}

func (i *Importer) remapABSPath(cfg ImportConfig, candidatePath string) string {
	candidatePath = strings.TrimSpace(candidatePath)
	if candidatePath == "" || strings.TrimSpace(cfg.PathRemap) == "" {
		return candidatePath
	}
	return bindimporter.ParseRemap(cfg.PathRemap).Apply(candidatePath)
}

func (i *Importer) bookAlreadyTracksPath(ctx context.Context, bookID int64, format, path string) (bool, error) {
	files, err := i.books.ListFiles(ctx, bookID)
	if err != nil {
		return false, err
	}
	cleanPath := filepath.Clean(path)
	for _, file := range files {
		if file.Format == format && filepath.Clean(file.Path) == cleanPath {
			return true, nil
		}
	}
	return false, nil
}

func (i *Importer) pathAllowedForBook(ctx context.Context, author *models.Author, format, path string) bool {
	for _, root := range i.allowedRootsForBook(ctx, author, format) {
		if root == "" {
			continue
		}
		if pathUnderDir(path, root) {
			return true
		}
	}
	return false
}

func (i *Importer) allowedRootsForBook(ctx context.Context, author *models.Author, format string) []string {
	roots := make([]string, 0, 3)
	if format == models.MediaTypeAudiobook {
		if root := strings.TrimSpace(i.audiobookDir); root != "" {
			roots = append(roots, filepath.Clean(root))
		}
	}
	if root := strings.TrimSpace(i.effectiveLibraryDir(ctx, author)); root != "" {
		roots = append(roots, filepath.Clean(root))
	}
	if format == models.MediaTypeAudiobook {
		if root := strings.TrimSpace(i.libraryDir); root != "" {
			roots = append(roots, filepath.Clean(root))
		}
	}
	return dedupeCleanPaths(roots)
}

func (i *Importer) effectiveLibraryDir(ctx context.Context, author *models.Author) string {
	if author != nil && author.RootFolderID != nil && i.rootFolders != nil {
		if root, err := i.rootFolders.GetByID(ctx, *author.RootFolderID); err == nil && root != nil {
			return root.Path
		}
	}
	if i.settings != nil && i.rootFolders != nil {
		if setting, err := i.settings.Get(ctx, settingDefaultRootID); err == nil && setting != nil && strings.TrimSpace(setting.Value) != "" {
			if id, err := strconv.ParseInt(strings.TrimSpace(setting.Value), 10, 64); err == nil && id > 0 {
				if root, err := i.rootFolders.GetByID(ctx, id); err == nil && root != nil {
					return root.Path
				}
			}
		}
	}
	return i.libraryDir
}

type bookUpsertResult struct {
	row       *models.Book
	matchedBy string
}

func (i *Importer) resolveAuthor(ctx context.Context, cfg ImportConfig, runID int64, item NormalizedLibraryItem, allowCreate bool) (*models.Author, bool, string, metadataMergeResult, error) {
	if len(item.Authors) == 0 {
		return nil, false, "", metadataMergeResult{}, errors.New("item has no authors")
	}
	primary := item.Authors[0]
	name := strings.TrimSpace(primary.Name)
	if name == "" {
		return nil, false, "", metadataMergeResult{}, errors.New("primary author name is empty")
	}
	if strings.TrimSpace(item.ResolvedAuthorForeignID) != "" || strings.TrimSpace(item.ResolvedAuthorName) != "" {
		return i.resolveManualAuthor(ctx, cfg, runID, item)
	}
	externalID := authorExternalID(primary)
	if i.provenance != nil {
		if link, err := i.provenance.GetByExternal(ctx, cfg.SourceID, item.LibraryID, entityTypeAuthor, externalID); err != nil {
			return nil, false, "", metadataMergeResult{}, err
		} else if link != nil {
			existing, err := i.authors.GetByID(ctx, link.LocalID)
			if err != nil {
				return nil, false, "", metadataMergeResult{}, err
			}
			if existing != nil {
				if !cfg.DryRun {
					if err := i.upsertProvenance(ctx, &models.ABSProvenance{
						SourceID:    cfg.SourceID,
						LibraryID:   item.LibraryID,
						EntityType:  entityTypeAuthor,
						ExternalID:  externalID,
						LocalID:     existing.ID,
						ItemID:      item.ItemID,
						ImportRunID: ptrInt64(runID),
					}); err != nil {
						return nil, false, "", metadataMergeResult{}, err
					}
				}
				_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, existing.ID, itemOutcomeLinked, nil)
				if cfg.DryRun {
					return existing, false, "provenance", metadataMergeResult{}, nil
				}
				metaResult, err := i.enrichAuthor(ctx, cfg, item, existing)
				return existing, false, "provenance", metaResult, err
			}
		}
	}

	if existing, matchedBy, ambiguous, err := i.findAuthorByName(ctx, name); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	} else if existing != nil {
		if !cfg.DryRun {
			if err := i.upsertProvenance(ctx, &models.ABSProvenance{
				SourceID:    cfg.SourceID,
				LibraryID:   item.LibraryID,
				EntityType:  entityTypeAuthor,
				ExternalID:  externalID,
				LocalID:     existing.ID,
				ItemID:      item.ItemID,
				ImportRunID: ptrInt64(runID),
			}); err != nil {
				return nil, false, "", metadataMergeResult{}, err
			}
			if matchedBy == "normalized_name" {
				i.recordAuthorVariantAlias(ctx, existing.ID, name)
			}
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, existing.ID, itemOutcomeLinked, nil)
		if cfg.DryRun {
			return existing, false, matchedBy, metadataMergeResult{}, nil
		}
		metaResult, err := i.enrichAuthor(ctx, cfg, item, existing)
		return existing, false, matchedBy, metaResult, err
	} else if ambiguous && !allowCreate {
		return nil, false, "", metadataMergeResult{}, reviewRequiredError{Reason: reviewReasonAmbiguousAuthor}
	}

	if !allowCreate {
		return nil, false, "", metadataMergeResult{}, reviewRequiredError{Reason: reviewReasonUnmatchedAuthor}
	}

	if cfg.DryRun {
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, 0, itemOutcomeCreated, nil)
		return &models.Author{
			ForeignID:        absForeignID("author", item.LibraryID, externalID),
			Name:             name,
			SortName:         sortNameFromFull(name),
			Monitored:        true,
			MetadataProvider: providerAudiobookshelf,
		}, true, "created", metadataMergeResult{}, nil
	}

	author := &models.Author{
		ForeignID:        absForeignID("author", item.LibraryID, externalID),
		Name:             name,
		SortName:         sortNameFromFull(name),
		Monitored:        true,
		MetadataProvider: providerAudiobookshelf,
	}
	if err := i.authors.Create(ctx, author); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	}
	if err := i.upsertProvenance(ctx, &models.ABSProvenance{
		SourceID:    cfg.SourceID,
		LibraryID:   item.LibraryID,
		EntityType:  entityTypeAuthor,
		ExternalID:  externalID,
		LocalID:     author.ID,
		ItemID:      item.ItemID,
		ImportRunID: ptrInt64(runID),
	}); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	}
	_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, author.ID, itemOutcomeCreated, nil)
	metaResult, err := i.enrichAuthor(ctx, cfg, item, author)
	return author, true, "created", metaResult, err
}

func (i *Importer) resolveManualAuthor(ctx context.Context, cfg ImportConfig, runID int64, item NormalizedLibraryItem) (*models.Author, bool, string, metadataMergeResult, error) {
	primary := item.Authors[0]
	absName := strings.TrimSpace(primary.Name)
	absExternalID := authorExternalID(primary)
	foreignID := strings.TrimSpace(item.ResolvedAuthorForeignID)
	name := strings.TrimSpace(item.ResolvedAuthorName)
	if foreignID == "" || name == "" {
		return nil, false, "", metadataMergeResult{}, errors.New("resolved author requires foreignAuthorId and authorName")
	}

	if existing, err := i.authors.GetByForeignID(ctx, foreignID); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	} else if existing != nil {
		if !cfg.DryRun {
			if err := i.upsertProvenance(ctx, &models.ABSProvenance{
				SourceID:    cfg.SourceID,
				LibraryID:   item.LibraryID,
				EntityType:  entityTypeAuthor,
				ExternalID:  absExternalID,
				LocalID:     existing.ID,
				ItemID:      item.ItemID,
				ImportRunID: ptrInt64(runID),
			}); err != nil {
				return nil, false, "", metadataMergeResult{}, err
			}
			if normalizeAuthorName(absName) != normalizeAuthorName(existing.Name) {
				i.recordAuthorVariantAlias(ctx, existing.ID, absName)
			}
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, absExternalID, existing.ID, itemOutcomeLinked, map[string]string{"matchedBy": "manual_author"})
		return existing, false, "manual_author", metadataMergeResult{}, nil
	}

	if existing, _, ambiguous, err := i.findAuthorByName(ctx, name); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	} else if ambiguous {
		return nil, false, "", metadataMergeResult{}, reviewRequiredError{Reason: reviewReasonAmbiguousAuthor}
	} else if existing != nil {
		if !cfg.DryRun {
			if existing.ForeignID == "" || strings.HasPrefix(existing.ForeignID, "abs:") {
				existing.ForeignID = foreignID
				if existing.MetadataProvider == "" || existing.MetadataProvider == providerAudiobookshelf {
					existing.MetadataProvider = "openlibrary"
				}
				if err := i.authors.Update(ctx, existing); err != nil {
					return nil, false, "", metadataMergeResult{}, err
				}
			}
			if err := i.upsertProvenance(ctx, &models.ABSProvenance{
				SourceID:    cfg.SourceID,
				LibraryID:   item.LibraryID,
				EntityType:  entityTypeAuthor,
				ExternalID:  absExternalID,
				LocalID:     existing.ID,
				ItemID:      item.ItemID,
				ImportRunID: ptrInt64(runID),
			}); err != nil {
				return nil, false, "", metadataMergeResult{}, err
			}
			if normalizeAuthorName(absName) != normalizeAuthorName(existing.Name) {
				i.recordAuthorVariantAlias(ctx, existing.ID, absName)
			}
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, absExternalID, existing.ID, itemOutcomeLinked, map[string]string{"matchedBy": "manual_author_name"})
		return existing, false, "manual_author", metadataMergeResult{}, nil
	}

	author := &models.Author{
		ForeignID:        foreignID,
		Name:             name,
		SortName:         sortNameFromFull(name),
		Monitored:        true,
		MetadataProvider: "openlibrary",
	}
	if i.meta != nil && !cfg.DryRun {
		if full, err := i.meta.GetAuthor(ctx, foreignID); err == nil && full != nil {
			author = full
			if author.Name == "" {
				author.Name = name
			}
			if author.SortName == "" {
				author.SortName = sortNameFromFull(author.Name)
			}
			author.ForeignID = foreignID
			author.Monitored = true
			if author.MetadataProvider == "" {
				author.MetadataProvider = "openlibrary"
			}
		} else if err != nil {
			slog.Warn("abs import: manual author metadata fetch failed", "foreignID", foreignID, "error", err)
		}
	}
	if cfg.DryRun {
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, absExternalID, 0, itemOutcomeCreated, map[string]string{"matchedBy": "manual_author"})
		return author, true, "manual_author", metadataMergeResult{}, nil
	}
	if err := i.authors.Create(ctx, author); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	}
	if err := i.upsertProvenance(ctx, &models.ABSProvenance{
		SourceID:    cfg.SourceID,
		LibraryID:   item.LibraryID,
		EntityType:  entityTypeAuthor,
		ExternalID:  absExternalID,
		LocalID:     author.ID,
		ItemID:      item.ItemID,
		ImportRunID: ptrInt64(runID),
	}); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	}
	if normalizeAuthorName(absName) != normalizeAuthorName(author.Name) {
		i.recordAuthorVariantAlias(ctx, author.ID, absName)
	}
	_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, absExternalID, author.ID, itemOutcomeCreated, map[string]string{"matchedBy": "manual_author"})
	return author, true, "manual_author", metadataMergeResult{}, nil
}

func (i *Importer) findAuthorByName(ctx context.Context, name string) (*models.Author, string, bool, error) {
	all, err := i.authors.List(ctx)
	if err != nil {
		return nil, "", false, err
	}
	aliases := []models.AuthorAlias{}
	if i.aliases != nil {
		loaded, err := i.aliases.List(ctx)
		if err != nil {
			return nil, "", false, err
		}
		aliases = loaded
	}

	needle := strings.ToLower(strings.TrimSpace(name))
	exact := make(map[int64]*models.Author)
	for idx := range all {
		if strings.ToLower(strings.TrimSpace(all[idx].Name)) == needle {
			copy := all[idx]
			exact[copy.ID] = &copy
		}
	}
	for _, alias := range aliases {
		if strings.ToLower(strings.TrimSpace(alias.Name)) != needle {
			continue
		}
		author, err := i.authors.GetByID(ctx, alias.AuthorID)
		if err != nil {
			return nil, "", false, err
		}
		if author != nil {
			exact[author.ID] = author
		}
	}
	if len(exact) == 1 {
		for _, author := range exact {
			matchedBy := "name"
			if strings.ToLower(strings.TrimSpace(author.Name)) != needle {
				matchedBy = "alias"
			}
			return author, matchedBy, false, nil
		}
	}
	if len(exact) > 1 {
		return nil, "", true, nil
	}

	normNeedle := normalizeAuthorName(name)
	if normNeedle == "" {
		return nil, "", false, nil
	}
	normalized := make(map[int64]*models.Author)
	for idx := range all {
		if normalizeAuthorName(all[idx].Name) != normNeedle {
			continue
		}
		copy := all[idx]
		normalized[copy.ID] = &copy
	}
	for _, alias := range aliases {
		if normalizeAuthorName(alias.Name) != normNeedle {
			continue
		}
		author, err := i.authors.GetByID(ctx, alias.AuthorID)
		if err != nil {
			return nil, "", false, err
		}
		if author != nil {
			normalized[author.ID] = author
		}
	}
	if len(normalized) == 1 {
		for _, author := range normalized {
			return author, "normalized_name", false, nil
		}
	}
	if len(normalized) > 1 {
		return nil, "", true, nil
	}
	return nil, "", false, nil
}

func (i *Importer) recordSecondaryAuthors(ctx context.Context, canonicalID int64, extras []NormalizedAuthor) {
	if canonicalID == 0 {
		return
	}
	for _, author := range extras {
		name := strings.TrimSpace(author.Name)
		if name == "" {
			continue
		}
		if err := i.aliases.Create(ctx, &models.AuthorAlias{AuthorID: canonicalID, Name: name}); err != nil {
			slog.Debug("abs import: alias record skipped", "name", name, "error", err)
		}
	}
}

func (i *Importer) recordAuthorVariantAlias(ctx context.Context, canonicalID int64, name string) {
	if canonicalID == 0 || i.aliases == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if err := i.aliases.Create(ctx, &models.AuthorAlias{AuthorID: canonicalID, Name: name}); err != nil {
		slog.Debug("abs import: author variant alias skipped", "name", name, "error", err)
	}
}

func (i *Importer) enrichAuthor(ctx context.Context, cfg ImportConfig, item NormalizedLibraryItem, author *models.Author) (metadataMergeResult, error) {
	if i.meta == nil || author == nil || len(item.Authors) == 0 {
		return metadataMergeResult{}, nil
	}

	full, ambiguous, err := i.lookupUpstreamAuthor(ctx, item.Authors[0].Name)
	if err != nil {
		slog.Warn("abs import: author metadata lookup failed", "author", item.Authors[0].Name, "error", err)
		return metadataMergeResult{}, nil
	}
	if ambiguous {
		return metadataMergeResult{Messages: []string{"author relink skipped: upstream author match was ambiguous"}}, nil
	}
	if full == nil {
		return metadataMergeResult{}, nil
	}

	result := metadataMergeResult{Matched: 1}
	changed := false
	if name := strings.TrimSpace(full.Name); name != "" && strings.TrimSpace(author.Name) != name {
		oldName := author.Name
		author.Name = name
		if full.SortName != "" {
			author.SortName = full.SortName
		}
		if !cfg.DryRun {
			i.recordAuthorVariantAlias(ctx, author.ID, oldName)
		}
		changed = true
	}
	if full.ForeignID != "" && author.ForeignID != full.ForeignID {
		existing, err := i.authors.GetByForeignID(ctx, full.ForeignID)
		if err != nil {
			return metadataMergeResult{}, err
		}
		if existing != nil && existing.ID != author.ID {
			result.Messages = append(result.Messages, "author relink skipped: upstream author already exists locally")
		} else {
			author.ForeignID = full.ForeignID
			if full.MetadataProvider != "" {
				author.MetadataProvider = full.MetadataProvider
			}
			result.Relinked++
			changed = true
		}
	}
	for _, field := range authorConflictFields {
		fieldResult, fieldChanged, err := i.applyConflictField(ctx, cfg, item, entityTypeAuthor, author.ID, field,
			SerializeAuthorConflictValue(author, field),
			SerializeAuthorConflictValue(full, field),
			func(value string) error { return ApplyAuthorConflictValue(author, field, value) },
			func() string { return SerializeAuthorConflictValue(author, field) },
		)
		if err != nil {
			return metadataMergeResult{}, err
		}
		result.Matched += fieldResult.Matched
		result.Relinked += fieldResult.Relinked
		result.Conflicts += fieldResult.Conflicts
		result.AutoResolved += fieldResult.AutoResolved
		result.Messages = append(result.Messages, fieldResult.Messages...)
		changed = changed || fieldChanged
	}
	if !changed && result.Conflicts == 0 && result.AutoResolved == 0 && result.Relinked == 0 {
		return result, nil
	}
	now := time.Now().UTC()
	author.LastMetadataRefreshAt = &now
	if err := i.authors.Update(ctx, author); err != nil {
		return metadataMergeResult{}, err
	}
	return result, nil
}

func (i *Importer) lookupUpstreamAuthor(ctx context.Context, name string) (*models.Author, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, nil
	}
	want := normalizeAuthorName(name)
	var match *models.Author
	matchedQuery := ""
	for _, query := range authorSearchQueries(name) {
		results, err := i.meta.SearchAuthors(ctx, query)
		if err != nil {
			return nil, false, err
		}
		for idx := range results {
			if normalizeAuthorName(results[idx].Name) != want {
				continue
			}
			if match != nil {
				slog.Info("abs import: upstream author match ambiguous", "author", name, "query", query)
				return nil, true, nil
			}
			copy := results[idx]
			match = &copy
		}
		if match != nil {
			matchedQuery = query
			break
		}
	}
	if match == nil {
		slog.Debug("abs import: upstream author match not found", "author", name, "queries", authorSearchQueries(name))
		return nil, false, nil
	}
	full, err := i.meta.GetAuthor(ctx, match.ForeignID)
	if err != nil {
		return nil, false, err
	}
	slog.Info("abs import: upstream author matched", "author", name, "query", matchedQuery, "foreignId", match.ForeignID)
	return full, false, nil
}

func (i *Importer) enrichBook(ctx context.Context, cfg ImportConfig, item NormalizedLibraryItem, author *models.Author, book *models.Book) (metadataMergeResult, error) {
	if i.meta == nil || book == nil {
		return metadataMergeResult{}, nil
	}

	full, matchedBy, ambiguous, err := i.lookupUpstreamBook(ctx, author, item)
	if err != nil {
		slog.Warn("abs import: book metadata lookup failed", "title", item.Title, "error", err)
		return metadataMergeResult{}, nil
	}
	if ambiguous {
		return metadataMergeResult{Messages: []string{"book relink skipped: upstream book match was ambiguous"}}, nil
	}
	if full == nil {
		return metadataMergeResult{}, nil
	}

	result := metadataMergeResult{Matched: 1}
	changed := false
	if full.ForeignID != "" && book.ForeignID != full.ForeignID {
		existing, err := i.books.GetByForeignID(ctx, full.ForeignID)
		if err != nil {
			return metadataMergeResult{}, err
		}
		if existing != nil && existing.ID != book.ID {
			result.Messages = append(result.Messages, "book relink skipped: upstream book already exists locally")
			full = nil
		} else {
			book.ForeignID = full.ForeignID
			if full.MetadataProvider != "" {
				book.MetadataProvider = full.MetadataProvider
			}
			result.Relinked++
			changed = true
		}
	}
	if full == nil {
		return result, nil
	}
	for _, field := range bookConflictFields {
		fieldResult, fieldChanged, err := i.applyConflictField(ctx, cfg, item, entityTypeBook, book.ID, field,
			bookABSCandidateValue(book, item, field),
			SerializeBookConflictValue(full, field),
			func(value string) error { return ApplyBookConflictValue(book, field, value) },
			func() string { return SerializeBookConflictValue(book, field) },
		)
		if err != nil {
			return metadataMergeResult{}, err
		}
		result.Matched += fieldResult.Matched
		result.Relinked += fieldResult.Relinked
		result.Conflicts += fieldResult.Conflicts
		result.AutoResolved += fieldResult.AutoResolved
		result.Messages = append(result.Messages, fieldResult.Messages...)
		changed = changed || fieldChanged
	}
	if matchedBy != "" && result.Relinked > 0 {
		result.Messages = append(result.Messages, fmt.Sprintf("book relinked by %s metadata match", matchedBy))
	}
	if !changed && result.Conflicts == 0 && result.AutoResolved == 0 && result.Relinked == 0 {
		return result, nil
	}
	now := time.Now().UTC()
	book.LastMetadataRefreshAt = &now
	if err := i.books.Update(ctx, book); err != nil {
		return metadataMergeResult{}, err
	}
	return result, nil
}

func (i *Importer) lookupUpstreamBook(ctx context.Context, author *models.Author, item NormalizedLibraryItem) (*models.Book, string, bool, error) {
	if isbn := isbnDigits(item.ISBN); isbn != "" {
		match, err := i.meta.GetBookByISBN(ctx, isbn)
		if err != nil {
			return nil, "", false, err
		}
		if match != nil {
			full, err := i.meta.GetBook(ctx, match.ForeignID)
			if err != nil {
				return nil, "", false, err
			}
			if full != nil {
				return full, "isbn", false, nil
			}
			return match, "isbn", false, nil
		}
	}
	if author == nil || author.ForeignID == "" || author.MetadataProvider == providerAudiobookshelf {
		return nil, "", false, nil
	}
	works, err := i.meta.GetAuthorWorks(ctx, author.ForeignID)
	if err != nil {
		return nil, "", false, err
	}
	var match *models.Book
	key := normalizeTitle(item.Title)
	for idx := range works {
		if normalizeTitle(works[idx].Title) != key {
			continue
		}
		if match != nil {
			return nil, "", true, nil
		}
		copy := works[idx]
		match = &copy
	}
	if match == nil {
		return nil, "", false, nil
	}
	full, err := i.meta.GetBook(ctx, match.ForeignID)
	if err != nil {
		return nil, "", false, err
	}
	if full != nil {
		return full, "title", false, nil
	}
	return match, "title", false, nil
}

func (i *Importer) applyConflictField(
	ctx context.Context,
	cfg ImportConfig,
	item NormalizedLibraryItem,
	entityType string,
	localID int64,
	fieldName, absValue, upstreamValue string,
	apply func(string) error,
	currentValue func() string,
) (metadataMergeResult, bool, error) {
	result := metadataMergeResult{}
	if apply == nil || currentValue == nil {
		return result, false, nil
	}
	normABS := normalizeConflictValue(fieldName, absValue)
	normUpstream := normalizeConflictValue(fieldName, upstreamValue)
	existing := (*models.ABSMetadataConflict)(nil)
	if i.conflicts != nil {
		conflict, err := i.conflicts.GetByEntityField(ctx, entityType, localID, fieldName)
		if err != nil {
			return result, false, err
		}
		existing = conflict
	}

	chosenSource := ""
	chosenValue := ""
	preferredSource := ""
	resolutionStatus := conflictStatusResolved
	shouldPersist := existing != nil
	if existing != nil && existing.PreferredSource != "" {
		preferredSource = existing.PreferredSource
	}

	switch {
	case normABS == "" && normUpstream == "":
		return result, false, nil
	case normABS == "":
		chosenSource = MetadataSourceUpstream
		chosenValue = upstreamValue
		result.AutoResolved++
	case normUpstream == "":
		chosenSource = MetadataSourceABS
		chosenValue = absValue
		result.AutoResolved++
	case normABS == normUpstream:
		chosenSource = MetadataSourceUpstream
		chosenValue = upstreamValue
		if strings.TrimSpace(chosenValue) == "" {
			chosenSource = MetadataSourceABS
			chosenValue = absValue
		}
		result.AutoResolved++
	default:
		shouldPersist = true
		chosenSource = MetadataSourceUpstream
		chosenValue = upstreamValue
		resolutionStatus = conflictStatusUnresolved
		if existing != nil && existing.PreferredSource != "" {
			preferredSource = existing.PreferredSource
			chosenSource = preferredSource
			if preferredSource == MetadataSourceABS {
				chosenValue = absValue
			} else {
				chosenValue = upstreamValue
			}
			resolutionStatus = conflictStatusResolved
			result.AutoResolved++
		} else {
			result.Conflicts++
		}
	}

	changed := normalizeConflictValue(fieldName, currentValue()) != normalizeConflictValue(fieldName, chosenValue)
	if changed {
		if err := apply(chosenValue); err != nil {
			return metadataMergeResult{}, false, err
		}
	}
	if !shouldPersist || i.conflicts == nil {
		return result, changed, nil
	}
	conflict := &models.ABSMetadataConflict{
		SourceID:         cfg.SourceID,
		LibraryID:        item.LibraryID,
		ItemID:           item.ItemID,
		EntityType:       entityType,
		LocalID:          localID,
		FieldName:        fieldName,
		ABSValue:         absValue,
		UpstreamValue:    upstreamValue,
		AppliedSource:    chosenSource,
		PreferredSource:  preferredSource,
		ResolutionStatus: resolutionStatus,
	}
	if err := i.conflicts.Upsert(ctx, conflict); err != nil {
		return metadataMergeResult{}, false, err
	}
	return result, changed, nil
}

func bookABSCandidateValue(book *models.Book, item NormalizedLibraryItem, field string) string {
	switch field {
	case "description":
		if desc := textutil.CleanDescription(item.Description); desc != "" {
			return desc
		}
	case "release_date":
		if date := formatConflictDate(parseABSDate(item.PublishedDate, item.PublishedYear)); date != "" {
			return date
		}
	case "language":
		if lang := normalizeLanguage(item.Language); lang != "" {
			return lang
		}
	}
	return SerializeBookConflictValue(book, field)
}

func (i *Importer) upsertBook(ctx context.Context, cfg ImportConfig, runID int64, author *models.Author, item NormalizedLibraryItem, allowCreate bool) (*bookUpsertResult, bool, bool, metadataMergeResult, error) {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return nil, false, false, metadataMergeResult{}, errors.New("item title is empty")
	}
	externalID := item.ItemID
	if strings.TrimSpace(item.ResolvedBookForeignID) != "" || strings.TrimSpace(item.ResolvedBookTitle) != "" {
		return i.upsertManualBook(ctx, cfg, runID, author, item)
	}
	if i.provenance != nil {
		if link, err := i.provenance.GetByExternal(ctx, cfg.SourceID, item.LibraryID, entityTypeBook, externalID); err != nil {
			return nil, false, false, metadataMergeResult{}, err
		} else if link != nil {
			existing, err := i.books.GetByID(ctx, link.LocalID)
			if err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
			if existing != nil {
				if !cfg.DryRun {
					if err := i.applyBookFields(ctx, existing, author.ID, item); err != nil {
						return nil, false, false, metadataMergeResult{}, err
					}
					if err := i.upsertBookProvenance(ctx, cfg, runID, existing.ID, item); err != nil {
						return nil, false, false, metadataMergeResult{}, err
					}
				}
				_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, externalID, existing.ID, itemOutcomeUpdated, nil)
				if cfg.DryRun {
					return &bookUpsertResult{row: existing, matchedBy: "provenance"}, false, false, metadataMergeResult{}, nil
				}
				metaResult, err := i.enrichBook(ctx, cfg, item, author, existing)
				return &bookUpsertResult{row: existing, matchedBy: "provenance"}, false, false, metaResult, err
			}
		}
	}

	fid := absForeignID("book", item.LibraryID, externalID)
	if existing, err := i.books.GetByForeignID(ctx, fid); err != nil {
		return nil, false, false, metadataMergeResult{}, err
	} else if existing != nil {
		if !cfg.DryRun {
			if err := i.applyBookFields(ctx, existing, author.ID, item); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
			if err := i.upsertBookProvenance(ctx, cfg, runID, existing.ID, item); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, externalID, existing.ID, itemOutcomeUpdated, nil)
		if cfg.DryRun {
			return &bookUpsertResult{row: existing, matchedBy: "foreign_id"}, false, false, metadataMergeResult{}, nil
		}
		metaResult, err := i.enrichBook(ctx, cfg, item, author, existing)
		return &bookUpsertResult{row: existing, matchedBy: "foreign_id"}, false, false, metaResult, err
	}

	match, ambiguous, err := i.findBookByNormalizedTitle(ctx, author.ID, item.Title)
	if err != nil {
		return nil, false, false, metadataMergeResult{}, err
	}
	if ambiguous {
		if !allowCreate {
			return nil, false, false, metadataMergeResult{}, reviewRequiredError{Reason: reviewReasonAmbiguousBook}
		}
	} else if match != nil {
		if !cfg.DryRun {
			if err := i.applyBookFields(ctx, match, author.ID, item); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
			if err := i.upsertBookProvenance(ctx, cfg, runID, match.ID, item); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, externalID, match.ID, itemOutcomeLinked, nil)
		if cfg.DryRun {
			return &bookUpsertResult{row: match, matchedBy: "author+normalized_title"}, false, true, metadataMergeResult{}, nil
		}
		metaResult, err := i.enrichBook(ctx, cfg, item, author, match)
		return &bookUpsertResult{row: match, matchedBy: "author+normalized_title"}, false, true, metaResult, err
	}
	if !allowCreate {
		return nil, false, false, metadataMergeResult{}, reviewRequiredError{Reason: reviewReasonUnmatchedBook}
	}

	if cfg.DryRun {
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, externalID, 0, itemOutcomeCreated, nil)
		return &bookUpsertResult{row: &models.Book{
			ForeignID:        fid,
			AuthorID:         author.ID,
			Title:            title,
			SortTitle:        title,
			Description:      textutil.CleanDescription(item.Description),
			ReleaseDate:      parseABSDate(item.PublishedDate, item.PublishedYear),
			Genres:           cleanStrings(item.Genres),
			Monitored:        true,
			Status:           models.BookStatusWanted,
			AnyEditionOK:     true,
			Language:         normalizeLanguage(item.Language),
			MediaType:        deriveMediaType(item),
			Narrator:         joinNarrators(item.Narrators),
			DurationSeconds:  int(math.Round(item.DurationSeconds)),
			ASIN:             strings.TrimSpace(item.ASIN),
			MetadataProvider: providerAudiobookshelf,
		}, matchedBy: "created"}, true, false, metadataMergeResult{}, nil
	}

	book := &models.Book{
		ForeignID:        fid,
		AuthorID:         author.ID,
		Title:            title,
		SortTitle:        title,
		Description:      textutil.CleanDescription(item.Description),
		ReleaseDate:      parseABSDate(item.PublishedDate, item.PublishedYear),
		Genres:           cleanStrings(item.Genres),
		Monitored:        true,
		Status:           models.BookStatusWanted,
		AnyEditionOK:     true,
		Language:         normalizeLanguage(item.Language),
		MediaType:        deriveMediaType(item),
		Narrator:         joinNarrators(item.Narrators),
		DurationSeconds:  int(math.Round(item.DurationSeconds)),
		ASIN:             strings.TrimSpace(item.ASIN),
		MetadataProvider: providerAudiobookshelf,
	}
	if err := i.books.Create(ctx, book); err != nil {
		return nil, false, false, metadataMergeResult{}, err
	}
	if err := i.upsertBookProvenance(ctx, cfg, runID, book.ID, item); err != nil {
		return nil, false, false, metadataMergeResult{}, err
	}
	_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, externalID, book.ID, itemOutcomeCreated, nil)
	metaResult, err := i.enrichBook(ctx, cfg, item, author, book)
	return &bookUpsertResult{row: book, matchedBy: "created"}, true, false, metaResult, err
}

func (i *Importer) upsertManualBook(ctx context.Context, cfg ImportConfig, runID int64, author *models.Author, item NormalizedLibraryItem) (*bookUpsertResult, bool, bool, metadataMergeResult, error) {
	foreignID := strings.TrimSpace(item.ResolvedBookForeignID)
	title := strings.TrimSpace(firstNonEmpty(item.ResolvedBookTitle, item.EditedTitle, item.Title))
	if foreignID == "" || title == "" {
		return nil, false, false, metadataMergeResult{}, errors.New("resolved book requires foreignBookId and title")
	}
	if existing, err := i.books.GetByForeignID(ctx, foreignID); err != nil {
		return nil, false, false, metadataMergeResult{}, err
	} else if existing != nil {
		if !cfg.DryRun {
			existing.AuthorID = author.ID
			i.applyABSFormatFields(existing, item)
			if err := i.books.Update(ctx, existing); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
			if err := i.upsertBookProvenance(ctx, cfg, runID, existing.ID, item); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, item.ItemID, existing.ID, itemOutcomeLinked, map[string]string{"matchedBy": "manual_book"})
		return &bookUpsertResult{row: existing, matchedBy: "manual_book"}, false, true, metadataMergeResult{}, nil
	}

	book := &models.Book{
		ForeignID:        foreignID,
		AuthorID:         author.ID,
		Title:            title,
		SortTitle:        title,
		Description:      textutil.CleanDescription(item.Description),
		ReleaseDate:      parseABSDate(item.PublishedDate, item.PublishedYear),
		Genres:           cleanStrings(item.Genres),
		Monitored:        true,
		Status:           models.BookStatusWanted,
		AnyEditionOK:     true,
		Language:         normalizeLanguage(item.Language),
		MediaType:        deriveMediaType(item),
		Narrator:         joinNarrators(item.Narrators),
		DurationSeconds:  int(math.Round(item.DurationSeconds)),
		ASIN:             strings.TrimSpace(item.ASIN),
		MetadataProvider: "openlibrary",
	}
	if i.meta != nil && !cfg.DryRun {
		if full, err := i.meta.GetBook(ctx, foreignID); err == nil && full != nil {
			book = full
			book.ForeignID = foreignID
			book.AuthorID = author.ID
			if book.Title == "" {
				book.Title = title
			}
			if book.SortTitle == "" {
				book.SortTitle = book.Title
			}
			book.Monitored = true
			book.Status = models.BookStatusWanted
			book.AnyEditionOK = true
			if book.MetadataProvider == "" {
				book.MetadataProvider = "openlibrary"
			}
			i.applyABSFormatFields(book, item)
		} else if err != nil {
			slog.Warn("abs import: manual book metadata fetch failed", "foreignID", foreignID, "error", err)
		}
	}
	if cfg.DryRun {
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, item.ItemID, 0, itemOutcomeCreated, map[string]string{"matchedBy": "manual_book"})
		return &bookUpsertResult{row: book, matchedBy: "manual_book"}, true, false, metadataMergeResult{}, nil
	}
	if err := i.books.Create(ctx, book); err != nil {
		return nil, false, false, metadataMergeResult{}, err
	}
	if err := i.upsertBookProvenance(ctx, cfg, runID, book.ID, item); err != nil {
		return nil, false, false, metadataMergeResult{}, err
	}
	_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, item.ItemID, book.ID, itemOutcomeCreated, map[string]string{"matchedBy": "manual_book"})
	return &bookUpsertResult{row: book, matchedBy: "manual_book"}, true, false, metadataMergeResult{Matched: 1}, nil
}

func (i *Importer) applyABSFormatFields(book *models.Book, item NormalizedLibraryItem) {
	if mediaType := deriveMediaType(item); mediaType != "" {
		book.MediaType = mergeMediaType(book.MediaType, mediaType)
	}
	if narrator := joinNarrators(item.Narrators); narrator != "" {
		book.Narrator = narrator
	}
	if item.DurationSeconds > 0 {
		book.DurationSeconds = int(math.Round(item.DurationSeconds))
	}
	if asin := strings.TrimSpace(item.ASIN); asin != "" {
		book.ASIN = asin
	}
	if lang := normalizeLanguage(item.Language); lang != "" && book.Language == "" {
		book.Language = lang
	}
}

func (i *Importer) upsertBookProvenance(ctx context.Context, cfg ImportConfig, runID, bookID int64, item NormalizedLibraryItem) error {
	return i.upsertProvenance(ctx, &models.ABSProvenance{
		SourceID:    cfg.SourceID,
		LibraryID:   item.LibraryID,
		EntityType:  entityTypeBook,
		ExternalID:  item.ItemID,
		LocalID:     bookID,
		ItemID:      item.ItemID,
		FileIDs:     itemFileIDs(item),
		ImportRunID: ptrInt64(runID),
	})
}

func (i *Importer) findBookByNormalizedTitle(ctx context.Context, authorID int64, title string) (*models.Book, bool, error) {
	books, err := i.books.ListByAuthorIncludingExcluded(ctx, authorID)
	if err != nil {
		return nil, false, err
	}
	key := normalizeTitle(title)
	var match *models.Book
	for idx := range books {
		if normalizeTitle(books[idx].Title) != key {
			continue
		}
		if match != nil {
			return nil, true, nil
		}
		copy := books[idx]
		match = &copy
	}
	return match, false, nil
}

func (i *Importer) applyBookFields(ctx context.Context, book *models.Book, authorID int64, item NormalizedLibraryItem) error {
	book.AuthorID = authorID
	book.Title = strings.TrimSpace(item.Title)
	book.SortTitle = book.Title
	if desc := textutil.CleanDescription(item.Description); desc != "" {
		book.Description = desc
	}
	if rd := parseABSDate(item.PublishedDate, item.PublishedYear); rd != nil {
		book.ReleaseDate = rd
	}
	if genres := cleanStrings(item.Genres); len(genres) > 0 {
		book.Genres = genres
	}
	if lang := normalizeLanguage(item.Language); lang != "" {
		book.Language = lang
	}
	if narrator := joinNarrators(item.Narrators); narrator != "" {
		book.Narrator = narrator
	}
	if item.DurationSeconds > 0 {
		book.DurationSeconds = int(math.Round(item.DurationSeconds))
	}
	if asin := strings.TrimSpace(item.ASIN); asin != "" {
		book.ASIN = asin
	}
	book.MediaType = mergeMediaType(book.MediaType, deriveMediaType(item))
	book.Monitored = true
	if book.Status == "" {
		book.Status = models.BookStatusWanted
	}
	if book.MetadataProvider == "" {
		book.MetadataProvider = providerAudiobookshelf
	}
	return i.books.Update(ctx, book)
}

func (i *Importer) upsertSeries(ctx context.Context, cfg ImportConfig, runID, bookID int64, ref NormalizedSeries) (bool, string, error) {
	title := strings.TrimSpace(ref.Name)
	if title == "" {
		return false, "", nil
	}
	externalID := seriesExternalID(ref)
	var existing *models.Series
	if i.provenance != nil {
		if link, err := i.provenance.GetByExternal(ctx, cfg.SourceID, cfg.LibraryID, entityTypeSeries, externalID); err != nil {
			return false, "", err
		} else if link != nil {
			existing, err = i.series.GetByID(ctx, link.LocalID)
			if err != nil {
				return false, "", err
			}
		}
	}
	matchedBy := ""
	if existing == nil {
		match, ambiguous, err := i.findSeriesByTitle(ctx, title)
		if err != nil {
			return false, "", err
		}
		if ambiguous {
			return false, "", fmt.Errorf("ambiguous existing series match for %q", title)
		}
		if match != nil {
			existing = match
			matchedBy = "normalized_title"
		}
	}
	created := false
	if existing == nil {
		existing = &models.Series{
			ForeignID:   absForeignID("series", cfg.LibraryID, externalID),
			Title:       title,
			Description: "",
		}
		if !cfg.DryRun {
			if err := i.series.CreateOrGet(ctx, existing); err != nil {
				return false, "", err
			}
		}
		created = true
		matchedBy = "created"
	}
	metadataJSON := mustJSON(map[string]any{
		"bookId":   bookID,
		"sequence": strings.TrimSpace(ref.Sequence),
	})
	if !cfg.DryRun {
		if err := i.series.LinkBook(ctx, existing.ID, bookID, strings.TrimSpace(ref.Sequence), true); err != nil {
			return false, "", err
		}
		if err := i.upsertProvenance(ctx, &models.ABSProvenance{
			SourceID:    cfg.SourceID,
			LibraryID:   cfg.LibraryID,
			EntityType:  entityTypeSeries,
			ExternalID:  externalID,
			LocalID:     existing.ID,
			ItemID:      "",
			ImportRunID: ptrInt64(runID),
		}); err != nil {
			return false, "", err
		}
	}
	localID := existing.ID
	if cfg.DryRun && created {
		localID = 0
	}
	outcome := itemOutcomeLinked
	if created {
		outcome = itemOutcomeCreated
	}
	_ = i.recordRunEntity(ctx, runID, cfg, cfg.LibraryID, "", entityTypeSeries, externalID, localID, outcome, json.RawMessage(metadataJSON))
	return created, matchedBy, nil
}

func (i *Importer) findSeriesByTitle(ctx context.Context, title string) (*models.Series, bool, error) {
	all, err := i.series.List(ctx)
	if err != nil {
		return nil, false, err
	}
	key := normalizeTitle(title)
	var match *models.Series
	for idx := range all {
		if normalizeTitle(all[idx].Title) != key {
			continue
		}
		if match != nil {
			return nil, true, nil
		}
		copy := all[idx]
		match = &copy
	}
	return match, false, nil
}

func (i *Importer) upsertEditions(ctx context.Context, cfg ImportConfig, runID, bookID int64, item NormalizedLibraryItem) (int, error) {
	added := 0
	for _, format := range deriveEditionFormats(item) {
		externalID := fmt.Sprintf("%s:%s", item.ItemID, format)
		prior, err := i.editions.GetByForeignID(ctx, absForeignID("edition", item.LibraryID, externalID))
		if err != nil {
			return added, err
		}
		edition := &models.Edition{
			ForeignID:   absForeignID("edition", item.LibraryID, externalID),
			BookID:      bookID,
			Title:       item.Title,
			ISBN13:      isbn13Ptr(item.ISBN),
			ISBN10:      isbn10Ptr(item.ISBN),
			ASIN:        ptrString(strings.TrimSpace(item.ASIN)),
			Publisher:   strings.TrimSpace(item.Publisher),
			PublishDate: parseABSDate(item.PublishedDate, item.PublishedYear),
			Format:      strings.ToUpper(format),
			Language:    normalizeLanguage(item.Language),
			IsEbook:     format == models.MediaTypeEbook,
			EditionInfo: "Imported from Audiobookshelf",
			Monitored:   true,
		}
		if !cfg.DryRun {
			if err := i.editions.Upsert(ctx, edition); err != nil {
				return added, err
			}
		}
		if prior == nil {
			added++
		}
		if !cfg.DryRun {
			if err := i.upsertProvenance(ctx, &models.ABSProvenance{
				SourceID:    cfg.SourceID,
				LibraryID:   item.LibraryID,
				EntityType:  entityTypeEdition,
				ExternalID:  externalID,
				LocalID:     edition.ID,
				ItemID:      item.ItemID,
				Format:      format,
				FileIDs:     itemFileIDs(item),
				ImportRunID: ptrInt64(runID),
			}); err != nil {
				return added, err
			}
		}
		outcome := itemOutcomeUpdated
		if prior == nil {
			outcome = itemOutcomeCreated
		}
		localID := edition.ID
		if cfg.DryRun && prior == nil {
			localID = 0
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeEdition, externalID, localID, outcome, map[string]any{"format": format, "bookId": bookID})
	}
	return added, nil
}

func (i *Importer) upsertProvenance(ctx context.Context, p *models.ABSProvenance) error {
	if i.provenance == nil {
		return nil
	}
	return i.provenance.Upsert(ctx, p)
}

type RollbackStats struct {
	ActionsPlanned     int `json:"actionsPlanned"`
	EntitiesDeleted    int `json:"entitiesDeleted"`
	ProvenanceUnlinked int `json:"provenanceUnlinked"`
	Skipped            int `json:"skipped"`
	Failed             int `json:"failed"`
}

type RollbackAction struct {
	EntityType string `json:"entityType"`
	ExternalID string `json:"externalId"`
	LocalID    int64  `json:"localId"`
	Outcome    string `json:"outcome"`
	Action     string `json:"action"`
	Reason     string `json:"reason,omitempty"`
}

type RollbackResult struct {
	RunID    int64            `json:"runId"`
	Preview  bool             `json:"preview"`
	DryRun   bool             `json:"dryRun"`
	Status   string           `json:"status"`
	Stats    RollbackStats    `json:"stats"`
	Actions  []RollbackAction `json:"actions"`
	Finished time.Time        `json:"finishedAt"`
}

func (i *Importer) RecentRuns(ctx context.Context, limit int) ([]models.ABSImportRun, error) {
	if i.runs == nil {
		return nil, nil
	}
	return i.runs.ListRecent(ctx, limit)
}

func HydrateRun(run models.ABSImportRun) PersistedImportRun {
	out := PersistedImportRun{
		ID:          run.ID,
		SourceID:    run.SourceID,
		SourceLabel: run.SourceLabel,
		BaseURL:     run.BaseURL,
		LibraryID:   run.LibraryID,
		Status:      run.Status,
		DryRun:      run.DryRun,
		StartedAt:   run.StartedAt,
		FinishedAt:  run.FinishedAt,
		Source: ImportSourceSnapshot{
			SourceID:  run.SourceID,
			Label:     run.SourceLabel,
			BaseURL:   run.BaseURL,
			LibraryID: run.LibraryID,
			DryRun:    run.DryRun,
		},
		Summary: ImportSummary{DryRun: run.DryRun},
	}
	if strings.TrimSpace(run.SourceConfigJSON) != "" {
		_ = json.Unmarshal([]byte(run.SourceConfigJSON), &out.Source)
	}
	if strings.TrimSpace(run.CheckpointJSON) != "" && strings.TrimSpace(run.CheckpointJSON) != "{}" {
		var checkpoint ImportCheckpoint
		if err := json.Unmarshal([]byte(run.CheckpointJSON), &checkpoint); err == nil {
			out.Checkpoint = &checkpoint
		}
	}
	if strings.TrimSpace(run.SummaryJSON) != "" {
		_ = json.Unmarshal([]byte(run.SummaryJSON), &out.Summary)
	}
	return out
}

func (i *Importer) GetRun(ctx context.Context, runID int64) (*models.ABSImportRun, error) {
	if i.runs == nil {
		return nil, nil
	}
	return i.runs.GetByID(ctx, runID)
}

func (i *Importer) RollbackPreview(ctx context.Context, runID int64) (*RollbackResult, error) {
	return i.rollback(ctx, runID, true)
}

func (i *Importer) Rollback(ctx context.Context, runID int64) (*RollbackResult, error) {
	return i.rollback(ctx, runID, false)
}

func (i *Importer) rollback(ctx context.Context, runID int64, preview bool) (*RollbackResult, error) {
	if i.runs == nil || i.runEntities == nil {
		return nil, errors.New("abs rollback is unavailable")
	}
	run, err := i.runs.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("abs import run %d not found", runID)
	}
	result := &RollbackResult{
		RunID:    runID,
		Preview:  preview,
		DryRun:   run.DryRun,
		Status:   run.Status,
		Finished: time.Now().UTC(),
	}
	if run.DryRun {
		return result, nil
	}
	entities, err := i.runEntities.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entities, func(a, b int) bool {
		return rollbackEntityRank(entities[a]) < rollbackEntityRank(entities[b])
	})
	type set map[int64]struct{}
	deletedBooks := make(set)
	for _, entity := range entities {
		current, err := i.provenance.GetByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID)
		if err != nil {
			result.Stats.Failed++
			result.Actions = append(result.Actions, RollbackAction{
				EntityType: entity.EntityType,
				ExternalID: entity.ExternalID,
				LocalID:    entity.LocalID,
				Outcome:    entity.Outcome,
				Action:     "inspect",
				Reason:     err.Error(),
			})
			continue
		}
		if current == nil || current.ImportRunID == nil || *current.ImportRunID != runID {
			result.Stats.Skipped++
			result.Actions = append(result.Actions, RollbackAction{
				EntityType: entity.EntityType,
				ExternalID: entity.ExternalID,
				LocalID:    entity.LocalID,
				Outcome:    entity.Outcome,
				Action:     "skip",
				Reason:     "run is no longer the current provenance owner for this entity",
			})
			continue
		}

		action := RollbackAction{
			EntityType: entity.EntityType,
			ExternalID: entity.ExternalID,
			LocalID:    entity.LocalID,
			Outcome:    entity.Outcome,
		}
		metadata := parseJSONObject(entity.MetadataJSON)
		switch {
		case entity.EntityType == entityTypeBook && entity.Outcome == itemOutcomeCreated:
			action.Action = "delete_book"
			result.Stats.ActionsPlanned++
			if !preview {
				if err := i.provenance.DeleteByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				if err := i.books.Delete(ctx, entity.LocalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				deletedBooks[entity.LocalID] = struct{}{}
				result.Stats.EntitiesDeleted++
				result.Stats.ProvenanceUnlinked++
			}
		case entity.EntityType == entityTypeEdition && entity.Outcome == itemOutcomeCreated:
			if bookID, _ := metadata["bookId"].(float64); int64(bookID) != 0 {
				if _, ok := deletedBooks[int64(bookID)]; ok {
					action.Action = "skip"
					action.Reason = "parent book rollback removes this edition implicitly"
					result.Stats.Skipped++
					result.Actions = append(result.Actions, action)
					continue
				}
			}
			action.Action = "delete_edition"
			result.Stats.ActionsPlanned++
			if !preview {
				if err := i.provenance.DeleteByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				if err := i.editions.Delete(ctx, entity.LocalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				result.Stats.EntitiesDeleted++
				result.Stats.ProvenanceUnlinked++
			}
		case entity.EntityType == entityTypeSeries:
			bookID, _ := metadata["bookId"].(float64)
			if bookID > 0 {
				action.Action = "unlink_series"
				result.Stats.ActionsPlanned++
				if !preview {
					if err := i.series.UnlinkBook(ctx, entity.LocalID, int64(bookID)); err != nil {
						action.Action = "skip"
						action.Reason = err.Error()
						result.Stats.Failed++
						result.Actions = append(result.Actions, action)
						continue
					}
				}
				if entity.Outcome == itemOutcomeCreated {
					books, err := i.series.ListBooksInSeries(ctx, entity.LocalID)
					if err == nil {
						remaining := len(books)
						if bookID > 0 && remaining > 0 {
							remaining--
						}
						if remaining <= 0 {
							if !preview {
								_ = i.series.Delete(ctx, entity.LocalID)
								result.Stats.EntitiesDeleted++
							}
							action.Action = "delete_series"
						}
					}
				}
				if !preview {
					if err := i.provenance.DeleteByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID); err == nil {
						result.Stats.ProvenanceUnlinked++
					}
				}
			} else {
				action.Action = "unlink_provenance"
				result.Stats.ActionsPlanned++
				if !preview {
					if err := i.provenance.DeleteByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID); err != nil {
						action.Action = "skip"
						action.Reason = err.Error()
						result.Stats.Failed++
						result.Actions = append(result.Actions, action)
						continue
					}
					result.Stats.ProvenanceUnlinked++
				}
			}
		case entity.EntityType == entityTypeAuthor && entity.Outcome == itemOutcomeCreated:
			books, err := i.books.ListByAuthorIncludingExcluded(ctx, entity.LocalID)
			if err != nil {
				action.Action = "skip"
				action.Reason = err.Error()
				result.Stats.Failed++
				result.Actions = append(result.Actions, action)
				continue
			}
			remaining := 0
			for _, book := range books {
				if _, ok := deletedBooks[book.ID]; ok {
					continue
				}
				remaining++
			}
			if remaining > 0 {
				action.Action = "skip"
				action.Reason = "author still has linked books"
				result.Stats.Skipped++
				result.Actions = append(result.Actions, action)
				continue
			}
			action.Action = "delete_author"
			result.Stats.ActionsPlanned++
			if !preview {
				if err := i.provenance.DeleteByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				if err := i.authors.Delete(ctx, entity.LocalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				result.Stats.EntitiesDeleted++
				result.Stats.ProvenanceUnlinked++
			}
		default:
			action.Action = "unlink_provenance"
			result.Stats.ActionsPlanned++
			if !preview {
				if err := i.provenance.DeleteByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				result.Stats.ProvenanceUnlinked++
			}
		}
		result.Actions = append(result.Actions, action)
	}
	if !preview && result.Stats.Failed == 0 {
		if err := i.runs.UpdateStatus(ctx, runID, runStatusRolledBack); err != nil {
			return nil, err
		}
		result.Status = runStatusRolledBack
	}
	return result, nil
}

func (i *Importer) recordRunEntity(ctx context.Context, runID int64, cfg ImportConfig, libraryID, itemID, entityType, externalID string, localID int64, outcome string, metadata any) error {
	if runID == 0 || i.runEntities == nil {
		return nil
	}
	return i.runEntities.Record(ctx, &models.ABSImportRunEntity{
		RunID:        runID,
		SourceID:     cfg.SourceID,
		LibraryID:    libraryID,
		ItemID:       itemID,
		EntityType:   entityType,
		ExternalID:   externalID,
		LocalID:      localID,
		Outcome:      outcome,
		MetadataJSON: mustJSON(metadata),
	})
}

func (i *Importer) fail(err error) {
	slog.Error("abs import failed", "error", err)
	i.setProgress(func(p *ImportProgress) {
		p.Error = err.Error()
		p.Message = "failed"
	})
}

func (i *Importer) setProgress(mutate func(*ImportProgress)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	mutate(&i.progress)
}

func authorExternalID(author NormalizedAuthor) string {
	if strings.TrimSpace(author.ID) != "" {
		return strings.TrimSpace(author.ID)
	}
	return "name:" + normalizeAuthorName(author.Name)
}

func seriesExternalID(series NormalizedSeries) string {
	if strings.TrimSpace(series.ID) != "" {
		return strings.TrimSpace(series.ID)
	}
	return "name:" + normalizeTitle(series.Name)
}

func absForeignID(kind, libraryID, externalID string) string {
	return fmt.Sprintf("abs:%s:%s:%s", kind, strings.TrimSpace(libraryID), strings.TrimSpace(externalID))
}

func parseABSDate(dateStr, yearStr string) *time.Time {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr != "" {
		layouts := []string{time.RFC3339, "2006-01-02", "2006-1-2"}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, dateStr); err == nil {
				utc := parsed.UTC()
				return &utc
			}
		}
	}
	yearStr = strings.TrimSpace(yearStr)
	if yearStr != "" {
		if year, err := strconv.Atoi(yearStr); err == nil && year > 0 {
			t := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
			return &t
		}
	}
	return nil
}

func deriveMediaType(item NormalizedLibraryItem) string {
	hasAudio := len(item.AudioFiles) > 0
	hasEbook := strings.TrimSpace(item.EbookPath) != ""
	switch {
	case hasAudio && hasEbook:
		return models.MediaTypeBoth
	case hasEbook:
		return models.MediaTypeEbook
	case hasAudio:
		return models.MediaTypeAudiobook
	default:
		return models.MediaTypeAudiobook
	}
}

func mergeMediaType(current, next string) string {
	switch {
	case current == models.MediaTypeBoth || next == models.MediaTypeBoth:
		return models.MediaTypeBoth
	case current == "":
		return next
	case current == next:
		return current
	case (current == models.MediaTypeAudiobook && next == models.MediaTypeEbook) || (current == models.MediaTypeEbook && next == models.MediaTypeAudiobook):
		return models.MediaTypeBoth
	default:
		return current
	}
}

func deriveEditionFormats(item NormalizedLibraryItem) []string {
	formats := make([]string, 0, 2)
	if strings.TrimSpace(item.EbookPath) != "" {
		formats = append(formats, models.MediaTypeEbook)
	}
	if len(item.AudioFiles) > 0 {
		formats = append(formats, models.MediaTypeAudiobook)
	}
	if len(formats) == 0 {
		formats = append(formats, models.MediaTypeAudiobook)
	}
	return formats
}

func joinNarrators(narrators []string) string {
	return strings.Join(cleanStrings(narrators), ", ")
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func authorSearchQueries(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	queries := []string{name}
	if compact := compactInitialsAuthorQuery(name); compact != "" {
		queries = append(queries, compact)
	}
	if norm := normalizeAuthorName(name); norm != "" {
		queries = append(queries, norm)
		if surname := initialedSurnameFallback(norm); surname != "" {
			queries = append(queries, surname)
		}
	}
	return dedupeStrings(queries)
}

func compactInitialsAuthorQuery(name string) string {
	fields := strings.Fields(name)
	if len(fields) < 3 {
		return ""
	}
	initials := make([]string, 0, len(fields)-1)
	idx := 0
	for idx < len(fields)-1 {
		initial, ok := authorInitial(fields[idx])
		if !ok {
			break
		}
		initials = append(initials, strings.ToUpper(initial)+".")
		idx++
	}
	if len(initials) < 2 || idx >= len(fields) {
		return ""
	}
	return strings.Join(initials, "") + " " + strings.Join(fields[idx:], " ")
}

func authorInitial(token string) (string, bool) {
	var letters []rune
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letters = append(letters, unicode.ToLower(r))
		}
	}
	if len(letters) != 1 {
		return "", false
	}
	return string(letters[0]), true
}

func initialedSurnameFallback(normalized string) string {
	fields := strings.Fields(normalized)
	if len(fields) < 2 {
		return ""
	}
	for _, field := range fields[:len(fields)-1] {
		if len([]rune(field)) != 1 {
			return ""
		}
	}
	return fields[len(fields)-1]
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeLanguage(language string) string {
	return strings.ToLower(strings.TrimSpace(language))
}

func normalizeAuthorName(name string) string {
	return textutil.NormalizeAuthorName(name)
}

func normalizeTitle(title string) string {
	return indexer.NormalizeTitleForDedup(strings.TrimSpace(title))
}

func primaryAuthorName(item NormalizedLibraryItem) string {
	if len(item.Authors) == 0 {
		return ""
	}
	return strings.TrimSpace(item.Authors[0].Name)
}

func allowImmediateImport(item NormalizedLibraryItem) bool {
	return strings.TrimSpace(item.ASIN) != "" && (len(item.AudioFiles) > 0 || deriveMediaType(item) == models.MediaTypeBoth || deriveMediaType(item) == models.MediaTypeAudiobook)
}

func itemFileIDs(item NormalizedLibraryItem) []string {
	ids := make([]string, 0, len(item.AudioFiles)+1)
	for _, file := range item.AudioFiles {
		switch {
		case strings.TrimSpace(file.INO) != "":
			ids = append(ids, "audio:"+strings.TrimSpace(file.INO))
		case strings.TrimSpace(file.Path) != "":
			ids = append(ids, "audio:"+strings.TrimSpace(file.Path))
		}
	}
	if strings.TrimSpace(item.EbookINO) != "" {
		ids = append(ids, "ebook:"+strings.TrimSpace(item.EbookINO))
	} else if strings.TrimSpace(item.EbookPath) != "" {
		ids = append(ids, "ebook:"+strings.TrimSpace(item.EbookPath))
	}
	return ids
}

func sortNameFromFull(name string) string {
	fields := strings.Fields(name)
	if len(fields) < 2 {
		return name
	}
	last := fields[len(fields)-1]
	rest := strings.Join(fields[:len(fields)-1], " ")
	return last + ", " + rest
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sourceSnapshot(cfg ImportConfig) ImportSourceSnapshot {
	return ImportSourceSnapshot{
		SourceID:  cfg.SourceID,
		Label:     cfg.Label,
		BaseURL:   cfg.BaseURL,
		LibraryID: cfg.LibraryID,
		PathRemap: cfg.PathRemap,
		Enabled:   cfg.Enabled,
		DryRun:    cfg.DryRun,
	}
}

func mustJSON(value any) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) == 0 {
		return "{}"
	}
	return string(data)
}

func parseJSONObject(raw string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func loadImportCheckpoint(ctx context.Context, settings *db.SettingsRepo) (*ImportCheckpoint, error) {
	if settings == nil {
		return nil, nil
	}
	setting, err := settings.Get(ctx, SettingABSImportCheckpoint)
	if err != nil || setting == nil || strings.TrimSpace(setting.Value) == "" {
		return nil, err
	}
	var checkpoint ImportCheckpoint
	if err := json.Unmarshal([]byte(setting.Value), &checkpoint); err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

func importStartMessage(dryRun bool) string {
	if dryRun {
		return "starting ABS dry-run…"
	}
	return "starting ABS import…"
}

func importItemMessage(dryRun bool, label string) string {
	if dryRun {
		return fmt.Sprintf("previewing %s", label)
	}
	return fmt.Sprintf("importing %s", label)
}

func importDoneMessage(dryRun bool) string {
	if dryRun {
		return "dry-run complete"
	}
	return "done"
}

func rollbackEntityRank(entity models.ABSImportRunEntity) int {
	switch entity.EntityType {
	case entityTypeBook:
		return 0
	case entityTypeEdition:
		return 1
	case entityTypeSeries:
		return 2
	case entityTypeAuthor:
		return 3
	default:
		return 4
	}
}

func pathUnderDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && !strings.HasPrefix(rel, "..")
}

func dedupeCleanPaths(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		clean := filepath.Clean(strings.TrimSpace(value))
		if clean == "." || clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func ptrString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func ptrInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func isbn13Ptr(raw string) *string {
	digits := isbnDigits(raw)
	if len(digits) == 13 {
		return &digits
	}
	return nil
}

func isbn10Ptr(raw string) *string {
	digits := isbnDigits(raw)
	if len(digits) == 10 {
		return &digits
	}
	return nil
}

func isbnDigits(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s ImportStats) String() string {
	data, _ := json.Marshal(s)
	return string(data)
}
