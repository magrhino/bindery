-- +migrate Up

-- Blocklist: prevent re-grabbing releases that failed
CREATE TABLE blocklist (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id      INTEGER REFERENCES books(id) ON DELETE SET NULL,
    guid         TEXT    NOT NULL,
    title        TEXT    NOT NULL,
    indexer_id   INTEGER REFERENCES indexers(id) ON DELETE SET NULL,
    reason       TEXT    NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_blocklist_guid ON blocklist(guid);
CREATE INDEX idx_blocklist_book_id ON blocklist(book_id);

-- Notifications (webhooks etc)
CREATE TABLE notifications (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT    NOT NULL,
    type          TEXT    NOT NULL DEFAULT 'webhook',
    url           TEXT    NOT NULL DEFAULT '',
    method        TEXT    NOT NULL DEFAULT 'POST',
    headers       TEXT    NOT NULL DEFAULT '{}',
    on_grab       INTEGER NOT NULL DEFAULT 1,
    on_import     INTEGER NOT NULL DEFAULT 1,
    on_upgrade    INTEGER NOT NULL DEFAULT 0,
    on_failure    INTEGER NOT NULL DEFAULT 1,
    on_health     INTEGER NOT NULL DEFAULT 0,
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Tags for scoping indexers/profiles/notifications to authors
CREATE TABLE tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT    NOT NULL UNIQUE
);

CREATE TABLE author_tags (
    author_id INTEGER NOT NULL REFERENCES authors(id) ON DELETE CASCADE,
    tag_id    INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (author_id, tag_id)
);

-- Metadata profiles for filtering which books get added
CREATE TABLE metadata_profiles (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    name                   TEXT    NOT NULL,
    min_popularity         INTEGER NOT NULL DEFAULT 0,
    min_pages              INTEGER NOT NULL DEFAULT 0,
    skip_missing_date      INTEGER NOT NULL DEFAULT 0,
    skip_missing_isbn      INTEGER NOT NULL DEFAULT 0,
    skip_part_books        INTEGER NOT NULL DEFAULT 0,
    allowed_languages      TEXT    NOT NULL DEFAULT 'eng',
    created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed a default metadata profile
INSERT INTO metadata_profiles (name, allowed_languages) VALUES ('Standard', 'eng');

-- Import lists
CREATE TABLE import_lists (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    name               TEXT    NOT NULL,
    type               TEXT    NOT NULL DEFAULT 'csv',
    url                TEXT    NOT NULL DEFAULT '',
    api_key            TEXT    NOT NULL DEFAULT '',
    root_folder_id     INTEGER,
    quality_profile_id INTEGER,
    monitor_new        INTEGER NOT NULL DEFAULT 1,
    auto_add           INTEGER NOT NULL DEFAULT 1,
    enabled            INTEGER NOT NULL DEFAULT 1,
    last_sync_at       DATETIME,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Import list exclusions
CREATE TABLE import_list_exclusions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    foreign_id  TEXT    NOT NULL UNIQUE,
    title       TEXT    NOT NULL DEFAULT '',
    author_name TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Delay profiles
CREATE TABLE delay_profiles (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    usenet_delay         INTEGER NOT NULL DEFAULT 0,
    torrent_delay        INTEGER NOT NULL DEFAULT 0,
    preferred_protocol   TEXT    NOT NULL DEFAULT 'usenet',
    enable_usenet        INTEGER NOT NULL DEFAULT 1,
    enable_torrent       INTEGER NOT NULL DEFAULT 1,
    "order"              INTEGER NOT NULL DEFAULT 0,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed default delay profile (no delay)
INSERT INTO delay_profiles (usenet_delay, torrent_delay, preferred_protocol) VALUES (0, 0, 'usenet');

-- Custom formats
CREATE TABLE custom_formats (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    conditions  TEXT    NOT NULL DEFAULT '[]',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Add metadata_profile_id to authors
ALTER TABLE authors ADD COLUMN metadata_profile_id INTEGER;

-- Add tags column to indexers and download_clients for tag scoping
ALTER TABLE indexers ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
ALTER TABLE download_clients ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';

-- +migrate Down

DROP TABLE IF EXISTS custom_formats;
DROP TABLE IF EXISTS delay_profiles;
DROP TABLE IF EXISTS import_list_exclusions;
DROP TABLE IF EXISTS import_lists;
DROP TABLE IF EXISTS metadata_profiles;
DROP TABLE IF EXISTS author_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS blocklist;
