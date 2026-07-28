CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    created_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS urls (
    id TEXT PRIMARY KEY,
    short_code TEXT NOT NULL UNIQUE,
    original_url TEXT NOT NULL,
    expires_in TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    visits BIGINT NOT NULL DEFAULT 0 CHECK (visits >= 0)
);

CREATE INDEX IF NOT EXISTS idx_urls_created_by_created_at
    ON urls (created_by, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_urls_expires_at
    ON urls (expires_at);
