package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

type DownloadClientRepo struct {
	db *sql.DB
}

func NewDownloadClientRepo(db *sql.DB) *DownloadClientRepo {
	return &DownloadClientRepo{db: db}
}

func (r *DownloadClientRepo) List(ctx context.Context) ([]models.DownloadClient, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, type, host, port, api_key, use_ssl, url_base, category, priority, enabled, created_at, updated_at
		FROM download_clients ORDER BY priority`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []models.DownloadClient
	for rows.Next() {
		var c models.DownloadClient
		var enabled, useSSL int
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Host, &c.Port, &c.APIKey,
			&useSSL, &c.URLBase, &c.Category, &c.Priority, &enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = enabled == 1
		c.UseSSL = useSSL == 1
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

func (r *DownloadClientRepo) GetByID(ctx context.Context, id int64) (*models.DownloadClient, error) {
	var c models.DownloadClient
	var enabled, useSSL int
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, type, host, port, api_key, use_ssl, url_base, category, priority, enabled, created_at, updated_at
		FROM download_clients WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Type, &c.Host, &c.Port, &c.APIKey,
			&useSSL, &c.URLBase, &c.Category, &c.Priority, &enabled, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1
	c.UseSSL = useSSL == 1
	return &c, nil
}

func (r *DownloadClientRepo) GetFirstEnabled(ctx context.Context) (*models.DownloadClient, error) {
	var c models.DownloadClient
	var enabled, useSSL int
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, type, host, port, api_key, use_ssl, url_base, category, priority, enabled, created_at, updated_at
		FROM download_clients WHERE enabled=1 ORDER BY priority LIMIT 1`).
		Scan(&c.ID, &c.Name, &c.Type, &c.Host, &c.Port, &c.APIKey,
			&useSSL, &c.URLBase, &c.Category, &c.Priority, &enabled, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1
	c.UseSSL = useSSL == 1
	return &c, nil
}

func (r *DownloadClientRepo) Create(ctx context.Context, c *models.DownloadClient) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO download_clients (name, type, host, port, api_key, use_ssl, url_base, category, priority, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Type, c.Host, c.Port, c.APIKey, c.UseSSL, c.URLBase, c.Category, c.Priority, c.Enabled, now, now)
	if err != nil {
		return fmt.Errorf("create download client: %w", err)
	}
	id, _ := result.LastInsertId()
	c.ID = id
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

func (r *DownloadClientRepo) Update(ctx context.Context, c *models.DownloadClient) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE download_clients SET name=?, type=?, host=?, port=?, api_key=?, use_ssl=?,
		                            url_base=?, category=?, priority=?, enabled=?, updated_at=?
		WHERE id=?`,
		c.Name, c.Type, c.Host, c.Port, c.APIKey, c.UseSSL, c.URLBase, c.Category, c.Priority, c.Enabled, now, c.ID)
	return err
}

func (r *DownloadClientRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM download_clients WHERE id=?", id)
	return err
}
