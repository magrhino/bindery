-- +migrate Up
CREATE TABLE recommendations (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL DEFAULT 1,
    foreign_id    TEXT NOT NULL,
    rec_type      TEXT NOT NULL,
    title         TEXT NOT NULL,
    author_name   TEXT NOT NULL DEFAULT '',
    author_id     INTEGER,
    image_url     TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    genres        TEXT NOT NULL DEFAULT '[]',
    rating        REAL NOT NULL DEFAULT 0,
    ratings_count INTEGER NOT NULL DEFAULT 0,
    release_date  DATE,
    language      TEXT NOT NULL DEFAULT '',
    media_type    TEXT NOT NULL DEFAULT 'ebook',
    score         REAL NOT NULL DEFAULT 0,
    reason        TEXT NOT NULL DEFAULT '',
    series_id     INTEGER,
    series_pos    TEXT NOT NULL DEFAULT '',
    dismissed     INTEGER NOT NULL DEFAULT 0,
    batch_id      TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_recommendations_user_score ON recommendations(user_id, score DESC);
CREATE INDEX idx_recommendations_user_dismissed ON recommendations(user_id, dismissed);

CREATE TABLE recommendation_dismissals (
    user_id    INTEGER NOT NULL DEFAULT 1,
    foreign_id TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, foreign_id)
);

CREATE TABLE recommendation_author_exclusions (
    user_id     INTEGER NOT NULL DEFAULT 1,
    author_name TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, author_name)
);
