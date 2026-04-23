package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vavallee/bindery/internal/models"
	"github.com/vavallee/bindery/internal/textutil"
)

type ABSImportRunRepo struct {
	db *sql.DB
}

func NewABSImportRunRepo(db *sql.DB) *ABSImportRunRepo {
	return &ABSImportRunRepo{db: db}
}

func (r *ABSImportRunRepo) Create(ctx context.Context, run *models.ABSImportRun) error {
	now := time.Now().UTC()
	if run.SourceID == "" {
		run.SourceID = "default"
	}
	sourceConfigJSON := run.SourceConfigJSON
	if sourceConfigJSON == "" {
		sourceConfigJSON = "{}"
	}
	checkpointJSON := run.CheckpointJSON
	if checkpointJSON == "" {
		checkpointJSON = "{}"
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO abs_import_runs (source_id, source_label, base_url, library_id, status, dry_run, source_config_json, checkpoint_json, summary_json, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.SourceID, run.SourceLabel, run.BaseURL, run.LibraryID, run.Status, boolToInt(run.DryRun), sourceConfigJSON, checkpointJSON, run.SummaryJSON, now)
	if err != nil {
		return fmt.Errorf("create abs import run: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("abs import run last insert id: %w", err)
	}
	run.ID = id
	run.StartedAt = now
	run.SourceConfigJSON = sourceConfigJSON
	run.CheckpointJSON = checkpointJSON
	return nil
}

func (r *ABSImportRunRepo) Finish(ctx context.Context, id int64, status string, summary any) error {
	if id == 0 {
		return nil
	}
	now := time.Now().UTC()
	payload := "{}"
	if summary != nil {
		data, err := json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("encode abs import summary: %w", err)
		}
		payload = string(data)
	}
	query := `
		UPDATE abs_import_runs
		SET status = ?, summary_json = ?, finished_at = ?
		WHERE id = ?`
	args := []any{status, payload, now, id}
	if status == "completed" || status == "rolled_back" {
		query = `
		UPDATE abs_import_runs
		SET status = ?, summary_json = ?, finished_at = ?, checkpoint_json = '{}'
		WHERE id = ?`
		args = []any{status, payload, now, id}
	}
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("finish abs import run %d: %w", id, err)
	}
	return nil
}

func (r *ABSImportRunRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	if id == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE abs_import_runs
		SET status = ?
		WHERE id = ?`,
		status, id)
	if err != nil {
		return fmt.Errorf("update abs import run status %d: %w", id, err)
	}
	return nil
}

func (r *ABSImportRunRepo) UpdateCheckpoint(ctx context.Context, id int64, checkpoint any) error {
	if id == 0 {
		return nil
	}
	payload := "{}"
	if checkpoint != nil {
		data, err := json.Marshal(checkpoint)
		if err != nil {
			return fmt.Errorf("encode abs checkpoint: %w", err)
		}
		payload = string(data)
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE abs_import_runs
		SET checkpoint_json = ?
		WHERE id = ?`,
		payload, id)
	if err != nil {
		return fmt.Errorf("update abs import run checkpoint %d: %w", id, err)
	}
	return nil
}

func (r *ABSImportRunRepo) GetByID(ctx context.Context, id int64) (*models.ABSImportRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, source_id, source_label, base_url, library_id, status, dry_run, source_config_json, checkpoint_json, summary_json, started_at, finished_at
		FROM abs_import_runs
		WHERE id = ?`, id)
	var run models.ABSImportRun
	var dryRun int
	if err := row.Scan(&run.ID, &run.SourceID, &run.SourceLabel, &run.BaseURL, &run.LibraryID, &run.Status, &dryRun, &run.SourceConfigJSON, &run.CheckpointJSON, &run.SummaryJSON, &run.StartedAt, &run.FinishedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get abs import run %d: %w", id, err)
	}
	run.DryRun = dryRun == 1
	return &run, nil
}

func (r *ABSImportRunRepo) ListRecent(ctx context.Context, limit int) ([]models.ABSImportRun, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_id, source_label, base_url, library_id, status, dry_run, source_config_json, checkpoint_json, summary_json, started_at, finished_at
		FROM abs_import_runs
		ORDER BY started_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent abs import runs: %w", err)
	}
	defer rows.Close()

	var out []models.ABSImportRun
	for rows.Next() {
		var run models.ABSImportRun
		var dryRun int
		if err := rows.Scan(&run.ID, &run.SourceID, &run.SourceLabel, &run.BaseURL, &run.LibraryID, &run.Status, &dryRun, &run.SourceConfigJSON, &run.CheckpointJSON, &run.SummaryJSON, &run.StartedAt, &run.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan abs import run: %w", err)
		}
		run.DryRun = dryRun == 1
		out = append(out, run)
	}
	return out, rows.Err()
}

type ABSProvenanceRepo struct {
	db *sql.DB
}

func NewABSProvenanceRepo(db *sql.DB) *ABSProvenanceRepo {
	return &ABSProvenanceRepo{db: db}
}

func (r *ABSProvenanceRepo) GetByExternal(ctx context.Context, sourceID, libraryID, entityType, externalID string) (*models.ABSProvenance, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, source_id, library_id, entity_type, external_id, local_id, item_id, format, file_ids_json, import_run_id, created_at, updated_at
		FROM abs_provenance
		WHERE source_id = ? AND library_id = ? AND entity_type = ? AND external_id = ?`,
		sourceID, libraryID, entityType, externalID)
	return scanABSProvenance(row, "get abs provenance")
}

func (r *ABSProvenanceRepo) ListByLocal(ctx context.Context, entityType string, localID int64) ([]models.ABSProvenance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_id, library_id, entity_type, external_id, local_id, item_id, format, file_ids_json, import_run_id, created_at, updated_at
		FROM abs_provenance
		WHERE entity_type = ? AND local_id = ?
		ORDER BY id`, entityType, localID)
	if err != nil {
		return nil, fmt.Errorf("list abs provenance for %s %d: %w", entityType, localID, err)
	}
	defer rows.Close()

	var out []models.ABSProvenance
	for rows.Next() {
		item, err := scanABSProvenanceRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *ABSProvenanceRepo) Upsert(ctx context.Context, p *models.ABSProvenance) error {
	now := time.Now().UTC()
	if p.SourceID == "" {
		p.SourceID = "default"
	}
	fileIDsJSON, err := json.Marshal(p.FileIDs)
	if err != nil {
		return fmt.Errorf("encode abs provenance file ids: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO abs_provenance (source_id, library_id, entity_type, external_id, local_id, item_id, format, file_ids_json, import_run_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, library_id, entity_type, external_id) DO UPDATE SET
			local_id      = excluded.local_id,
			item_id       = excluded.item_id,
			format        = excluded.format,
			file_ids_json = excluded.file_ids_json,
			import_run_id = excluded.import_run_id,
			updated_at    = excluded.updated_at`,
		p.SourceID, p.LibraryID, p.EntityType, p.ExternalID, p.LocalID, p.ItemID, p.Format, string(fileIDsJSON), p.ImportRunID, now, now)
	if err != nil {
		return fmt.Errorf("upsert abs provenance %s/%s/%s: %w", p.EntityType, p.LibraryID, p.ExternalID, err)
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at
		FROM abs_provenance
		WHERE source_id = ? AND library_id = ? AND entity_type = ? AND external_id = ?`,
		p.SourceID, p.LibraryID, p.EntityType, p.ExternalID)
	if err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return fmt.Errorf("reload abs provenance %s/%s/%s: %w", p.EntityType, p.LibraryID, p.ExternalID, err)
	}
	return nil
}

func (r *ABSProvenanceRepo) DeleteByExternal(ctx context.Context, sourceID, libraryID, entityType, externalID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM abs_provenance
		WHERE source_id = ? AND library_id = ? AND entity_type = ? AND external_id = ?`,
		sourceID, libraryID, entityType, externalID)
	if err != nil {
		return fmt.Errorf("delete abs provenance %s/%s/%s: %w", entityType, libraryID, externalID, err)
	}
	return nil
}

type ABSImportRunEntityRepo struct {
	db *sql.DB
}

func NewABSImportRunEntityRepo(db *sql.DB) *ABSImportRunEntityRepo {
	return &ABSImportRunEntityRepo{db: db}
}

func (r *ABSImportRunEntityRepo) Record(ctx context.Context, entity *models.ABSImportRunEntity) error {
	if entity == nil || entity.RunID == 0 {
		return nil
	}
	sourceID := entity.SourceID
	if sourceID == "" {
		sourceID = "default"
	}
	metadataJSON := entity.MetadataJSON
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO abs_import_run_entities (run_id, source_id, library_id, item_id, entity_type, external_id, local_id, outcome, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, entity_type, external_id, local_id) DO UPDATE SET
			item_id = excluded.item_id,
			outcome = excluded.outcome,
			metadata_json = excluded.metadata_json`,
		entity.RunID, sourceID, entity.LibraryID, entity.ItemID, entity.EntityType, entity.ExternalID, entity.LocalID, entity.Outcome, metadataJSON)
	if err != nil {
		return fmt.Errorf("record abs import run entity %d/%s/%s: %w", entity.RunID, entity.EntityType, entity.ExternalID, err)
	}
	if entity.ID == 0 {
		if id, err := result.LastInsertId(); err == nil && id > 0 {
			entity.ID = id
		}
	}
	entity.SourceID = sourceID
	entity.MetadataJSON = metadataJSON
	return nil
}

func (r *ABSImportRunEntityRepo) ListByRun(ctx context.Context, runID int64) ([]models.ABSImportRunEntity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, source_id, library_id, item_id, entity_type, external_id, local_id, outcome, metadata_json, created_at
		FROM abs_import_run_entities
		WHERE run_id = ?
		ORDER BY id`, runID)
	if err != nil {
		return nil, fmt.Errorf("list abs import run entities %d: %w", runID, err)
	}
	defer rows.Close()

	var out []models.ABSImportRunEntity
	for rows.Next() {
		var entity models.ABSImportRunEntity
		if err := rows.Scan(&entity.ID, &entity.RunID, &entity.SourceID, &entity.LibraryID, &entity.ItemID, &entity.EntityType, &entity.ExternalID, &entity.LocalID, &entity.Outcome, &entity.MetadataJSON, &entity.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan abs import run entity: %w", err)
		}
		out = append(out, entity)
	}
	return out, rows.Err()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type ABSReviewItemRepo struct {
	db *sql.DB
}

func NewABSReviewItemRepo(db *sql.DB) *ABSReviewItemRepo {
	return &ABSReviewItemRepo{db: db}
}

func (r *ABSReviewItemRepo) UpsertPending(ctx context.Context, item *models.ABSReviewItem) error {
	if item == nil {
		return nil
	}
	now := time.Now().UTC()
	if strings.TrimSpace(item.Status) == "" {
		item.Status = "pending"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO abs_review_queue (source_id, library_id, item_id, title, primary_author, asin, media_type, review_reason, payload_json, latest_run_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(source_id, library_id, item_id) DO UPDATE SET
			title = excluded.title,
			primary_author = excluded.primary_author,
			asin = excluded.asin,
			media_type = excluded.media_type,
			review_reason = excluded.review_reason,
			payload_json = excluded.payload_json,
			latest_run_id = excluded.latest_run_id,
			status = 'pending',
			updated_at = excluded.updated_at`,
		item.SourceID, item.LibraryID, item.ItemID, item.Title, item.PrimaryAuthor, item.ASIN, item.MediaType, item.ReviewReason, item.PayloadJSON, item.LatestRunID, now, now)
	if err != nil {
		return fmt.Errorf("upsert abs review item %s/%s/%s: %w", item.SourceID, item.LibraryID, item.ItemID, err)
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at
		FROM abs_review_queue
		WHERE source_id = ? AND library_id = ? AND item_id = ?`,
		item.SourceID, item.LibraryID, item.ItemID)
	if err := row.Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return fmt.Errorf("reload abs review item %s/%s/%s: %w", item.SourceID, item.LibraryID, item.ItemID, err)
	}
	item.Status = "pending"
	return nil
}

func (r *ABSReviewItemRepo) ListByStatus(ctx context.Context, status string) ([]models.ABSReviewItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_id, library_id, item_id, title, primary_author, asin, media_type, review_reason, payload_json,
		       resolved_author_foreign_id, resolved_author_name, resolved_book_foreign_id, resolved_book_title, edited_title,
		       latest_run_id, status, created_at, updated_at
		FROM abs_review_queue
		WHERE status = ?
		ORDER BY updated_at DESC, id DESC`, strings.TrimSpace(status))
	if err != nil {
		return nil, fmt.Errorf("list abs review items: %w", err)
	}
	defer rows.Close()

	var out []models.ABSReviewItem
	for rows.Next() {
		var item models.ABSReviewItem
		if err := scanABSReviewItemRows(rows, &item); err != nil {
			return nil, fmt.Errorf("scan abs review item: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *ABSReviewItemRepo) GetByID(ctx context.Context, id int64) (*models.ABSReviewItem, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, source_id, library_id, item_id, title, primary_author, asin, media_type, review_reason, payload_json,
		       resolved_author_foreign_id, resolved_author_name, resolved_book_foreign_id, resolved_book_title, edited_title,
		       latest_run_id, status, created_at, updated_at
		FROM abs_review_queue
		WHERE id = ?`, id)
	var item models.ABSReviewItem
	if err := scanABSReviewItemRow(row, &item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get abs review item %d: %w", id, err)
	}
	return &item, nil
}

func (r *ABSReviewItemRepo) ResolveAuthorForPrimary(ctx context.Context, sourceID, libraryID, primaryAuthor, foreignID, name string) (int, error) {
	sourceID = strings.TrimSpace(sourceID)
	libraryID = strings.TrimSpace(libraryID)
	foreignID = strings.TrimSpace(foreignID)
	name = strings.TrimSpace(name)
	if foreignID == "" || name == "" {
		return 0, errors.New("foreignAuthorId and authorName required")
	}
	key := textutil.NormalizeAuthorName(primaryAuthor)
	if key == "" {
		return 0, errors.New("primary author is required")
	}
	items, err := r.ListByStatus(ctx, "pending")
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	updated := 0
	for _, item := range items {
		if strings.TrimSpace(item.SourceID) != sourceID || strings.TrimSpace(item.LibraryID) != libraryID {
			continue
		}
		if textutil.NormalizeAuthorName(item.PrimaryAuthor) != key {
			continue
		}
		if _, err := r.db.ExecContext(ctx, `
			UPDATE abs_review_queue
			SET resolved_author_foreign_id = ?, resolved_author_name = ?, updated_at = ?
			WHERE id = ?`, foreignID, name, now, item.ID); err != nil {
			return updated, fmt.Errorf("resolve abs review author %d: %w", item.ID, err)
		}
		updated++
	}
	return updated, nil
}

func (r *ABSReviewItemRepo) ResolveBook(ctx context.Context, id int64, foreignID, title, editedTitle string) error {
	foreignID = strings.TrimSpace(foreignID)
	title = strings.TrimSpace(title)
	editedTitle = strings.TrimSpace(editedTitle)
	if foreignID == "" || title == "" {
		return errors.New("foreignBookId and title required")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE abs_review_queue
		SET resolved_book_foreign_id = ?, resolved_book_title = ?, edited_title = ?, updated_at = ?
		WHERE id = ?`, foreignID, title, editedTitle, now, id)
	if err != nil {
		return fmt.Errorf("resolve abs review book %d: %w", id, err)
	}
	return nil
}

func (r *ABSReviewItemRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE abs_review_queue
		SET status = ?, updated_at = ?
		WHERE id = ?`, strings.TrimSpace(status), now, id)
	if err != nil {
		return fmt.Errorf("update abs review item %d: %w", id, err)
	}
	return nil
}

type absReviewItemRowScanner interface {
	Scan(dest ...any) error
}

func scanABSReviewItemRow(row absReviewItemRowScanner, item *models.ABSReviewItem) error {
	return row.Scan(
		&item.ID,
		&item.SourceID,
		&item.LibraryID,
		&item.ItemID,
		&item.Title,
		&item.PrimaryAuthor,
		&item.ASIN,
		&item.MediaType,
		&item.ReviewReason,
		&item.PayloadJSON,
		&item.ResolvedAuthorForeignID,
		&item.ResolvedAuthorName,
		&item.ResolvedBookForeignID,
		&item.ResolvedBookTitle,
		&item.EditedTitle,
		&item.LatestRunID,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
}

func scanABSReviewItemRows(rows *sql.Rows, item *models.ABSReviewItem) error {
	return scanABSReviewItemRow(rows, item)
}

type absProvenanceScanner interface {
	Scan(dest ...any) error
}

func scanABSProvenance(scanner absProvenanceScanner, context string) (*models.ABSProvenance, error) {
	var (
		item        models.ABSProvenance
		fileIDsJSON string
	)
	if err := scanner.Scan(&item.ID, &item.SourceID, &item.LibraryID, &item.EntityType, &item.ExternalID, &item.LocalID, &item.ItemID, &item.Format, &fileIDsJSON, &item.ImportRunID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", context, err)
	}
	if fileIDsJSON != "" {
		if err := json.Unmarshal([]byte(fileIDsJSON), &item.FileIDs); err != nil {
			return nil, fmt.Errorf("%s decode file ids: %w", context, err)
		}
	}
	return &item, nil
}

func scanABSProvenanceRows(rows *sql.Rows) (models.ABSProvenance, error) {
	item, err := scanABSProvenance(rows, "scan abs provenance")
	if err != nil {
		return models.ABSProvenance{}, err
	}
	if item == nil {
		return models.ABSProvenance{}, sql.ErrNoRows
	}
	return *item, nil
}
