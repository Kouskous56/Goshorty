-- Keyset pagination support for the URL list queries. The list filters on
-- expiry and (for non-admins) the owner, then orders by
-- (created_at DESC, short_code ASC); these indexes let PostgreSQL satisfy the
-- filter and ordering without a full table scan or an explicit sort.
CREATE INDEX IF NOT EXISTS idx_urls_list_user
    ON urls (created_by, expires_at, created_at DESC, short_code ASC);

CREATE INDEX IF NOT EXISTS idx_urls_list_admin
    ON urls (expires_at, created_at DESC, short_code ASC);