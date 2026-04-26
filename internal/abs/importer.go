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

	fuzzy "github.com/creditx/go-fuzzywuzzy"
	"github.com/vavallee/bindery/internal/db"
	bindimporter "github.com/vavallee/bindery/internal/importer"
	"github.com/vavallee/bindery/internal/indexer"
	"github.com/vavallee/bindery/internal/metadata"
	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/textutil"
)

const (
	DefaultSourceID             = "default"
	importProgressResultsLimit  = 100
	SettingABSLastImportAt      = "abs.last_import_at"
	settingDefaultRootID        = "library.defaultRootFolderId"
	runEntityMetadataKind       = "abs_run_entity_metadata"
	runEntityMetadataVersion    = 1
	entityTypeAuthor            = "author"
	entityTypeBook              = "book"
	entityTypeSeries            = "series"
	entityTypeEdition           = "edition"
	providerAudiobookshelf      = "audiobookshelf"
	providerHardcover           = "hardcover"
	runStatusRunning            = "running"
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
	if _, err := NormalizeAPIKey(c.APIKey); err != nil {
		return err
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

	dryRunSeriesExternalIDs map[string]struct{}
	dryRunSeriesTitles      map[string]struct{}
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

type runEntityMetadataEnvelope struct {
	Kind     string                     `json:"kind"`
	Version  int                        `json:"version"`
	Data     map[string]any             `json:"data,omitempty"`
	Snapshot *runEntitySnapshotEnvelope `json:"snapshot,omitempty"`
}

// runEntitySnapshotEnvelope carries a before/after snapshot of a single entity
// so rollback can restore field-level state. Before/After are encoded as raw
// JSON with the shape indicated by EntityType; decoders should match on
// EntityType before unmarshaling into a concrete snapshot struct.
type runEntitySnapshotEnvelope struct {
	EntityType string          `json:"entityType"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
}

type bookRollbackSnapshot struct {
	ForeignID             string     `json:"foreignId"`
	AuthorID              int64      `json:"authorId"`
	Title                 string     `json:"title"`
	SortTitle             string     `json:"sortTitle"`
	OriginalTitle         string     `json:"originalTitle"`
	Description           string     `json:"description"`
	ImageURL              string     `json:"imageUrl"`
	ReleaseDate           *time.Time `json:"releaseDate,omitempty"`
	Genres                []string   `json:"genres"`
	AverageRating         float64    `json:"averageRating"`
	RatingsCount          int        `json:"ratingsCount"`
	Monitored             bool       `json:"monitored"`
	Status                string     `json:"status"`
	AnyEditionOK          bool       `json:"anyEditionOk"`
	SelectedEditionID     *int64     `json:"selectedEditionId,omitempty"`
	Language              string     `json:"language"`
	MediaType             string     `json:"mediaType"`
	Narrator              string     `json:"narrator"`
	DurationSeconds       int        `json:"durationSeconds"`
	ASIN                  string     `json:"asin"`
	CalibreID             *int64     `json:"calibreId,omitempty"`
	MetadataProvider      string     `json:"metadataProvider"`
	LastMetadataRefreshAt *time.Time `json:"lastMetadataRefreshAt,omitempty"`
}

// authorRollbackSnapshot captures the subset of author fields the importer
// mutates so rollback can restore prior state without trampling post-import
// user edits. Fields omitted here (stats, profile FKs, Monitored) are not
// touched by the ABS import path.
type authorRollbackSnapshot struct {
	ForeignID             string     `json:"foreignId"`
	Name                  string     `json:"name"`
	SortName              string     `json:"sortName"`
	Description           string     `json:"description"`
	ImageURL              string     `json:"imageUrl"`
	Disambiguation        string     `json:"disambiguation"`
	MetadataProvider      string     `json:"metadataProvider"`
	LastMetadataRefreshAt *time.Time `json:"lastMetadataRefreshAt,omitempty"`
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
	progress := i.progress
	if len(progress.Results) > 0 {
		progress.Results = append([]ImportItemResult(nil), progress.Results...)
	}
	return progress
}

func (i *Importer) Running() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.running
}

func (i *Importer) ResumeInterrupted(ctx context.Context, fallback ImportConfig) (bool, error) {
	if i.runs == nil {
		return false, nil
	}
	run, err := i.runs.LatestRunningWithCheckpoint(ctx)
	if err != nil {
		return false, err
	}
	if run == nil {
		return false, nil
	}
	checkpoint, err := decodeImportCheckpoint(run.CheckpointJSON)
	if err != nil {
		return true, fmt.Errorf("decode interrupted abs import checkpoint: %w", err)
	}
	if checkpoint == nil {
		return false, nil
	}
	if i.settings != nil {
		if err := i.settings.Set(ctx, SettingABSImportCheckpoint, strings.TrimSpace(run.CheckpointJSON)); err != nil {
			return true, fmt.Errorf("restore abs import checkpoint: %w", err)
		}
	}

	cfg := resumeConfigFromRun(*run, fallback)
	if err := i.runs.Finish(ctx, run.ID, runStatusFailed, ImportSummary{
		DryRun:                run.DryRun,
		ResumedFromCheckpoint: true,
		Checkpoint:            checkpoint,
		Error:                 "interrupted by process restart; resumed from checkpoint",
	}); err != nil {
		return true, fmt.Errorf("mark interrupted abs import run %d failed: %w", run.ID, err)
	}
	if err := cfg.Validate(); err != nil {
		return true, fmt.Errorf("resume abs import run %d: %w", run.ID, err)
	}
	if err := i.Start(context.WithoutCancel(ctx), cfg); err != nil {
		return true, err
	}
	return true, nil
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
	matcher, err := i.newAuthorMatcher(ctx)
	if err != nil {
		return ImportItemResult{
			ItemID:  item.ItemID,
			Title:   item.Title,
			Outcome: itemOutcomeFailed,
			Message: err.Error(),
		}, err
	}
	result := i.importOne(ctx, cfg, 0, item, stats, true, matcher)
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

	authorMatcher, err := i.newAuthorMatcher(ctx)
	if err != nil {
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

	sourceConfigJSON, err := encodeJSON(sourceSnapshot(cfg))
	if err != nil {
		i.fail(fmt.Errorf("encode abs import source config: %w", err))
		return stats
	}
	checkpointJSON, err := encodeJSON(summary.Checkpoint)
	if err != nil {
		i.fail(fmt.Errorf("encode abs import checkpoint: %w", err))
		return stats
	}
	run := &models.ABSImportRun{
		SourceID:         cfg.SourceID,
		SourceLabel:      cfg.Label,
		BaseURL:          cfg.BaseURL,
		LibraryID:        cfg.LibraryID,
		Status:           runStatusRunning,
		DryRun:           cfg.DryRun,
		SourceConfigJSON: sourceConfigJSON,
		CheckpointJSON:   checkpointJSON,
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
		result := i.importOne(ctx, cfg, run.ID, item, stats, allowImmediateImport(item), authorMatcher)
		i.setProgress(func(p *ImportProgress) {
			p.Processed++
			p.Results = appendImportProgressResult(p.Results, result)
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
		p.ResumedFromCheckpoint = false
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

func (i *Importer) importOne(ctx context.Context, cfg ImportConfig, runID int64, item NormalizedLibraryItem, stats *ImportStats, allowCreate bool, matcher *authorMatcher) ImportItemResult {
	result := ImportItemResult{
		ItemID:  item.ItemID,
		Title:   item.Title,
		Outcome: itemOutcomeUpdated,
	}
	author, authorCreated, authorMatchedBy, authorMeta, err := i.resolveAuthor(ctx, cfg, runID, item, allowCreate, matcher)
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
		i.recordSecondaryAuthors(ctx, author.ID, item.Authors[1:], matcher)
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
		created, matchedBy, err := i.upsertSeries(ctx, cfg, runID, bookResult.row.ID, series, stats)
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
	seriesMeta, hardcoverSeriesCount := i.matchHardcoverSeries(ctx, cfg, runID, author, bookResult.row, item, stats)
	stats.MetadataMatched += seriesMeta.Matched
	stats.MetadataRelinked += seriesMeta.Relinked
	stats.MetadataConflicts += seriesMeta.Conflicts
	stats.MetadataAutoResolved += seriesMeta.AutoResolved
	seriesCount += hardcoverSeriesCount
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
	messages = append(messages, seriesMeta.Messages...)
	if reconcile.Message != "" {
		messages = append(messages, reconcile.Message)
	}
	if len(messages) > 0 {
		result.Message = strings.Join(messages, "; ")
	}
	if !cfg.DryRun && !created {
		data := map[string]any{}
		if result.MatchedBy != "" {
			data["matchedBy"] = result.MatchedBy
		}
		if err := i.recordBookAfterSnapshot(ctx, runID, cfg, item, bookResult.row.ID, result.Outcome, data); err != nil {
			slog.Warn("abs import: persist book rollback snapshot failed", "bookID", bookResult.row.ID, "runID", runID, "error", err)
		}
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
	payloadJSON, err := encodeJSON(item)
	if err != nil {
		return fmt.Errorf("encode abs review payload: %w", err)
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
		PayloadJSON:   payloadJSON,
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

func (i *Importer) resolveAuthor(ctx context.Context, cfg ImportConfig, runID int64, item NormalizedLibraryItem, allowCreate bool, matcher *authorMatcher) (*models.Author, bool, string, metadataMergeResult, error) {
	if len(item.Authors) == 0 {
		return nil, false, "", metadataMergeResult{}, errors.New("item has no authors")
	}
	primary := item.Authors[0]
	name := strings.TrimSpace(primary.Name)
	if name == "" {
		return nil, false, "", metadataMergeResult{}, errors.New("primary author name is empty")
	}
	if strings.TrimSpace(item.ResolvedAuthorForeignID) != "" || strings.TrimSpace(item.ResolvedAuthorName) != "" {
		return i.resolveManualAuthor(ctx, cfg, runID, item, matcher)
	}
	if matcher == nil {
		loaded, err := i.newAuthorMatcher(ctx)
		if err != nil {
			return nil, false, "", metadataMergeResult{}, err
		}
		matcher = loaded
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
				if cfg.DryRun {
					_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, existing.ID, itemOutcomeLinked, nil)
					return existing, false, "provenance", metadataMergeResult{}, nil
				}
				if err := i.recordAuthorBeforeSnapshot(ctx, runID, cfg, item, externalID, existing, itemOutcomeLinked, nil); err != nil {
					slog.Warn("abs import: persist author rollback snapshot failed", "authorID", existing.ID, "runID", runID, "error", err)
				}
				metaResult, err := i.enrichAuthor(ctx, cfg, item, existing, matcher)
				if perr := i.recordAuthorAfterSnapshot(ctx, runID, cfg, item, externalID, existing.ID, itemOutcomeLinked, nil); perr != nil {
					slog.Warn("abs import: persist author rollback snapshot failed", "authorID", existing.ID, "runID", runID, "error", perr)
				}
				return existing, false, "provenance", metaResult, err
			}
		}
	}

	if existing, matchedBy, ambiguous, err := matcher.findAuthorByName(ctx, name); err != nil {
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
			if shouldRecordAuthorVariantAlias(matchedBy) {
				i.recordAuthorVariantAlias(ctx, existing.ID, name, matcher)
			}
		}
		if cfg.DryRun {
			_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, existing.ID, itemOutcomeLinked, nil)
			return existing, false, matchedBy, metadataMergeResult{}, nil
		}
		if err := i.recordAuthorBeforeSnapshot(ctx, runID, cfg, item, externalID, existing, itemOutcomeLinked, nil); err != nil {
			slog.Warn("abs import: persist author rollback snapshot failed", "authorID", existing.ID, "runID", runID, "error", err)
		}
		metaResult, err := i.enrichAuthor(ctx, cfg, item, existing, matcher)
		if perr := i.recordAuthorAfterSnapshot(ctx, runID, cfg, item, externalID, existing.ID, itemOutcomeLinked, nil); perr != nil {
			slog.Warn("abs import: persist author rollback snapshot failed", "authorID", existing.ID, "runID", runID, "error", perr)
		}
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
	matcher.addAuthor(author)
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
	metaResult, err := i.enrichAuthor(ctx, cfg, item, author, matcher)
	return author, true, "created", metaResult, err
}

func (i *Importer) resolveManualAuthor(ctx context.Context, cfg ImportConfig, runID int64, item NormalizedLibraryItem, matcher *authorMatcher) (*models.Author, bool, string, metadataMergeResult, error) {
	primary := item.Authors[0]
	absName := strings.TrimSpace(primary.Name)
	absExternalID := authorExternalID(primary)
	foreignID := strings.TrimSpace(item.ResolvedAuthorForeignID)
	name := strings.TrimSpace(item.ResolvedAuthorName)
	if foreignID == "" || name == "" {
		return nil, false, "", metadataMergeResult{}, errors.New("resolved author requires foreignAuthorId and authorName")
	}
	if matcher == nil {
		loaded, err := i.newAuthorMatcher(ctx)
		if err != nil {
			return nil, false, "", metadataMergeResult{}, err
		}
		matcher = loaded
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
				i.recordAuthorVariantAlias(ctx, existing.ID, absName, matcher)
			}
		}
		_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, absExternalID, existing.ID, itemOutcomeLinked, map[string]string{"matchedBy": "manual_author"})
		return existing, false, "manual_author", metadataMergeResult{}, nil
	}

	if existing, _, ambiguous, err := matcher.findAuthorByName(ctx, name); err != nil {
		return nil, false, "", metadataMergeResult{}, err
	} else if ambiguous {
		return nil, false, "", metadataMergeResult{}, reviewRequiredError{Reason: reviewReasonAmbiguousAuthor}
	} else if existing != nil {
		manualMeta := map[string]string{"matchedBy": "manual_author_name"}
		if cfg.DryRun {
			_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, absExternalID, existing.ID, itemOutcomeLinked, manualMeta)
			return existing, false, "manual_author", metadataMergeResult{}, nil
		}
		if err := i.recordAuthorBeforeSnapshot(ctx, runID, cfg, item, absExternalID, existing, itemOutcomeLinked, map[string]any{"matchedBy": "manual_author_name"}); err != nil {
			slog.Warn("abs import: persist author rollback snapshot failed", "authorID", existing.ID, "runID", runID, "error", err)
		}
		if existing.ForeignID == "" || strings.HasPrefix(existing.ForeignID, "abs:") {
			existing.ForeignID = foreignID
			if existing.MetadataProvider == "" || existing.MetadataProvider == providerAudiobookshelf {
				existing.MetadataProvider = "openlibrary"
			}
			if err := i.authors.Update(ctx, existing); err != nil {
				return nil, false, "", metadataMergeResult{}, err
			}
			matcher.addAuthor(existing)
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
			i.recordAuthorVariantAlias(ctx, existing.ID, absName, matcher)
		}
		if perr := i.recordAuthorAfterSnapshot(ctx, runID, cfg, item, absExternalID, existing.ID, itemOutcomeLinked, map[string]any{"matchedBy": "manual_author_name"}); perr != nil {
			slog.Warn("abs import: persist author rollback snapshot failed", "authorID", existing.ID, "runID", runID, "error", perr)
		}
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
	matcher.addAuthor(author)
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
		i.recordAuthorVariantAlias(ctx, author.ID, absName, matcher)
	}
	_ = i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, absExternalID, author.ID, itemOutcomeCreated, map[string]string{"matchedBy": "manual_author"})
	return author, true, "manual_author", metadataMergeResult{}, nil
}

type authorMatcher struct {
	authors     *db.AuthorRepo
	all         []*models.Author
	byID        map[int64]*models.Author
	aliases     []models.AuthorAlias
	aliasLoaded map[authorAliasKey]struct{}
}

type authorAliasKey struct {
	authorID int64
	name     string
}

func (i *Importer) newAuthorMatcher(ctx context.Context) (*authorMatcher, error) {
	all, err := i.authors.List(ctx)
	if err != nil {
		return nil, err
	}
	matcher := &authorMatcher{
		authors:     i.authors,
		byID:        make(map[int64]*models.Author, len(all)),
		aliasLoaded: make(map[authorAliasKey]struct{}),
	}
	for idx := range all {
		matcher.addAuthor(&all[idx])
	}
	if i.aliases != nil {
		loaded, err := i.aliases.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, alias := range loaded {
			matcher.addAlias(alias)
		}
	}
	return matcher, nil
}

func (m *authorMatcher) addAuthor(author *models.Author) {
	if m == nil || author == nil || author.ID == 0 {
		return
	}
	cp := *author
	if existing, ok := m.byID[cp.ID]; ok {
		*existing = cp
		return
	}
	m.byID[cp.ID] = &cp
	m.all = append(m.all, &cp)
}

func (m *authorMatcher) addAlias(alias models.AuthorAlias) {
	if m == nil || alias.AuthorID == 0 {
		return
	}
	alias.Name = strings.TrimSpace(alias.Name)
	if alias.Name == "" {
		return
	}
	key := authorAliasKey{authorID: alias.AuthorID, name: strings.ToLower(alias.Name)}
	if _, ok := m.aliasLoaded[key]; ok {
		return
	}
	m.aliasLoaded[key] = struct{}{}
	m.aliases = append(m.aliases, alias)
}

func (m *authorMatcher) getAuthor(ctx context.Context, id int64) (*models.Author, error) {
	if m == nil {
		return nil, nil
	}
	if a, ok := m.byID[id]; ok {
		cp := *a
		return &cp, nil
	}
	a, err := m.authors.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	m.addAuthor(a)
	if a == nil {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

// findAuthorByName looks up a local author whose name matches the supplied
// name. Matching proceeds in tiers: exact lowercase (author name, then alias),
// then exact via normalized variants (initials, suffixes, last-first swap),
// then Jaro-Winkler fuzzy matching. The returned matchedBy string distinguishes
// these tiers so callers can decide when to record a variant alias.
func (i *Importer) findAuthorByName(ctx context.Context, name string) (*models.Author, string, bool, error) {
	matcher, err := i.newAuthorMatcher(ctx)
	if err != nil {
		return nil, "", false, err
	}
	return matcher.findAuthorByName(ctx, name)
}

func (m *authorMatcher) findAuthorByName(ctx context.Context, name string) (*models.Author, string, bool, error) {
	// Tier 1: exact lowercase.
	needle := strings.ToLower(strings.TrimSpace(name))
	exact := make(map[int64]*models.Author)
	viaAlias := make(map[int64]bool)
	for _, author := range m.all {
		if strings.ToLower(strings.TrimSpace(author.Name)) == needle {
			cp := *author
			exact[cp.ID] = &cp
		}
	}
	for _, alias := range m.aliases {
		if strings.ToLower(strings.TrimSpace(alias.Name)) != needle {
			continue
		}
		author, err := m.getAuthor(ctx, alias.AuthorID)
		if err != nil {
			return nil, "", false, err
		}
		if author == nil {
			continue
		}
		if _, already := exact[author.ID]; !already {
			viaAlias[author.ID] = true
		}
		exact[author.ID] = author
	}
	if len(exact) == 1 {
		for id, author := range exact {
			matchedBy := "name"
			if viaAlias[id] {
				matchedBy = "alias"
			}
			return author, matchedBy, false, nil
		}
	}
	if len(exact) > 1 {
		return nil, "", true, nil
	}

	// Tier 2: exact via normalized variants.
	normExact := make(map[int64]*models.Author)
	normViaAlias := make(map[int64]bool)
	for _, author := range m.all {
		if textutil.MatchAuthorName(name, author.Name).Kind == textutil.AuthorMatchExact {
			cp := *author
			normExact[cp.ID] = &cp
		}
	}
	for _, alias := range m.aliases {
		if textutil.MatchAuthorName(name, alias.Name).Kind != textutil.AuthorMatchExact {
			continue
		}
		author, err := m.getAuthor(ctx, alias.AuthorID)
		if err != nil {
			return nil, "", false, err
		}
		if author == nil {
			continue
		}
		if _, already := normExact[author.ID]; !already {
			normViaAlias[author.ID] = true
		}
		normExact[author.ID] = author
	}
	if len(normExact) == 1 {
		for id, author := range normExact {
			matchedBy := "normalized_name"
			if normViaAlias[id] {
				matchedBy = "normalized_alias"
			}
			return author, matchedBy, false, nil
		}
	}
	if len(normExact) > 1 {
		return nil, "", true, nil
	}

	// Tier 3: Jaro-Winkler fuzzy match. Collect the best score per author
	// across both direct name and alias comparisons.
	type scored struct {
		author    *models.Author
		score     float64
		fromAlias bool
	}
	best := make(map[int64]*scored)
	consider := func(a *models.Author, score float64, fromAlias bool) {
		if a == nil {
			return
		}
		existing, ok := best[a.ID]
		if !ok || score > existing.score {
			best[a.ID] = &scored{author: a, score: score, fromAlias: fromAlias}
			return
		}
		if score == existing.score && existing.fromAlias && !fromAlias {
			// Prefer a direct-name match over alias when scores tie.
			existing.fromAlias = false
		}
	}
	for _, author := range m.all {
		res := textutil.MatchAuthorName(name, author.Name)
		if res.Score < textutil.AuthorMatchAmbiguousMinimum {
			continue
		}
		cp := *author
		consider(&cp, res.Score, false)
	}
	for _, alias := range m.aliases {
		res := textutil.MatchAuthorName(name, alias.Name)
		if res.Score < textutil.AuthorMatchAmbiguousMinimum {
			continue
		}
		author, err := m.getAuthor(ctx, alias.AuthorID)
		if err != nil {
			return nil, "", false, err
		}
		if author == nil {
			continue
		}
		consider(author, res.Score, true)
	}
	if len(best) == 0 {
		return nil, "", false, nil
	}

	var top *scored
	var second float64
	for _, s := range best {
		if top == nil || s.score > top.score {
			if top != nil {
				second = top.score
			}
			top = s
		} else if s.score > second {
			second = s.score
		}
	}
	if top.score >= textutil.AuthorMatchAutoThreshold {
		// Require a clear margin over any close runner-up before auto-matching.
		const fuzzyTieMargin = 0.02
		if len(best) > 1 && top.score-second < fuzzyTieMargin {
			return nil, "", true, nil
		}
		matchedBy := "fuzzy_name"
		if top.fromAlias {
			matchedBy = "fuzzy_alias"
		}
		return top.author, matchedBy, false, nil
	}
	// Best score is in the ambiguous band (0.88 <= score < 0.94): surface as
	// review rather than silently create or merge.
	return nil, "", true, nil
}

// shouldRecordAuthorVariantAlias returns true when the matchedBy tier is one
// that identifies the canonical author via a form different from the supplied
// ABS name, so recording the ABS form as an alias is helpful. "alias" and
// "name" are omitted because the ABS name already equals the alias/canonical
// name and re-recording would be a no-op.
func shouldRecordAuthorVariantAlias(matchedBy string) bool {
	switch matchedBy {
	case "normalized_name", "normalized_alias", "fuzzy_name", "fuzzy_alias":
		return true
	}
	return false
}

func (i *Importer) recordSecondaryAuthors(ctx context.Context, canonicalID int64, extras []NormalizedAuthor, matcher *authorMatcher) {
	if canonicalID == 0 || i.aliases == nil {
		return
	}
	for _, author := range extras {
		name := strings.TrimSpace(author.Name)
		if name == "" {
			continue
		}
		alias := &models.AuthorAlias{AuthorID: canonicalID, Name: name}
		if err := i.aliases.Create(ctx, alias); err != nil {
			slog.Debug("abs import: alias record skipped", "name", name, "error", err)
			continue
		}
		matcher.addAlias(*alias)
	}
}

func (i *Importer) recordAuthorVariantAlias(ctx context.Context, canonicalID int64, name string, matcher *authorMatcher) {
	if canonicalID == 0 || i.aliases == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	alias := &models.AuthorAlias{AuthorID: canonicalID, Name: name}
	if err := i.aliases.Create(ctx, alias); err != nil {
		slog.Debug("abs import: author variant alias skipped", "name", name, "error", err)
		return
	}
	matcher.addAlias(*alias)
}

func (i *Importer) enrichAuthor(ctx context.Context, cfg ImportConfig, item NormalizedLibraryItem, author *models.Author, matcher *authorMatcher) (metadataMergeResult, error) {
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
			i.recordAuthorVariantAlias(ctx, author.ID, oldName, matcher)
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
	matcher.addAuthor(author)
	return result, nil
}

func (i *Importer) lookupUpstreamAuthor(ctx context.Context, name string) (*models.Author, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, nil
	}
	const (
		exactScore     = 1.0
		fuzzyTieMargin = 0.02
	)
	var (
		best         *models.Author
		bestScore    float64
		secondScore  float64
		matchedQuery string
		sawAmbiguous bool
		exactHits    = make(map[string]struct{})
	)
	for _, query := range authorSearchQueries(name) {
		results, err := i.meta.SearchAuthors(ctx, query)
		if err != nil {
			return nil, false, err
		}
		for idx := range results {
			res := textutil.MatchAuthorName(name, results[idx].Name)
			var score float64
			switch res.Kind {
			case textutil.AuthorMatchExact:
				score = exactScore
			case textutil.AuthorMatchFuzzyAuto:
				score = res.Score
			case textutil.AuthorMatchFuzzyAmbiguous:
				sawAmbiguous = true
				continue
			default:
				continue
			}
			cp := results[idx]
			// Treat duplicates of the same upstream foreignID as the same
			// candidate rather than an ambiguity signal.
			if best != nil && best.ForeignID != "" && best.ForeignID == cp.ForeignID {
				if score > bestScore {
					bestScore = score
				}
				continue
			}
			if score >= exactScore {
				exactHits[cp.ForeignID] = struct{}{}
			}
			if best == nil || score > bestScore {
				secondScore = bestScore
				best = &cp
				bestScore = score
				matchedQuery = query
			} else if score > secondScore {
				secondScore = score
			}
		}
		if best != nil && bestScore >= exactScore {
			break
		}
	}
	if best == nil {
		if sawAmbiguous {
			slog.Info("abs import: upstream author match ambiguous band", "author", name)
			return nil, true, nil
		}
		slog.Debug("abs import: upstream author match not found", "author", name, "queries", authorSearchQueries(name))
		return nil, false, nil
	}
	if len(exactHits) > 1 {
		slog.Info("abs import: upstream author match ambiguous", "author", name, "hits", len(exactHits))
		return nil, true, nil
	}
	if bestScore < exactScore && bestScore-secondScore < fuzzyTieMargin {
		slog.Info("abs import: upstream author match ambiguous (tie)", "author", name, "best", bestScore, "second", secondScore)
		return nil, true, nil
	}
	full, err := i.meta.GetAuthor(ctx, best.ForeignID)
	if err != nil {
		return nil, false, err
	}
	slog.Info("abs import: upstream author matched", "author", name, "query", matchedQuery, "foreignId", best.ForeignID, "score", bestScore)
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

	return i.mergeUpstreamBook(ctx, cfg, item, book, full, matchedBy)
}

func (i *Importer) mergeUpstreamBook(ctx context.Context, cfg ImportConfig, item NormalizedLibraryItem, book *models.Book, full *models.Book, matchedBy string) (metadataMergeResult, error) {
	if book == nil || full == nil {
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

func bookSnapshot(book *models.Book) *bookRollbackSnapshot {
	if book == nil {
		return nil
	}
	return &bookRollbackSnapshot{
		ForeignID:             book.ForeignID,
		AuthorID:              book.AuthorID,
		Title:                 book.Title,
		SortTitle:             book.SortTitle,
		OriginalTitle:         book.OriginalTitle,
		Description:           book.Description,
		ImageURL:              book.ImageURL,
		ReleaseDate:           cloneTimePtr(book.ReleaseDate),
		Genres:                append([]string(nil), book.Genres...),
		AverageRating:         book.AverageRating,
		RatingsCount:          book.RatingsCount,
		Monitored:             book.Monitored,
		Status:                book.Status,
		AnyEditionOK:          book.AnyEditionOK,
		SelectedEditionID:     cloneInt64Ptr(book.SelectedEditionID),
		Language:              book.Language,
		MediaType:             book.MediaType,
		Narrator:              book.Narrator,
		DurationSeconds:       book.DurationSeconds,
		ASIN:                  book.ASIN,
		CalibreID:             cloneInt64Ptr(book.CalibreID),
		MetadataProvider:      book.MetadataProvider,
		LastMetadataRefreshAt: cloneTimePtr(book.LastMetadataRefreshAt),
	}
}

func bookSnapshotMetadata(data map[string]any, before, after *bookRollbackSnapshot) (runEntityMetadataEnvelope, error) {
	beforePayload, err := marshalSnapshotPayload(before)
	if err != nil {
		return runEntityMetadataEnvelope{}, err
	}
	afterPayload, err := marshalSnapshotPayload(after)
	if err != nil {
		return runEntityMetadataEnvelope{}, err
	}
	return runEntityMetadataEnvelope{
		Kind:    runEntityMetadataKind,
		Version: runEntityMetadataVersion,
		Data:    data,
		Snapshot: &runEntitySnapshotEnvelope{
			EntityType: entityTypeBook,
			Before:     beforePayload,
			After:      afterPayload,
		},
	}, nil
}

func authorSnapshot(author *models.Author) *authorRollbackSnapshot {
	if author == nil {
		return nil
	}
	return &authorRollbackSnapshot{
		ForeignID:             author.ForeignID,
		Name:                  author.Name,
		SortName:              author.SortName,
		Description:           author.Description,
		ImageURL:              author.ImageURL,
		Disambiguation:        author.Disambiguation,
		MetadataProvider:      author.MetadataProvider,
		LastMetadataRefreshAt: cloneTimePtr(author.LastMetadataRefreshAt),
	}
}

func authorSnapshotMetadata(data map[string]any, before, after *authorRollbackSnapshot) (runEntityMetadataEnvelope, error) {
	beforePayload, err := marshalSnapshotPayload(before)
	if err != nil {
		return runEntityMetadataEnvelope{}, err
	}
	afterPayload, err := marshalSnapshotPayload(after)
	if err != nil {
		return runEntityMetadataEnvelope{}, err
	}
	return runEntityMetadataEnvelope{
		Kind:    runEntityMetadataKind,
		Version: runEntityMetadataVersion,
		Data:    data,
		Snapshot: &runEntitySnapshotEnvelope{
			EntityType: entityTypeAuthor,
			Before:     beforePayload,
			After:      afterPayload,
		},
	}, nil
}

// marshalSnapshotPayload encodes a concrete snapshot struct into the
// envelope's RawMessage slot. A nil input returns a nil payload so the
// omitempty tag drops the field entirely.
func marshalSnapshotPayload(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	// Guard against typed-nil pointers: json.Marshal on a (*T)(nil) encodes
	// "null", which is indistinguishable from a genuinely absent snapshot
	// and would defeat before/after gating.
	switch t := v.(type) {
	case *bookRollbackSnapshot:
		if t == nil {
			return nil, nil
		}
	case *authorRollbackSnapshot:
		if t == nil {
			return nil, nil
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode abs rollback snapshot: %w", err)
	}
	return data, nil
}

func (i *Importer) recordBookBeforeSnapshot(ctx context.Context, runID int64, cfg ImportConfig, item NormalizedLibraryItem, externalID string, book *models.Book, outcome string, data map[string]any) error {
	if cfg.DryRun || book == nil {
		return nil
	}
	metadata, err := bookSnapshotMetadata(data, bookSnapshot(book), nil)
	if err != nil {
		return err
	}
	return i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, externalID, book.ID, outcome, metadata)
}

func (i *Importer) recordBookAfterSnapshot(ctx context.Context, runID int64, cfg ImportConfig, item NormalizedLibraryItem, bookID int64, outcome string, data map[string]any) error {
	if cfg.DryRun || bookID == 0 {
		return nil
	}
	book, err := i.books.GetByID(ctx, bookID)
	if err != nil || book == nil {
		return err
	}
	metadata, err := bookSnapshotMetadata(data, nil, bookSnapshot(book))
	if err != nil {
		return err
	}
	return i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeBook, item.ItemID, book.ID, outcome, metadata)
}

func (i *Importer) recordAuthorBeforeSnapshot(ctx context.Context, runID int64, cfg ImportConfig, item NormalizedLibraryItem, externalID string, author *models.Author, outcome string, data map[string]any) error {
	if cfg.DryRun || author == nil {
		return nil
	}
	metadata, err := authorSnapshotMetadata(data, authorSnapshot(author), nil)
	if err != nil {
		return err
	}
	return i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, author.ID, outcome, metadata)
}

func (i *Importer) recordAuthorAfterSnapshot(ctx context.Context, runID int64, cfg ImportConfig, item NormalizedLibraryItem, externalID string, authorID int64, outcome string, data map[string]any) error {
	if cfg.DryRun || authorID == 0 || i.authors == nil {
		return nil
	}
	author, err := i.authors.GetByID(ctx, authorID)
	if err != nil || author == nil {
		return err
	}
	metadata, err := authorSnapshotMetadata(data, nil, authorSnapshot(author))
	if err != nil {
		return err
	}
	return i.recordRunEntity(ctx, runID, cfg, item.LibraryID, item.ItemID, entityTypeAuthor, externalID, author.ID, outcome, metadata)
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
					if err := i.recordBookBeforeSnapshot(ctx, runID, cfg, item, externalID, existing, itemOutcomeUpdated, nil); err != nil {
						return nil, false, false, metadataMergeResult{}, err
					}
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
			if err := i.recordBookBeforeSnapshot(ctx, runID, cfg, item, externalID, existing, itemOutcomeUpdated, nil); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
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
			if err := i.recordBookBeforeSnapshot(ctx, runID, cfg, item, externalID, match, itemOutcomeLinked, nil); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
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
			if err := i.recordBookBeforeSnapshot(ctx, runID, cfg, item, item.ItemID, existing, itemOutcomeLinked, map[string]any{"matchedBy": "manual_book"}); err != nil {
				return nil, false, false, metadataMergeResult{}, err
			}
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

func (i *Importer) upsertSeries(ctx context.Context, cfg ImportConfig, runID, bookID int64, ref NormalizedSeries, stats *ImportStats) (bool, string, error) {
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
		if cfg.DryRun && stats != nil && stats.dryRunSeriesAlreadyPlanned(externalID, title) {
			return i.recordPlannedSeries(ctx, cfg, runID, bookID, externalID, ref, false, "planned")
		}
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
	metadata := map[string]any{
		"bookId":   bookID,
		"sequence": strings.TrimSpace(ref.Sequence),
	}
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
	_ = i.recordRunEntity(ctx, runID, cfg, cfg.LibraryID, "", entityTypeSeries, externalID, localID, outcome, metadata)
	if cfg.DryRun && created && stats != nil {
		stats.rememberDryRunSeries(externalID, title)
	}
	return created, matchedBy, nil
}

type hardcoverSeriesQuery struct {
	Title    string
	Sequence string
	FromABS  bool
}

type hardcoverSeriesCandidate struct {
	Result      metadata.SeriesSearchResult
	Catalog     *metadata.SeriesCatalog
	Book        metadata.SeriesCatalogBook
	Score       int
	MatchedBy   string
	SeriesScore int
	TitleScore  int
}

func (i *Importer) matchHardcoverSeries(ctx context.Context, cfg ImportConfig, runID int64, author *models.Author, book *models.Book, item NormalizedLibraryItem, stats *ImportStats) (metadataMergeResult, int) {
	if i.meta == nil || i.series == nil || i.books == nil || book == nil {
		return metadataMergeResult{}, 0
	}
	queries := hardcoverSeriesQueries(item)
	if len(queries) == 0 {
		return metadataMergeResult{}, 0
	}
	authorName := primaryAuthorName(item)
	if author != nil && strings.TrimSpace(author.Name) != "" {
		authorName = strings.TrimSpace(author.Name)
	}
	candidates := make(map[string]hardcoverSeriesCandidate)
	for _, query := range queries {
		results, err := i.meta.SearchSeries(ctx, query.Title, 5)
		if err != nil {
			slog.Warn("abs import: hardcover series search failed", "itemID", item.ItemID, "query", query.Title, "error", err)
			continue
		}
		for _, result := range results {
			catalog, err := i.meta.GetSeriesCatalog(ctx, result.ForeignID)
			if err != nil {
				slog.Warn("abs import: hardcover series catalog failed", "itemID", item.ItemID, "series", result.ForeignID, "error", err)
				continue
			}
			if catalog == nil {
				continue
			}
			candidate, ok := evaluateHardcoverSeriesCandidate(query, authorName, item, result, catalog)
			if !ok {
				continue
			}
			key := catalog.ForeignID
			if existing, exists := candidates[key]; !exists || candidate.Score > existing.Score {
				candidates[key] = candidate
			}
		}
	}
	selected, ambiguous := selectHardcoverSeriesCandidate(candidates)
	if ambiguous {
		return metadataMergeResult{Messages: []string{"hardcover series link skipped: match was ambiguous"}}, 0
	}
	if selected == nil {
		return metadataMergeResult{}, 0
	}

	created, matchedBy, err := i.upsertHardcoverSeries(ctx, cfg, runID, book.ID, selected.Catalog, selected.Book)
	if err != nil {
		slog.Warn("abs import: hardcover series link failed", "itemID", item.ItemID, "series", selected.Catalog.ForeignID, "error", err)
		return metadataMergeResult{}, 0
	}
	if stats != nil {
		if created {
			stats.SeriesCreated++
		} else if matchedBy != "" {
			stats.SeriesLinked++
		}
	}

	metaResult := metadataMergeResult{}
	if !cfg.DryRun && selected.Book.Book.ForeignID != "" {
		if mergeResult, err := i.mergeUpstreamBook(ctx, cfg, item, book, &selected.Book.Book, "hardcover_series"); err != nil {
			slog.Warn("abs import: hardcover series metadata merge failed", "itemID", item.ItemID, "book", selected.Book.Book.ForeignID, "error", err)
		} else {
			metaResult = mergeResult
		}
	}
	if !cfg.DryRun {
		extra, err := i.linkExistingHardcoverCatalogBooks(ctx, cfg, runID, author, selected.Catalog, book.ID, created)
		if err != nil {
			slog.Warn("abs import: hardcover catalog existing-book linking failed", "itemID", item.ItemID, "series", selected.Catalog.ForeignID, "error", err)
		} else if stats != nil {
			stats.SeriesLinked += extra
		}
	}
	if matchedBy != "" {
		metaResult.Messages = append(metaResult.Messages, fmt.Sprintf("hardcover series linked by %s", selected.MatchedBy))
	}
	return metaResult, 1
}

func hardcoverSeriesQueries(item NormalizedLibraryItem) []hardcoverSeriesQuery {
	queries := make([]hardcoverSeriesQuery, 0, len(item.Series)+1)
	seen := map[string]struct{}{}
	for _, series := range item.Series {
		title := strings.TrimSpace(series.Name)
		if title == "" {
			continue
		}
		key := normalizeTitle(title)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		queries = append(queries, hardcoverSeriesQuery{
			Title:    title,
			Sequence: strings.TrimSpace(series.Sequence),
			FromABS:  true,
		})
	}
	if len(queries) > 0 {
		return queries
	}
	if title := strings.TrimSpace(item.Title); title != "" {
		queries = append(queries, hardcoverSeriesQuery{Title: title})
	}
	return queries
}

func evaluateHardcoverSeriesCandidate(query hardcoverSeriesQuery, authorName string, item NormalizedLibraryItem, result metadata.SeriesSearchResult, catalog *metadata.SeriesCatalog) (hardcoverSeriesCandidate, bool) {
	if catalog == nil || catalog.ForeignID == "" || len(catalog.Books) == 0 {
		return hardcoverSeriesCandidate{}, false
	}
	hcAuthor := firstNonEmpty(result.AuthorName, catalog.AuthorName)
	authorScore := hardcoverAuthorScore(authorName, hcAuthor)
	if strings.TrimSpace(authorName) != "" && strings.TrimSpace(hcAuthor) != "" && authorScore == 0 {
		return hardcoverSeriesCandidate{}, false
	}
	matchedBook, titleScore, positionMatched, ok := matchHardcoverCatalogBook(item, query.Sequence, catalog.Books)
	if !ok {
		return hardcoverSeriesCandidate{}, false
	}
	seriesScore := 0
	if query.FromABS {
		seriesScore = shelfarrTitleScore(query.Title, firstNonEmpty(result.Title, catalog.Title))
		if normalizeSeriesName(query.Title) == normalizeSeriesName(firstNonEmpty(result.Title, catalog.Title)) {
			seriesScore = 100
		}
		if seriesScore < 80 {
			return hardcoverSeriesCandidate{}, false
		}
	}
	score := titleScore
	matchedBy := "hardcover_title"
	if positionMatched {
		score += 25
		matchedBy = "hardcover_position_title"
	}
	if query.FromABS {
		score += seriesScore / 2
		matchedBy = "hardcover_series_title"
		if positionMatched {
			matchedBy = "hardcover_series_position"
		}
	}
	score += authorScore
	if score < 105 {
		return hardcoverSeriesCandidate{}, false
	}
	return hardcoverSeriesCandidate{
		Result:      result,
		Catalog:     catalog,
		Book:        matchedBook,
		Score:       score,
		MatchedBy:   matchedBy,
		SeriesScore: seriesScore,
		TitleScore:  titleScore,
	}, true
}

func matchHardcoverCatalogBook(item NormalizedLibraryItem, sequence string, books []metadata.SeriesCatalogBook) (metadata.SeriesCatalogBook, int, bool, bool) {
	sequence = strings.TrimSpace(sequence)
	bestScore := 0
	var best metadata.SeriesCatalogBook
	matches := 0
	for _, book := range books {
		positionMatched := sequence != "" && sameSeriesPosition(sequence, book.Position)
		if sequence != "" && !positionMatched {
			continue
		}
		score := shelfarrTitleScore(item.Title, firstNonEmpty(book.Title, book.Book.Title))
		threshold := 88
		if positionMatched {
			threshold = 70
		}
		if score < threshold {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = book
			matches = 1
			continue
		}
		if score == bestScore {
			matches++
		}
	}
	if matches == 1 {
		return best, bestScore, sequence != "" && sameSeriesPosition(sequence, best.Position), true
	}
	return metadata.SeriesCatalogBook{}, 0, false, false
}

func selectHardcoverSeriesCandidate(candidates map[string]hardcoverSeriesCandidate) (*hardcoverSeriesCandidate, bool) {
	if len(candidates) == 0 {
		return nil, false
	}
	ordered := make([]hardcoverSeriesCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		ordered = append(ordered, candidate)
	}
	sort.SliceStable(ordered, func(a, b int) bool {
		if ordered[a].Score == ordered[b].Score {
			return ordered[a].Catalog.ForeignID < ordered[b].Catalog.ForeignID
		}
		return ordered[a].Score > ordered[b].Score
	})
	if len(ordered) > 1 && ordered[0].Score-ordered[1].Score < 10 {
		return nil, true
	}
	return &ordered[0], false
}

func (i *Importer) upsertHardcoverSeries(ctx context.Context, cfg ImportConfig, runID, bookID int64, catalog *metadata.SeriesCatalog, matchedBook metadata.SeriesCatalogBook) (bool, string, error) {
	if catalog == nil || strings.TrimSpace(catalog.Title) == "" || strings.TrimSpace(catalog.ForeignID) == "" {
		return false, "", nil
	}
	existing, err := i.series.GetByForeignID(ctx, catalog.ForeignID)
	if err != nil {
		return false, "", err
	}
	matchedBy := ""
	if existing == nil {
		match, ambiguous, err := i.findSeriesByTitle(ctx, catalog.Title)
		if err != nil {
			return false, "", err
		}
		if ambiguous {
			return false, "", fmt.Errorf("ambiguous existing series match for %q", catalog.Title)
		}
		if match != nil {
			existing = match
			matchedBy = "normalized_title"
			if shouldPromoteSeriesToHardcover(match, catalog) {
				if prior, err := i.series.GetByForeignID(ctx, catalog.ForeignID); err != nil {
					return false, "", err
				} else if prior == nil {
					if !cfg.DryRun {
						if err := i.series.UpdateForeignID(ctx, existing.ID, catalog.ForeignID); err != nil {
							return false, "", err
						}
					}
					existing.ForeignID = catalog.ForeignID
					matchedBy = "hardcover_promotion"
				}
			}
		}
	}
	created := false
	if existing == nil {
		existing = &models.Series{
			ForeignID:   catalog.ForeignID,
			Title:       strings.TrimSpace(catalog.Title),
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
	localID := existing.ID
	if cfg.DryRun && created {
		localID = 0
	}
	metadata := map[string]any{
		"bookId":          bookID,
		"sequence":        strings.TrimSpace(matchedBook.Position),
		"matchedBy":       "hardcover_series",
		"hardcoverBookId": strings.TrimSpace(matchedBook.ForeignID),
	}
	linkExternalID := hardcoverSeriesLinkExternalID(catalog.ForeignID, bookID)
	linkCreated := cfg.DryRun
	if !cfg.DryRun {
		var err error
		linkCreated, err = i.series.LinkBookIfMissing(ctx, existing.ID, bookID, strings.TrimSpace(matchedBook.Position), true)
		if err != nil {
			return false, "", err
		}
		if linkCreated {
			if err := i.upsertProvenance(ctx, &models.ABSProvenance{
				SourceID:    cfg.SourceID,
				LibraryID:   cfg.LibraryID,
				EntityType:  entityTypeSeries,
				ExternalID:  linkExternalID,
				LocalID:     existing.ID,
				ItemID:      "",
				ImportRunID: ptrInt64(runID),
			}); err != nil {
				return false, "", err
			}
		}
	}
	outcome := itemOutcomeLinked
	if created {
		outcome = itemOutcomeCreated
	}
	if linkCreated {
		_ = i.recordRunEntity(ctx, runID, cfg, cfg.LibraryID, "", entityTypeSeries, linkExternalID, localID, outcome, metadata)
	}
	return created, matchedBy, nil
}

func (i *Importer) linkExistingHardcoverCatalogBooks(ctx context.Context, cfg ImportConfig, runID int64, author *models.Author, catalog *metadata.SeriesCatalog, importedBookID int64, createdSeries bool) (int, error) {
	if catalog == nil || author == nil {
		return 0, nil
	}
	seriesRow, err := i.series.GetByForeignID(ctx, catalog.ForeignID)
	if err != nil {
		return 0, err
	}
	if seriesRow == nil {
		match, ambiguous, err := i.findSeriesByTitle(ctx, catalog.Title)
		if err != nil || ambiguous {
			return 0, err
		}
		seriesRow = match
	}
	if seriesRow == nil {
		return 0, nil
	}
	linked := 0
	for _, catalogBook := range catalog.Books {
		localBook, err := i.findExistingHardcoverCatalogBook(ctx, author.ID, catalogBook)
		if err != nil {
			return linked, err
		}
		if localBook == nil || localBook.ID == importedBookID {
			continue
		}
		if catalogBook.Book.Author != nil && textutil.MatchAuthorName(author.Name, catalogBook.Book.Author.Name).Kind == textutil.AuthorMatchNone {
			continue
		}
		linkCreated, err := i.series.LinkBookIfMissing(ctx, seriesRow.ID, localBook.ID, strings.TrimSpace(catalogBook.Position), false)
		if err != nil {
			return linked, err
		}
		if !linkCreated {
			continue
		}
		linkExternalID := hardcoverSeriesLinkExternalID(catalog.ForeignID, localBook.ID)
		if err := i.upsertProvenance(ctx, &models.ABSProvenance{
			SourceID:    cfg.SourceID,
			LibraryID:   cfg.LibraryID,
			EntityType:  entityTypeSeries,
			ExternalID:  linkExternalID,
			LocalID:     seriesRow.ID,
			ItemID:      "",
			ImportRunID: ptrInt64(runID),
		}); err != nil {
			return linked, err
		}
		outcome := itemOutcomeLinked
		if createdSeries {
			outcome = itemOutcomeCreated
		}
		_ = i.recordRunEntity(ctx, runID, cfg, cfg.LibraryID, "", entityTypeSeries, linkExternalID, seriesRow.ID, outcome, map[string]any{
			"bookId":          localBook.ID,
			"sequence":        strings.TrimSpace(catalogBook.Position),
			"matchedBy":       "hardcover_catalog_existing_book",
			"hardcoverBookId": strings.TrimSpace(catalogBook.ForeignID),
		})
		linked++
	}
	return linked, nil
}

func (i *Importer) findExistingHardcoverCatalogBook(ctx context.Context, authorID int64, catalogBook metadata.SeriesCatalogBook) (*models.Book, error) {
	if strings.TrimSpace(catalogBook.ForeignID) != "" {
		existing, err := i.books.GetByForeignID(ctx, catalogBook.ForeignID)
		if err != nil || existing != nil {
			return existing, err
		}
	}
	title := firstNonEmpty(catalogBook.Title, catalogBook.Book.Title)
	match, ambiguous, err := i.findBookByNormalizedTitle(ctx, authorID, title)
	if err != nil || ambiguous || match == nil {
		return nil, err
	}
	if shelfarrTitleScore(match.Title, title) < 92 {
		return nil, nil
	}
	return match, nil
}

func shouldPromoteSeriesToHardcover(existing *models.Series, catalog *metadata.SeriesCatalog) bool {
	if existing == nil || catalog == nil {
		return false
	}
	if !strings.HasPrefix(existing.ForeignID, "abs:series:") {
		return false
	}
	return normalizeSeriesName(existing.Title) == normalizeSeriesName(catalog.Title)
}

func hardcoverSeriesLinkExternalID(seriesForeignID string, bookID int64) string {
	seriesForeignID = strings.TrimSpace(seriesForeignID)
	if bookID <= 0 {
		return seriesForeignID + ":book:planned"
	}
	return fmt.Sprintf("%s:book:%d", seriesForeignID, bookID)
}

func hardcoverAuthorScore(absAuthor, hcAuthor string) int {
	absAuthor = strings.TrimSpace(absAuthor)
	hcAuthor = strings.TrimSpace(hcAuthor)
	if absAuthor == "" || hcAuthor == "" {
		return 0
	}
	match := textutil.MatchAuthorName(absAuthor, hcAuthor)
	switch match.Kind {
	case textutil.AuthorMatchExact:
		return 30
	case textutil.AuthorMatchFuzzyAuto:
		return 20
	default:
		score := shelfarrTitleScore(absAuthor, hcAuthor)
		if score >= 90 {
			return 15
		}
		return 0
	}
}

func sameSeriesPosition(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	af, aerr := strconv.ParseFloat(a, 64)
	bf, berr := strconv.ParseFloat(b, 64)
	return aerr == nil && berr == nil && math.Abs(af-bf) < 0.001
}

func normalizeSeriesName(name string) string {
	normalized := normalizeTitle(name)
	if normalized == "" {
		return ""
	}
	suffixes := map[string]struct{}{
		"series":     {},
		"trilogy":    {},
		"saga":       {},
		"chronicles": {},
		"cycle":      {},
		"books":      {},
		"novels":     {},
	}
	words := strings.Fields(normalized)
	if len(words) > 1 {
		if _, ok := suffixes[words[len(words)-1]]; ok {
			words = words[:len(words)-1]
		}
	}
	return strings.Join(words, " ")
}

func shelfarrTitleScore(a, b string) int {
	cleanA := shelfarrCleanTitle(a)
	cleanB := shelfarrCleanTitle(b)
	if cleanA == "" || cleanB == "" {
		return 0
	}
	return maxInt(
		fuzzy.TokenSetRatio(cleanA, cleanB),
		fuzzy.TokenSortRatio(cleanA, cleanB),
		fuzzy.Ratio(cleanA, cleanB),
		fuzzy.PartialRatio(cleanA, cleanB),
	)
}

func shelfarrCleanTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	if title == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range title {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(' ')
		}
	}
	noise := map[string]struct{}{
		"a":     {},
		"an":    {},
		"the":   {},
		"novel": {},
		"book":  {},
	}
	words := strings.Fields(b.String())
	out := words[:0]
	for _, word := range words {
		if _, ok := noise[word]; ok {
			continue
		}
		out = append(out, word)
	}
	return strings.Join(out, " ")
}

func maxInt(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func (i *Importer) recordPlannedSeries(ctx context.Context, cfg ImportConfig, runID, bookID int64, externalID string, ref NormalizedSeries, created bool, matchedBy string) (bool, string, error) {
	metadata := map[string]any{
		"bookId":   bookID,
		"sequence": strings.TrimSpace(ref.Sequence),
	}
	outcome := itemOutcomeLinked
	if created {
		outcome = itemOutcomeCreated
	}
	_ = i.recordRunEntity(ctx, runID, cfg, cfg.LibraryID, "", entityTypeSeries, externalID, 0, outcome, metadata)
	return created, matchedBy, nil
}

func (s *ImportStats) dryRunSeriesAlreadyPlanned(externalID, title string) bool {
	if s == nil {
		return false
	}
	if _, ok := s.dryRunSeriesExternalIDs[strings.TrimSpace(externalID)]; ok {
		return true
	}
	if _, ok := s.dryRunSeriesTitles[normalizeTitle(title)]; ok {
		return true
	}
	return false
}

func (s *ImportStats) rememberDryRunSeries(externalID, title string) {
	if s.dryRunSeriesExternalIDs == nil {
		s.dryRunSeriesExternalIDs = make(map[string]struct{})
	}
	if s.dryRunSeriesTitles == nil {
		s.dryRunSeriesTitles = make(map[string]struct{})
	}
	s.dryRunSeriesExternalIDs[strings.TrimSpace(externalID)] = struct{}{}
	s.dryRunSeriesTitles[normalizeTitle(title)] = struct{}{}
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

func (i *Importer) deleteProvenanceByLocal(ctx context.Context, entityType string, localID int64) (int, error) {
	if i.provenance == nil || localID == 0 {
		return 0, nil
	}
	count, err := i.provenance.DeleteByLocal(ctx, entityType, localID)
	return int(count), err
}

type RollbackStats struct {
	ActionsPlanned     int `json:"actionsPlanned"`
	EntitiesDeleted    int `json:"entitiesDeleted"`
	ProvenanceUnlinked int `json:"provenanceUnlinked"`
	Skipped            int `json:"skipped"`
	Failed             int `json:"failed"`
}

type RollbackAction struct {
	EntityType  string `json:"entityType"`
	ExternalID  string `json:"externalId"`
	LocalID     int64  `json:"localId"`
	DisplayName string `json:"displayName,omitempty"`
	Outcome     string `json:"outcome"`
	Action      string `json:"action"`
	Reason      string `json:"reason,omitempty"`
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
	// Nil-repo guards: every case below relies on at least one of these, and a
	// silent nil-deref would turn "safe rollback" into a crash that leaves the
	// run in an inconsistent half-rolled state.
	if i.provenance == nil || i.books == nil || i.authors == nil || i.series == nil || i.editions == nil {
		return nil, errors.New("abs rollback is unavailable: one or more repositories are not configured")
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
	createdBookEntities := make(map[int64]models.ABSImportRunEntity)
	for _, entity := range entities {
		if entity.EntityType == entityTypeBook && entity.Outcome == itemOutcomeCreated && entity.LocalID != 0 {
			createdBookEntities[entity.LocalID] = entity
		}
	}
	for _, entity := range entities {
		current, err := i.provenance.GetByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID)
		if err != nil {
			result.Stats.Failed++
			result.Actions = append(result.Actions, RollbackAction{
				EntityType:  entity.EntityType,
				ExternalID:  entity.ExternalID,
				LocalID:     entity.LocalID,
				DisplayName: i.rollbackActionDisplayName(ctx, entity),
				Outcome:     entity.Outcome,
				Action:      "inspect",
				Reason:      err.Error(),
			})
			continue
		}
		// NOTE: Intentionally no blanket "current owner must equal runID" gate
		// here. A shared-entity restore (book/author snapshot) is safe to run
		// field-by-field even if the provenance now points to another run —
		// restoreFromSnapshot only reverts fields where current == after, so
		// post-import edits or later re-imports stay intact. Destructive cases
		// (delete_book, delete_edition, delete_author, unlink_provenance) still
		// check ownership per-case below.
		if current == nil {
			result.Stats.Skipped++
			result.Actions = append(result.Actions, RollbackAction{
				EntityType:  entity.EntityType,
				ExternalID:  entity.ExternalID,
				LocalID:     entity.LocalID,
				DisplayName: i.rollbackActionDisplayName(ctx, entity),
				Outcome:     entity.Outcome,
				Action:      "skip",
				Reason:      "already rolled back",
			})
			continue
		}
		currentMatchesEntity := current.LocalID == entity.LocalID
		ownedByRun := currentMatchesEntity && current.ImportRunID != nil && *current.ImportRunID == runID
		if !currentMatchesEntity {
			result.Stats.Skipped++
			result.Actions = append(result.Actions, RollbackAction{
				EntityType:  entity.EntityType,
				ExternalID:  entity.ExternalID,
				LocalID:     entity.LocalID,
				DisplayName: i.rollbackActionDisplayName(ctx, entity),
				Outcome:     entity.Outcome,
				Action:      "skip",
				Reason:      "provenance now points to a different local entity",
			})
			continue
		}

		action := RollbackAction{
			EntityType:  entity.EntityType,
			ExternalID:  entity.ExternalID,
			LocalID:     entity.LocalID,
			DisplayName: i.rollbackActionDisplayName(ctx, entity),
			Outcome:     entity.Outcome,
		}
		metadata := runEntityMetadataData(entity.MetadataJSON)
		bookBefore, bookAfter, hasBookSnapshot := bookRollbackSnapshotFromMetadata(entity.MetadataJSON)
		authorBefore, authorAfter, hasAuthorSnapshot := authorRollbackSnapshotFromMetadata(entity.MetadataJSON)
		switch {
		case entity.EntityType == entityTypeBook && entity.Outcome == itemOutcomeCreated:
			if !ownedByRun {
				action.Action = "skip"
				action.Reason = "run is no longer the current provenance owner for this book"
				result.Stats.Skipped++
				result.Actions = append(result.Actions, action)
				continue
			}
			action.Action = "delete_book"
			result.Stats.ActionsPlanned++
			if !preview {
				if err := i.books.Delete(ctx, entity.LocalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				unlinked, err := i.deleteProvenanceByLocal(ctx, entity.EntityType, entity.LocalID)
				if err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				deletedBooks[entity.LocalID] = struct{}{}
				result.Stats.EntitiesDeleted++
				result.Stats.ProvenanceUnlinked += unlinked
			}
		case entity.EntityType == entityTypeBook && hasBookSnapshot:
			// Snapshot restore is safe to attempt regardless of provenance
			// ownership: restoreBookFromSnapshot only reverts fields where the
			// current value still equals the post-import ("after") snapshot,
			// so post-import user edits stay intact and a shared canonical book
			// owned by another run isn't harmed.
			action.Action = "restore_book"
			result.Stats.ActionsPlanned++
			if !preview {
				book, err := i.books.GetByID(ctx, entity.LocalID)
				if err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				if book == nil {
					action.Action = "skip"
					action.Reason = "book no longer exists"
					result.Stats.Skipped++
					result.Actions = append(result.Actions, action)
					continue
				}
				if restoreBookFromSnapshot(book, bookBefore, bookAfter) {
					if err := i.books.Update(ctx, book); err != nil {
						action.Action = "skip"
						action.Reason = err.Error()
						result.Stats.Failed++
						result.Actions = append(result.Actions, action)
						continue
					}
				}
				if ownedByRun {
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
			if !ownedByRun {
				action.Action = "skip"
				action.Reason = "run is no longer the current provenance owner for this author"
				result.Stats.Skipped++
				result.Actions = append(result.Actions, action)
				continue
			}
			books, err := i.books.ListByAuthorIncludingExcluded(ctx, entity.LocalID)
			if err != nil {
				action.Action = "skip"
				action.Reason = err.Error()
				result.Stats.Failed++
				result.Actions = append(result.Actions, action)
				continue
			}
			remaining := 0
			blocked := false
			for _, book := range books {
				if _, ok := deletedBooks[book.ID]; ok {
					continue
				}
				if bookEntity, ok := createdBookEntities[book.ID]; ok {
					bookCurrent, err := i.provenance.GetByExternal(ctx, bookEntity.SourceID, bookEntity.LibraryID, bookEntity.EntityType, bookEntity.ExternalID)
					if err != nil {
						action.Action = "skip"
						action.Reason = err.Error()
						result.Stats.Failed++
						blocked = true
						break
					}
					if bookCurrent != nil && bookCurrent.ImportRunID != nil && *bookCurrent.ImportRunID == runID {
						if !preview {
							if err := i.books.Delete(ctx, book.ID); err != nil {
								action.Action = "skip"
								action.Reason = err.Error()
								result.Stats.Failed++
								blocked = true
								break
							}
							unlinked, err := i.deleteProvenanceByLocal(ctx, bookEntity.EntityType, book.ID)
							if err != nil {
								action.Action = "skip"
								action.Reason = err.Error()
								result.Stats.Failed++
								blocked = true
								break
							}
							deletedBooks[book.ID] = struct{}{}
							result.Stats.EntitiesDeleted++
							result.Stats.ProvenanceUnlinked += unlinked
						}
						continue
					}
				}
				remaining++
			}
			if blocked {
				result.Actions = append(result.Actions, action)
				continue
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
				if err := i.authors.Delete(ctx, entity.LocalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				unlinked, err := i.deleteProvenanceByLocal(ctx, entity.EntityType, entity.LocalID)
				if err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				result.Stats.EntitiesDeleted++
				result.Stats.ProvenanceUnlinked += unlinked
			}
		case entity.EntityType == entityTypeAuthor && hasAuthorSnapshot:
			// Same safety argument as the book snapshot case: field-level
			// restore preserves post-import author edits and won't trample
			// canonical shared data owned by another run.
			action.Action = "restore_author"
			result.Stats.ActionsPlanned++
			if !preview {
				author, err := i.authors.GetByID(ctx, entity.LocalID)
				if err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				if author == nil {
					action.Action = "skip"
					action.Reason = "author no longer exists"
					result.Stats.Skipped++
					result.Actions = append(result.Actions, action)
					continue
				}
				if restoreAuthorFromSnapshot(author, authorBefore, authorAfter) {
					if err := i.authors.Update(ctx, author); err != nil {
						action.Action = "skip"
						action.Reason = err.Error()
						result.Stats.Failed++
						result.Actions = append(result.Actions, action)
						continue
					}
				}
				if ownedByRun {
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
			if !ownedByRun {
				action.Action = "skip"
				action.Reason = "run is no longer the current provenance owner for this edition"
				result.Stats.Skipped++
				result.Actions = append(result.Actions, action)
				continue
			}
			action.Action = "delete_edition"
			result.Stats.ActionsPlanned++
			if !preview {
				if err := i.editions.Delete(ctx, entity.LocalID); err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				unlinked, err := i.deleteProvenanceByLocal(ctx, entity.EntityType, entity.LocalID)
				if err != nil {
					action.Action = "skip"
					action.Reason = err.Error()
					result.Stats.Failed++
					result.Actions = append(result.Actions, action)
					continue
				}
				result.Stats.EntitiesDeleted++
				result.Stats.ProvenanceUnlinked += unlinked
			}
		case entity.EntityType == entityTypeSeries:
			if !ownedByRun {
				action.Action = "skip"
				action.Reason = "run is no longer the current provenance owner for this series"
				result.Stats.Skipped++
				result.Actions = append(result.Actions, action)
				continue
			}
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
								if err := i.series.Delete(ctx, entity.LocalID); err != nil {
									action.Action = "skip"
									action.Reason = err.Error()
									result.Stats.Failed++
									result.Actions = append(result.Actions, action)
									continue
								}
								result.Stats.EntitiesDeleted++
							}
							action.Action = "delete_series"
						}
					}
				}
				if !preview {
					if action.Action == "delete_series" {
						unlinked, err := i.deleteProvenanceByLocal(ctx, entity.EntityType, entity.LocalID)
						if err != nil {
							action.Action = "skip"
							action.Reason = err.Error()
							result.Stats.Failed++
							result.Actions = append(result.Actions, action)
							continue
						}
						result.Stats.ProvenanceUnlinked += unlinked
					} else if err := i.provenance.DeleteByExternal(ctx, entity.SourceID, entity.LibraryID, entity.EntityType, entity.ExternalID); err == nil {
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
		default:
			if !ownedByRun {
				action.Action = "skip"
				action.Reason = "run is no longer the current provenance owner for this entity"
				result.Stats.Skipped++
				result.Actions = append(result.Actions, action)
				continue
			}
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

func runEntityMetadataData(raw string) map[string]any {
	envelope, ok := parseRunEntityMetadata(raw)
	if ok {
		return envelope.Data
	}
	return parseJSONObject(raw)
}

func bookRollbackSnapshotFromMetadata(raw string) (*bookRollbackSnapshot, *bookRollbackSnapshot, bool) {
	envelope, ok := parseRunEntityMetadata(raw)
	if !ok || envelope.Snapshot == nil || envelope.Snapshot.EntityType != entityTypeBook {
		return nil, nil, false
	}
	if len(envelope.Snapshot.Before) == 0 || len(envelope.Snapshot.After) == 0 {
		return nil, nil, false
	}
	var before, after bookRollbackSnapshot
	if err := json.Unmarshal(envelope.Snapshot.Before, &before); err != nil {
		return nil, nil, false
	}
	if err := json.Unmarshal(envelope.Snapshot.After, &after); err != nil {
		return nil, nil, false
	}
	return &before, &after, true
}

func authorRollbackSnapshotFromMetadata(raw string) (*authorRollbackSnapshot, *authorRollbackSnapshot, bool) {
	envelope, ok := parseRunEntityMetadata(raw)
	if !ok || envelope.Snapshot == nil || envelope.Snapshot.EntityType != entityTypeAuthor {
		return nil, nil, false
	}
	if len(envelope.Snapshot.Before) == 0 || len(envelope.Snapshot.After) == 0 {
		return nil, nil, false
	}
	var before, after authorRollbackSnapshot
	if err := json.Unmarshal(envelope.Snapshot.Before, &before); err != nil {
		return nil, nil, false
	}
	if err := json.Unmarshal(envelope.Snapshot.After, &after); err != nil {
		return nil, nil, false
	}
	return &before, &after, true
}

func parseRunEntityMetadata(raw string) (runEntityMetadataEnvelope, bool) {
	var envelope runEntityMetadataEnvelope
	if strings.TrimSpace(raw) == "" {
		return envelope, false
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return envelope, false
	}
	if envelope.Kind != runEntityMetadataKind || envelope.Version != runEntityMetadataVersion {
		return envelope, false
	}
	if envelope.Data == nil {
		envelope.Data = map[string]any{}
	}
	return envelope, true
}

func restoreBookFromSnapshot(book *models.Book, before, after *bookRollbackSnapshot) bool {
	if book == nil || before == nil || after == nil {
		return false
	}
	changed := false
	restoreString(&book.ForeignID, before.ForeignID, after.ForeignID, &changed)
	restoreInt64(&book.AuthorID, before.AuthorID, after.AuthorID, &changed)
	restoreString(&book.Title, before.Title, after.Title, &changed)
	restoreString(&book.SortTitle, before.SortTitle, after.SortTitle, &changed)
	restoreString(&book.OriginalTitle, before.OriginalTitle, after.OriginalTitle, &changed)
	restoreString(&book.Description, before.Description, after.Description, &changed)
	restoreString(&book.ImageURL, before.ImageURL, after.ImageURL, &changed)
	restoreTimePtr(&book.ReleaseDate, before.ReleaseDate, after.ReleaseDate, &changed)
	restoreStrings(&book.Genres, before.Genres, after.Genres, &changed)
	restoreFloat64(&book.AverageRating, before.AverageRating, after.AverageRating, &changed)
	restoreInt(&book.RatingsCount, before.RatingsCount, after.RatingsCount, &changed)
	restoreBool(&book.Monitored, before.Monitored, after.Monitored, &changed)
	restoreString(&book.Status, before.Status, after.Status, &changed)
	restoreBool(&book.AnyEditionOK, before.AnyEditionOK, after.AnyEditionOK, &changed)
	restoreInt64Ptr(&book.SelectedEditionID, before.SelectedEditionID, after.SelectedEditionID, &changed)
	restoreString(&book.Language, before.Language, after.Language, &changed)
	restoreString(&book.MediaType, before.MediaType, after.MediaType, &changed)
	restoreString(&book.Narrator, before.Narrator, after.Narrator, &changed)
	restoreInt(&book.DurationSeconds, before.DurationSeconds, after.DurationSeconds, &changed)
	restoreString(&book.ASIN, before.ASIN, after.ASIN, &changed)
	restoreInt64Ptr(&book.CalibreID, before.CalibreID, after.CalibreID, &changed)
	restoreString(&book.MetadataProvider, before.MetadataProvider, after.MetadataProvider, &changed)
	restoreTimePtr(&book.LastMetadataRefreshAt, before.LastMetadataRefreshAt, after.LastMetadataRefreshAt, &changed)
	return changed
}

// restoreAuthorFromSnapshot mirrors restoreBookFromSnapshot: it only touches
// fields the importer writes, and only when the current value still matches
// the post-import snapshot (so a post-import user edit stays intact).
func restoreAuthorFromSnapshot(author *models.Author, before, after *authorRollbackSnapshot) bool {
	if author == nil || before == nil || after == nil {
		return false
	}
	changed := false
	restoreString(&author.ForeignID, before.ForeignID, after.ForeignID, &changed)
	restoreString(&author.Name, before.Name, after.Name, &changed)
	restoreString(&author.SortName, before.SortName, after.SortName, &changed)
	restoreString(&author.Description, before.Description, after.Description, &changed)
	restoreString(&author.ImageURL, before.ImageURL, after.ImageURL, &changed)
	restoreString(&author.Disambiguation, before.Disambiguation, after.Disambiguation, &changed)
	restoreString(&author.MetadataProvider, before.MetadataProvider, after.MetadataProvider, &changed)
	restoreTimePtr(&author.LastMetadataRefreshAt, before.LastMetadataRefreshAt, after.LastMetadataRefreshAt, &changed)
	return changed
}

func restoreString(target *string, before, after string, changed *bool) {
	if *target != after {
		return
	}
	if *target != before {
		*changed = true
	}
	*target = before
}

func restoreInt(target *int, before, after int, changed *bool) {
	if *target != after {
		return
	}
	if *target != before {
		*changed = true
	}
	*target = before
}

func restoreInt64(target *int64, before, after int64, changed *bool) {
	if *target != after {
		return
	}
	if *target != before {
		*changed = true
	}
	*target = before
}

func restoreFloat64(target *float64, before, after float64, changed *bool) {
	if *target != after {
		return
	}
	if *target != before {
		*changed = true
	}
	*target = before
}

func restoreBool(target *bool, before, after bool, changed *bool) {
	if *target != after {
		return
	}
	if *target != before {
		*changed = true
	}
	*target = before
}

func restoreStrings(target *[]string, before, after []string, changed *bool) {
	if !equalStrings(*target, after) {
		return
	}
	if !equalStrings(*target, before) {
		*changed = true
	}
	*target = append([]string(nil), before...)
}

func restoreInt64Ptr(target **int64, before, after *int64, changed *bool) {
	if !equalInt64Ptr(*target, after) {
		return
	}
	if !equalInt64Ptr(*target, before) {
		*changed = true
	}
	*target = cloneInt64Ptr(before)
}

func restoreTimePtr(target **time.Time, before, after *time.Time, changed *bool) {
	if !equalTimePtr(*target, after) {
		return
	}
	if !equalTimePtr(*target, before) {
		*changed = true
	}
	*target = cloneTimePtr(before)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func equalInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (i *Importer) rollbackActionDisplayName(ctx context.Context, entity models.ABSImportRunEntity) string {
	if entity.LocalID == 0 {
		return ""
	}
	switch entity.EntityType {
	case entityTypeBook:
		if i.books == nil {
			return ""
		}
		book, err := i.books.GetByID(ctx, entity.LocalID)
		if err != nil || book == nil {
			return ""
		}
		return strings.TrimSpace(book.Title)
	case entityTypeAuthor:
		if i.authors == nil {
			return ""
		}
		author, err := i.authors.GetByID(ctx, entity.LocalID)
		if err != nil || author == nil {
			return ""
		}
		return strings.TrimSpace(author.Name)
	case entityTypeSeries:
		if i.series == nil {
			return ""
		}
		series, err := i.series.GetByID(ctx, entity.LocalID)
		if err != nil || series == nil {
			return ""
		}
		return strings.TrimSpace(series.Title)
	case entityTypeEdition:
		if i.editions == nil {
			return ""
		}
		edition, err := i.editions.GetByForeignID(ctx, absForeignID("edition", entity.LibraryID, entity.ExternalID))
		if err != nil || edition == nil {
			return ""
		}
		return strings.TrimSpace(edition.Title)
	default:
		return ""
	}
}

func (i *Importer) recordRunEntity(ctx context.Context, runID int64, cfg ImportConfig, libraryID, itemID, entityType, externalID string, localID int64, outcome string, metadata any) error {
	if runID == 0 || i.runEntities == nil {
		return nil
	}
	metadataJSON, err := encodeJSON(metadata)
	if err != nil {
		err = fmt.Errorf("encode abs import run entity metadata: %w", err)
		slog.Warn("abs import: encode run entity metadata failed",
			"runID", runID,
			"libraryID", libraryID,
			"itemID", itemID,
			"entityType", entityType,
			"externalID", externalID,
			"localID", localID,
			"error", err)
		return err
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
		MetadataJSON: metadataJSON,
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

func appendImportProgressResult(results []ImportItemResult, result ImportItemResult) []ImportItemResult {
	if len(results) < importProgressResultsLimit {
		return append(results, result)
	}
	copy(results, results[len(results)-importProgressResultsLimit+1:])
	results[importProgressResultsLimit-1] = result
	return results[:importProgressResultsLimit]
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

func encodeJSON(value any) (string, error) {
	if value == nil {
		return "{}", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("json marshal produced empty payload")
	}
	return string(data), nil
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
	return decodeImportCheckpoint(setting.Value)
}

func decodeImportCheckpoint(raw string) (*ImportCheckpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return nil, nil
	}
	var checkpoint ImportCheckpoint
	if err := json.Unmarshal([]byte(raw), &checkpoint); err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

func resumeConfigFromRun(run models.ABSImportRun, fallback ImportConfig) ImportConfig {
	cfg := fallback.normalized()
	var source ImportSourceSnapshot
	rawSource := strings.TrimSpace(run.SourceConfigJSON)
	hasSource := rawSource != "" && rawSource != "{}" && rawSource != "null"
	if hasSource {
		if err := json.Unmarshal([]byte(rawSource), &source); err != nil {
			hasSource = false
		}
	}
	if hasSource {
		cfg.SourceID = firstNonEmpty(source.SourceID, run.SourceID, cfg.SourceID)
		cfg.BaseURL = firstNonEmpty(source.BaseURL, run.BaseURL, cfg.BaseURL)
		cfg.LibraryID = firstNonEmpty(source.LibraryID, run.LibraryID, cfg.LibraryID)
		cfg.Label = firstNonEmpty(source.Label, run.SourceLabel, cfg.Label)
		cfg.PathRemap = source.PathRemap
	} else {
		cfg.SourceID = firstNonEmpty(run.SourceID, cfg.SourceID)
		cfg.BaseURL = firstNonEmpty(run.BaseURL, cfg.BaseURL)
		cfg.LibraryID = firstNonEmpty(run.LibraryID, cfg.LibraryID)
		cfg.Label = firstNonEmpty(run.SourceLabel, cfg.Label)
	}
	cfg.DryRun = run.DryRun
	return cfg.normalized()
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

// rollbackEntityRank orders entities for rollback so children (editions) are
// unwound before parents (books, series, authors). Editions first avoids
// leaving dangling edition→book FK references while we restore the book;
// authors last so book-count preconditions have already been reduced by
// prior book-delete steps.
func rollbackEntityRank(entity models.ABSImportRunEntity) int {
	switch entity.EntityType {
	case entityTypeEdition:
		return 0
	case entityTypeBook:
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

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
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
