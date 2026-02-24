package store

const schema = `
CREATE TABLE IF NOT EXISTS items (
    id           TEXT PRIMARY KEY,
    source       TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    title        TEXT NOT NULL,
    url          TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    author       TEXT NOT NULL DEFAULT '',
    score        INTEGER NOT NULL DEFAULT 0,
    comments     INTEGER NOT NULL DEFAULT 0,
    tags         TEXT NOT NULL DEFAULT '[]',
    published_at DATETIME NOT NULL,
    collected_at DATETIME NOT NULL,
    extra        TEXT NOT NULL DEFAULT '{}',
    UNIQUE(source, external_id)
);

CREATE INDEX IF NOT EXISTS idx_items_source ON items(source);
CREATE INDEX IF NOT EXISTS idx_items_collected_at ON items(collected_at);
CREATE INDEX IF NOT EXISTS idx_items_published_at ON items(published_at);
CREATE INDEX IF NOT EXISTS idx_items_score ON items(score);

CREATE TABLE IF NOT EXISTS score_snapshots (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id    TEXT NOT NULL REFERENCES items(id),
    score      INTEGER NOT NULL,
    comments   INTEGER NOT NULL,
    checked_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_item ON score_snapshots(item_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_checked ON score_snapshots(checked_at);

CREATE TABLE IF NOT EXISTS trends (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    topic          TEXT NOT NULL,
    score          REAL NOT NULL DEFAULT 0,
    source_count   INTEGER NOT NULL DEFAULT 0,
    item_ids       TEXT NOT NULL DEFAULT '[]',
    first_seen     DATETIME NOT NULL,
    last_updated   DATETIME NOT NULL,
    alerted        BOOLEAN NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_trends_score ON trends(score);
CREATE INDEX IF NOT EXISTS idx_trends_updated ON trends(last_updated);
`
