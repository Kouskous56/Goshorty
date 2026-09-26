package storage

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"goshorty/models"
	"goshorty/utils"
)

const databaseOperationTimeout = 5 * time.Second

//go:embed migrations/*.sql
var migrationFiles embed.FS

// PostgresStore persists both users and URLs in PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore connects, migrates the schema, and bootstraps the admin.
func NewPostgresStore(ctx context.Context, databaseURL, adminPassword, adminEmail string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}

	store := &PostgresStore{pool: pool}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	if err := store.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := store.bootstrapAdmin(ctx, adminPassword, adminEmail); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

// Close releases all PostgreSQL connections.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// Ping verifies that PostgreSQL is available for readiness checks.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			checksum TEXT,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `
		ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT
	`); err != nil {
		return fmt.Errorf("upgrade migration ledger: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("list database migrations: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version := entry.Name()
		migration, err := migrationFiles.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("read database migration %s: %w", version, err)
		}
		checksum := migrationChecksum(migration)

		tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin database migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(7348200)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("lock database migrations: %w", err)
		}

		var storedChecksum *string
		err = tx.QueryRow(ctx,
			`SELECT checksum FROM schema_migrations WHERE version = $1`,
			version,
		).Scan(&storedChecksum)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			return fmt.Errorf("check database migration %s: %w", version, err)
		}
		if err == nil {
			if storedChecksum == nil {
				if _, err := tx.Exec(ctx,
					`UPDATE schema_migrations SET checksum = $1 WHERE version = $2`,
					checksum, version,
				); err != nil {
					tx.Rollback(ctx)
					return fmt.Errorf("backfill migration checksum %s: %w", version, err)
				}
			} else if *storedChecksum != checksum {
				tx.Rollback(ctx)
				return fmt.Errorf("migration %s checksum mismatch", version)
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit migration check %s: %w", version, err)
			}
			continue
		}

		if _, err := tx.Conn().PgConn().Exec(ctx, string(migration)).ReadAll(); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply database migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`,
			version, checksum,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record database migration %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit database migration %s: %w", version, err)
		}
	}
	return nil
}

func migrationChecksum(migration []byte) string {
	digest := sha256.Sum256(migration)
	return hex.EncodeToString(digest[:])
}

func (s *PostgresStore) bootstrapAdmin(ctx context.Context, password, email string) error {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap admin password: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, email, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (username) DO NOTHING
	`, utils.GenerateID(), defaultAdmin, passwordHash, email, models.RoleAdmin, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("bootstrap admin user: %w", err)
	}
	return nil
}

func (s *PostgresStore) SetIfAbsent(shortCode string, data *models.URLData) error {
	ctx, cancel := databaseContext()
	defer cancel()

	tag, err := s.pool.Exec(ctx, `
		INSERT INTO urls (
			id, short_code, original_url, expires_in, expires_at,
			created_at, created_by, visits
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (short_code) DO UPDATE SET
			id = EXCLUDED.id,
			original_url = EXCLUDED.original_url,
			expires_in = EXCLUDED.expires_in,
			expires_at = EXCLUDED.expires_at,
			created_at = EXCLUDED.created_at,
			created_by = EXCLUDED.created_by,
			visits = EXCLUDED.visits
		WHERE urls.expires_at <= NOW()
	`, data.ID, shortCode, data.OriginalURL, data.ExpiresIn, data.ExpiresAt,
		data.CreatedAt, data.CreatedBy, data.Visits)
	if err != nil {
		return mapPostgresError(err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrKeyExists, shortCode)
	}
	return nil
}

func (s *PostgresStore) Get(shortCode string) (*models.URLData, error) {
	ctx, cancel := databaseContext()
	defer cancel()

	return scanURL(s.pool.QueryRow(ctx, `
		SELECT id, short_code, original_url, expires_in, expires_at,
		       created_at, created_by, visits
		FROM urls
		WHERE short_code = $1 AND expires_at > NOW()
	`, shortCode))
}

func (s *PostgresStore) Delete(shortCode string) error {
	ctx, cancel := databaseContext()
	defer cancel()

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM urls
		WHERE short_code = $1 AND expires_at > NOW()
	`, shortCode)
	if err != nil {
		return fmt.Errorf("delete URL: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKeyNotFound
	}
	return nil
}

func (s *PostgresStore) GetAndIncrement(shortCode string) (*models.URLData, error) {
	ctx, cancel := databaseContext()
	defer cancel()

	return scanURL(s.pool.QueryRow(ctx, `
		UPDATE urls
		SET visits = visits + 1
		WHERE short_code = $1 AND expires_at > NOW()
		RETURNING id, short_code, original_url, expires_in, expires_at,
		          created_at, created_by, visits
	`, shortCode))
}

func (s *PostgresStore) GetAllFor(userID, role string) ([]*models.URLData, error) {
	ctx, cancel := databaseContext()
	defer cancel()

	query := `
		SELECT id, short_code, original_url, expires_in, expires_at,
		       created_at, created_by, visits
		FROM urls
		WHERE expires_at > NOW()
	`
	args := []any{}
	if role != models.RoleAdmin {
		query += " AND created_by = $1"
		args = append(args, userID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list URLs: %w", err)
	}
	defer rows.Close()

	urls := make([]*models.URLData, 0)
	for rows.Next() {
		data, err := scanURL(rows)
		if err != nil {
			return nil, err
		}
		urls = append(urls, data)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate URLs: %w", err)
	}
	return urls, nil
}

// ListURLs returns a single keyset page (newest first) of active URLs visible
// to the caller. Unlike GetAllFor this never loads the whole table: the page
// is fetched with a LIMIT (limit+1 rows to detect whether more remain) and
// the cursor filters directly in SQL via the (created_at, short_code) key.
func (s *PostgresStore) ListURLs(userID, role string, cursor models.URLCursor, limit int) (URLListResult, error) {
	ctx, cancel := databaseContext()
	defer cancel()

	scopeWhere := "WHERE expires_at > NOW()"
	scopeArgs := []any{}
	if role != models.RoleAdmin {
		scopeArgs = append(scopeArgs, userID)
		scopeWhere += fmt.Sprintf(" AND created_by = $%d", len(scopeArgs))
	}

	// The cursor only filters the page; the total counts every visible row,
	// so the count query keeps only the scope placeholders.
	pageWhere := scopeWhere
	pageArgs := append([]any{}, scopeArgs...)
	if cursor.CreatedAtUnixNano != 0 || cursor.ShortCode != "" {
		pageArgs = append(pageArgs, time.Unix(0, cursor.CreatedAtUnixNano).UTC(), cursor.ShortCode)
		n := len(pageArgs)
		pageWhere += fmt.Sprintf(
			" AND (created_at < $%d OR (created_at = $%d AND short_code > $%d))",
			n-1, n-1, n,
		)
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM urls `+scopeWhere, scopeArgs...).Scan(&total); err != nil {
		return URLListResult{}, fmt.Errorf("count URLs: %w", err)
	}

	pageArgs = append(pageArgs, limit+1)
	rows, err := s.pool.Query(ctx, `
		SELECT id, short_code, original_url, expires_in, expires_at,
		       created_at, created_by, visits
		FROM urls `+pageWhere+fmt.Sprintf(` ORDER BY created_at DESC, short_code ASC LIMIT $%d`, len(pageArgs)), pageArgs...)
	if err != nil {
		return URLListResult{}, fmt.Errorf("list URLs: %w", err)
	}
	defer rows.Close()

	items := make([]*models.URLData, 0, limit+1)
	for rows.Next() {
		data, err := scanURL(rows)
		if err != nil {
			return URLListResult{}, err
		}
		items = append(items, data)
	}
	if err := rows.Err(); err != nil {
		return URLListResult{}, fmt.Errorf("iterate URLs: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return URLListResult{Items: items, Total: total, HasMore: hasMore}, nil
}

func (s *PostgresStore) StatsFor(userID, role string) (map[string]interface{}, error) {
	ctx, cancel := databaseContext()
	defer cancel()

	query := `
		SELECT COUNT(*), COALESCE(SUM(visits), 0)
		FROM urls
		WHERE expires_at > NOW()
	`
	args := []any{}
	if role != models.RoleAdmin {
		query += " AND created_by = $1"
		args = append(args, userID)
	}

	var totalURLs int
	var totalVisits int64
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&totalURLs, &totalVisits); err != nil {
		return nil, fmt.Errorf("load statistics: %w", err)
	}
	return map[string]interface{}{
		"total_urls":   totalURLs,
		"total_visits": totalVisits,
	}, nil
}

func (s *PostgresStore) CreateUser(username, password, email string) (*models.User, error) {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}
	user := &models.User{
		ID:        utils.GenerateID(),
		Username:  username,
		Password:  passwordHash,
		Email:     email,
		Role:      models.RoleUser,
		CreatedAt: time.Now().Unix(),
	}

	ctx, cancel := databaseContext()
	defer cancel()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, email, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, user.ID, user.Username, user.Password, user.Email, user.Role, user.CreatedAt)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	return cloneUser(user), nil
}

func (s *PostgresStore) GetUser(username string) (*models.User, error) {
	ctx, cancel := databaseContext()
	defer cancel()
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, email, role, created_at, token_version
		FROM users WHERE username = $1
	`, username))
}

func (s *PostgresStore) GetUserByID(id string) (*models.User, error) {
	ctx, cancel := databaseContext()
	defer cancel()
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, email, role, created_at, token_version
		FROM users WHERE id = $1
	`, id))
}

func (s *PostgresStore) VerifyPassword(username, password string) (bool, error) {
	user, err := s.GetUser(username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Equalize timing with the real bcrypt path so that a missing
			// username does not reveal itself through faster responses.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash()), []byte(password))
		}
		return false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return false, nil
	}
	return true, nil
}

// UpdatePassword replaces a user's bcrypt password hash.
func (s *PostgresStore) UpdatePassword(userID, password string) error {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	ctx, cancel := databaseContext()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET password_hash = $1, token_version = token_version + 1
		WHERE id = $2
	`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// RevokeTokens invalidates every token previously issued for a user.
func (s *PostgresStore) RevokeTokens(userID string) error {
	ctx, cancel := databaseContext()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET token_version = token_version + 1 WHERE id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("revoke user tokens: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *PostgresStore) UpdateUserRole(username, role string) error {
	if role != models.RoleAdmin && role != models.RoleUser {
		return ErrInvalidRole
	}

	ctx, cancel := databaseContext()
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin role update: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(7348201)); err != nil {
		return fmt.Errorf("lock admin invariant: %w", err)
	}
	user, err := scanUser(tx.QueryRow(ctx, `
		SELECT id, username, password_hash, email, role, created_at, token_version
		FROM users WHERE username = $1 FOR UPDATE
	`, username))
	if err != nil {
		return err
	}
	if user.Role == models.RoleAdmin && role != models.RoleAdmin {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET role = $1 WHERE username = $2`, role, username); err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit role update: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetAllUsers() ([]*models.User, error) {
	ctx, cancel := databaseContext()
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, password_hash, email, role, created_at, token_version
		FROM users ORDER BY created_at ASC, username ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]*models.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

// ListUsers returns a single keyset page (username ASC) of all users. The
// page is fetched with LIMIT (limit+1 rows to detect whether more remain) and
// the cursor filters directly in SQL on the username key.
func (s *PostgresStore) ListUsers(cursor models.UserCursor, limit int) (UserListResult, error) {
	ctx, cancel := databaseContext()
	defer cancel()

	scopeWhere := ""
	scopeArgs := []any{}

	// The cursor only filters the page; the total counts every user, so the
	// count query keeps only the (empty) scope placeholders.
	pageWhere := scopeWhere
	pageArgs := append([]any{}, scopeArgs...)
	if cursor.Username != "" {
		pageArgs = append(pageArgs, cursor.Username)
		pageWhere = fmt.Sprintf(" WHERE username > $%d", len(pageArgs))
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`+scopeWhere, scopeArgs...).Scan(&total); err != nil {
		return UserListResult{}, fmt.Errorf("count users: %w", err)
	}

	pageArgs = append(pageArgs, limit+1)
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, password_hash, email, role, created_at, token_version
		FROM users`+pageWhere+fmt.Sprintf(` ORDER BY username ASC LIMIT $%d`, len(pageArgs)), pageArgs...)
	if err != nil {
		return UserListResult{}, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	items := make([]*models.User, 0, limit+1)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return UserListResult{}, err
		}
		items = append(items, user)
	}
	if err := rows.Err(); err != nil {
		return UserListResult{}, fmt.Errorf("iterate users: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return UserListResult{Items: items, Total: total, HasMore: hasMore}, nil
}

func (s *PostgresStore) DeleteUser(username string) error {
	ctx, cancel := databaseContext()
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin user deletion: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(7348201)); err != nil {
		return fmt.Errorf("lock admin invariant: %w", err)
	}
	user, err := scanUser(tx.QueryRow(ctx, `
		SELECT id, username, password_hash, email, role, created_at, token_version
		FROM users WHERE username = $1 FOR UPDATE
	`, username))
	if err != nil {
		return err
	}
	if user.Role == models.RoleAdmin {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE username = $1`, username); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user deletion: %w", err)
	}
	return nil
}

// DeleteExpiredURLs removes expired records in bounded batches.
func (s *PostgresStore) DeleteExpiredURLs(ctx context.Context, limit int) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM urls
		WHERE id IN (
			SELECT id FROM urls
			WHERE expires_at <= NOW()
			ORDER BY expires_at
			LIMIT $1
		)
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired URLs: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RunCleanup periodically deletes expired URL rows until ctx is cancelled.
func (s *PostgresStore) RunCleanup(ctx context.Context, interval time.Duration, batchSize int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, databaseOperationTimeout)
			_, _ = s.DeleteExpiredURLs(cleanupCtx, batchSize)
			cancel()
		}
	}
}

func scanURL(row interface{ Scan(...any) error }) (*models.URLData, error) {
	data := &models.URLData{}
	if err := row.Scan(
		&data.ID, &data.ShortCode, &data.OriginalURL, &data.ExpiresIn,
		&data.ExpiresAt, &data.CreatedAt, &data.CreatedBy, &data.Visits,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("scan URL: %w", err)
	}
	return data, nil
}

func scanUser(row interface{ Scan(...any) error }) (*models.User, error) {
	user := &models.User{}
	if err := row.Scan(
		&user.ID, &user.Username, &user.Password, &user.Email,
		&user.Role, &user.CreatedAt, &user.TokenVersion,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return user, nil
}

func mapPostgresError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if pgErr.ConstraintName == "users_username_key" {
				return ErrUsernameExists
			}
			if pgErr.ConstraintName == "urls_short_code_key" {
				return ErrKeyExists
			}
		case "23503":
			return fmt.Errorf("referenced user does not exist: %w", err)
		}
	}
	return fmt.Errorf("PostgreSQL operation failed: %w", err)
}

func databaseContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), databaseOperationTimeout)
}
