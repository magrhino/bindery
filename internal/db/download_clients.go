package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vavallee/bindery/internal/models"
)

type DownloadClientRepo struct {
	db *sql.DB
}

func NewDownloadClientRepo(db *sql.DB) *DownloadClientRepo {
	return &DownloadClientRepo{db: db}
}

// populateVirtualCredentials maps the storage columns to the virtual Username/Password
// fields for qBittorrent clients (which store credentials in url_base and api_key).
func populateVirtualCredentials(c *models.DownloadClient) {
	if c.Type == "qbittorrent" {
		c.Username = c.URLBase
		c.Password = c.APIKey
	}
}

// applyVirtualCredentials maps the virtual Username/Password fields back to the
// storage columns for qBittorrent clients before writing to the database.
func applyVirtualCredentials(c *models.DownloadClient) {
	if c.Type == "qbittorrent" {
		c.URLBase = c.Username
		c.APIKey = c.Password
	}
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
		populateVirtualCredentials(&c)
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
	populateVirtualCredentials(&c)
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
	populateVirtualCredentials(&c)
	return &c, nil
}

// GetFirstEnabledByProtocol returns the highest-priority enabled client that
// matches the given protocol ("usenet" → sabnzbd, "torrent" → qbittorrent).
// Returns (nil, nil) if no matching client is configured.
func (r *DownloadClientRepo) GetFirstEnabledByProtocol(ctx context.Context, protocol string) (*models.DownloadClient, error) {
	clientType := "sabnzbd"
	if protocol == "torrent" {
		clientType = "qbittorrent"
	}

	var c models.DownloadClient
	var enabled, useSSL int
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, type, host, port, api_key, use_ssl, url_base, category, priority, enabled, created_at, updated_at
		FROM download_clients WHERE enabled=1 AND type=? ORDER BY priority LIMIT 1`, clientType).
		Scan(&c.ID, &c.Name, &c.Type, &c.Host, &c.Port, &c.APIKey,
			&useSSL, &c.URLBase, &c.Category, &c.Priority, &enabled, &c.CreatedAt, &c.UpdatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	c.Enabled = enabled == 1
	c.UseSSL = useSSL == 1
	populateVirtualCredentials(&c)
	return &c, nil
}

// GetEnabledByProtocol returns all enabled clients matching the given protocol,
// ordered by priority. Used when multiple clients of the same type exist and
// the caller needs to pick the best one by category.
func (r *DownloadClientRepo) GetEnabledByProtocol(ctx context.Context, protocol string) ([]models.DownloadClient, error) {
	clientType := "sabnzbd"
	if protocol == "torrent" {
		clientType = "qbittorrent"
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, type, host, port, api_key, use_ssl, url_base, category, priority, enabled, created_at, updated_at
		FROM download_clients WHERE enabled=1 AND type=? ORDER BY priority`, clientType)
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
		populateVirtualCredentials(&c)
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

func (r *DownloadClientRepo) Create(ctx context.Context, c *models.DownloadClient) error {
	applyVirtualCredentials(c)
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
	applyVirtualCredentials(c)
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

// PickClientForMediaType selects the best client from a list for the given
// media type. Audiobooks prefer a client whose category contains "audio";
// other types prefer one without. Falls back to the first client.
func PickClientForMediaType(clients []models.DownloadClient, mediaType string) *models.DownloadClient {
	if len(clients) == 0 {
		return nil
	}
	if len(clients) == 1 {
		return &clients[0]
	}
	for i := range clients {
		cat := strings.ToLower(clients[i].Category)
		if mediaType == "audiobook" && strings.Contains(cat, "audio") {
			return &clients[i]
		}
		if mediaType != "audiobook" && !strings.Contains(cat, "audio") {
			return &clients[i]
		}
	}
	return &clients[0]
}
