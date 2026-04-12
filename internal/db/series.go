package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

type SeriesRepo struct {
	db *sql.DB
}

func NewSeriesRepo(db *sql.DB) *SeriesRepo {
	return &SeriesRepo{db: db}
}

func (r *SeriesRepo) List(ctx context.Context) ([]models.Series, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, foreign_id, title, description, created_at FROM series ORDER BY title")
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	defer rows.Close()

	var series []models.Series
	for rows.Next() {
		var s models.Series
		if err := rows.Scan(&s.ID, &s.ForeignID, &s.Title, &s.Description, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan series: %w", err)
		}
		series = append(series, s)
	}
	return series, rows.Err()
}

func (r *SeriesRepo) GetByID(ctx context.Context, id int64) (*models.Series, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, foreign_id, title, description, created_at FROM series WHERE id=?", id)

	var s models.Series
	err := row.Scan(&s.ID, &s.ForeignID, &s.Title, &s.Description, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get series %d: %w", id, err)
	}

	// Fetch series books with minimal book data
	bookRows, err := r.db.QueryContext(ctx, `
		SELECT sb.series_id, sb.book_id, sb.position_in_series, sb.primary_series,
		       b.id, b.foreign_id, b.author_id, b.title, b.sort_title, b.status,
		       b.monitored, b.image_url, b.created_at, b.updated_at
		FROM series_books sb
		JOIN books b ON b.id = sb.book_id
		WHERE sb.series_id = ?
		ORDER BY sb.position_in_series`, id)
	if err != nil {
		return &s, nil
	}
	defer bookRows.Close()

	for bookRows.Next() {
		var sb models.SeriesBook
		var b models.Book
		var monitored, primarySeries int
		err := bookRows.Scan(
			&sb.SeriesID, &sb.BookID, &sb.PositionInSeries, &primarySeries,
			&b.ID, &b.ForeignID, &b.AuthorID, &b.Title, &b.SortTitle, &b.Status,
			&monitored, &b.ImageURL, &b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan series book: %w", err)
		}
		b.Monitored = monitored == 1
		sb.PrimarySeries = primarySeries == 1
		sb.Book = &b
		s.Books = append(s.Books, sb)
	}

	return &s, bookRows.Err()
}

func (r *SeriesRepo) Create(ctx context.Context, s *models.Series) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO series (foreign_id, title, description, created_at) VALUES (?, ?, ?, ?)",
		s.ForeignID, s.Title, s.Description, now)
	if err != nil {
		return fmt.Errorf("create series: %w", err)
	}
	id, _ := result.LastInsertId()
	s.ID = id
	s.CreatedAt = now
	return nil
}

func (r *SeriesRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM series WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("delete series %d: %w", id, err)
	}
	return nil
}
