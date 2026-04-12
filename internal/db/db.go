// Package db contains the SQLite connection bootstrap, embedded migrations,
// and per-resource repository types backing the rest of Bindery.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open creates a new database connection and runs migrations.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := setPragmas(db); err != nil {
		db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	slog.Info("database ready", "path", dbPath)
	return db, nil
}

// OpenMemory creates an in-memory database for testing.
func OpenMemory() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("open memory database: %w", err)
	}

	if err := setPragmas(db); err != nil {
		db.Close()
		return nil, err
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func setPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("set %s: %w", p, err)
		}
	}
	return nil
}

func migrate(database *sql.DB) error {
	// Create a migrations tracking table
	_, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Read all migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Sort by filename (001_, 002_, etc.)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for version, entry := range entries {
		v := version + 1 // 1-indexed

		// Check if already applied
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", v).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", v, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		sqlStr := string(content)
		if idx := strings.Index(sqlStr, "-- +migrate Down"); idx >= 0 {
			sqlStr = sqlStr[:idx]
		}
		sqlStr = strings.Replace(sqlStr, "-- +migrate Up", "", 1)

		// Execute each statement individually
		for _, stmt := range strings.Split(sqlStr, ";") {
			// Strip comment-only lines
			lines := strings.Split(stmt, "\n")
			var cleaned []string
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "--") {
					continue
				}
				cleaned = append(cleaned, line)
			}
			stmt = strings.TrimSpace(strings.Join(cleaned, "\n"))
			if stmt == "" {
				continue
			}
			if _, err := database.Exec(stmt); err != nil {
				return fmt.Errorf("migration %d statement: %w\nSQL: %s", v, err, stmt)
			}
		}

		if _, err := database.Exec("INSERT INTO schema_migrations (version) VALUES (?)", v); err != nil {
			return fmt.Errorf("record migration %d: %w", v, err)
		}
		slog.Info("applied migration", "version", v, "file", entry.Name())
	}

	return nil
}
