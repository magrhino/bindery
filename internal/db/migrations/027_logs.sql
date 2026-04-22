-- +migrate Up

CREATE TABLE logs (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        DATETIME NOT NULL,
    level     TEXT     NOT NULL,
    component TEXT     NOT NULL DEFAULT '',
    message   TEXT     NOT NULL,
    fields    TEXT     NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_logs_ts        ON logs (ts);
CREATE INDEX idx_logs_level     ON logs (level);
CREATE INDEX idx_logs_component ON logs (component);

-- +migrate Down

DROP INDEX IF EXISTS idx_logs_component;
DROP INDEX IF EXISTS idx_logs_level;
DROP INDEX IF EXISTS idx_logs_ts;
DROP TABLE IF EXISTS logs;
