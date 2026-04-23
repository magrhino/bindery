package abs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vavallee/bindery/internal/db"
)

const SettingABSImportCheckpoint = "abs.import_checkpoint"

type enumerationClient interface {
	ListLibraryItems(ctx context.Context, libraryID string, page, limit int) (*LibraryItemsPage, error)
	GetLibraryItem(ctx context.Context, itemID string) (*LibraryItem, error)
}

type EnumerationStats struct {
	PagesScanned       int `json:"pagesScanned"`
	ItemsSeen          int `json:"itemsSeen"`
	ItemsNormalized    int `json:"itemsNormalized"`
	ItemsDetailFetched int `json:"itemsDetailFetched"`
}

type ImportCheckpoint struct {
	LibraryID  string    `json:"libraryId"`
	Page       int       `json:"page"`
	LastItemID string    `json:"lastItemId,omitempty"`
	PageSize   int       `json:"pageSize"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Enumerator struct {
	client        enumerationClient
	settings      *db.SettingsRepo
	pageSize      int
	checkpointKey string
	onCheckpoint  func(ImportCheckpoint)
}

func NewEnumerator(client enumerationClient, settings *db.SettingsRepo, pageSize int) *Enumerator {
	if pageSize <= 0 {
		pageSize = 50
	}
	return &Enumerator{
		client:        client,
		settings:      settings,
		pageSize:      pageSize,
		checkpointKey: SettingABSImportCheckpoint,
	}
}

func (e *Enumerator) WithCheckpointObserver(fn func(ImportCheckpoint)) *Enumerator {
	e.onCheckpoint = fn
	return e
}

func (e *Enumerator) Enumerate(ctx context.Context, libraryID string, fn func(context.Context, NormalizedLibraryItem) error) (EnumerationStats, error) {
	var stats EnumerationStats
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return stats, errors.New("library_id is required")
	}

	checkpoint, err := e.loadCheckpoint(ctx)
	if err != nil {
		return stats, err
	}
	page := 0
	skipUntilID := ""
	if checkpoint != nil && checkpoint.LibraryID == libraryID {
		page = checkpoint.Page
		skipUntilID = checkpoint.LastItemID
	}

	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		resp, err := e.client.ListLibraryItems(ctx, libraryID, page, e.pageSize)
		if err != nil {
			return stats, err
		}
		stats.PagesScanned++
		slog.Info("abs enumerate page",
			"libraryID", libraryID,
			"page", page,
			"limit", resp.Limit,
			"results", len(resp.Results),
			"total", resp.Total)

		if len(resp.Results) == 0 {
			break
		}

		startIndex := 0
		if skipUntilID != "" {
			found := false
			for idx, item := range resp.Results {
				if item.ID == skipUntilID {
					startIndex = idx + 1
					found = true
					break
				}
			}
			if !found {
				slog.Warn("abs checkpoint item not found on resume; reprocessing page",
					"libraryID", libraryID,
					"page", page,
					"lastItemID", skipUntilID)
			}
			skipUntilID = ""
		}

		for _, item := range resp.Results[startIndex:] {
			stats.ItemsSeen++
			reasons := item.DetailFetchReasons()
			detailFetched := len(reasons) > 0
			if detailFetched {
				slog.Info("abs enumerate detail fetch",
					"libraryID", libraryID,
					"itemID", item.ID,
					"reasons", reasons)
				detail, err := e.client.GetLibraryItem(ctx, item.ID)
				if err != nil {
					return stats, err
				}
				item = MergeLibraryItem(item, *detail)
				stats.ItemsDetailFetched++
			}

			normalized := NormalizeLibraryItem(item, detailFetched)
			if err := fn(ctx, normalized); err != nil {
				return stats, err
			}
			stats.ItemsNormalized++
			if err := e.saveCheckpoint(ctx, ImportCheckpoint{
				LibraryID:  libraryID,
				Page:       page,
				LastItemID: item.ID,
				PageSize:   e.pageSize,
				UpdatedAt:  time.Now().UTC(),
			}); err != nil {
				return stats, err
			}
		}

		page++
		if err := e.saveCheckpoint(ctx, ImportCheckpoint{
			LibraryID: libraryID,
			Page:      page,
			PageSize:  e.pageSize,
			UpdatedAt: time.Now().UTC(),
		}); err != nil {
			return stats, err
		}

		limit := resp.Limit
		if limit <= 0 {
			limit = e.pageSize
		}
		if limit <= 0 || len(resp.Results) < limit || page*limit >= resp.Total {
			break
		}
	}

	if err := e.clearCheckpoint(ctx); err != nil {
		return stats, err
	}
	slog.Info("abs enumerate complete",
		"libraryID", libraryID,
		"pagesScanned", stats.PagesScanned,
		"itemsSeen", stats.ItemsSeen,
		"itemsNormalized", stats.ItemsNormalized,
		"itemsDetailFetched", stats.ItemsDetailFetched)
	return stats, nil
}

func (e *Enumerator) loadCheckpoint(ctx context.Context) (*ImportCheckpoint, error) {
	if e.settings == nil {
		return nil, nil
	}
	setting, err := e.settings.Get(ctx, e.checkpointKey)
	if err != nil {
		return nil, err
	}
	if setting == nil || strings.TrimSpace(setting.Value) == "" {
		return nil, nil
	}
	var checkpoint ImportCheckpoint
	if err := json.Unmarshal([]byte(setting.Value), &checkpoint); err != nil {
		return nil, fmt.Errorf("decode abs checkpoint: %w", err)
	}
	return &checkpoint, nil
}

func (e *Enumerator) saveCheckpoint(ctx context.Context, checkpoint ImportCheckpoint) error {
	if e.settings == nil {
		if e.onCheckpoint != nil {
			e.onCheckpoint(checkpoint)
		}
		return nil
	}
	payload, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("encode abs checkpoint: %w", err)
	}
	if err := e.settings.Set(ctx, e.checkpointKey, string(payload)); err != nil {
		return err
	}
	if e.onCheckpoint != nil {
		e.onCheckpoint(checkpoint)
	}
	return nil
}

func (e *Enumerator) clearCheckpoint(ctx context.Context) error {
	if e.settings == nil {
		return nil
	}
	return e.settings.Delete(ctx, e.checkpointKey)
}
