-- +migrate Up

-- Users table holds local credentials. username unique, password_hash is
-- an argon2id PHC-format string produced by internal/auth.
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Auth settings live in the existing key-value settings table. See
-- cmd/bindery/main.go bootstrapAuth for the seed values applied on first run.

-- +migrate Down

DROP TABLE IF EXISTS users;
