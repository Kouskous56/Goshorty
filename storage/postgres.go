package storage

import (
	"context"
	"embed"
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

func (s *PostgresStore) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
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

		tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin database migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(7348200)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("lock database migrations: %w", err)
		}

		var applied bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`,
			version,
		).Scan(&applied); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("check database migration %s: %w", version, err)
		}
		if applied {
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
			`INSERT INTO schema_migrations (version) VALUES ($1)`,
			version,
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
		SELECT id, username, password_hash, email, role, created_at
		FROM users WHERE username = $1
	`, username))
}

func (s *PostgresStore) GetUserByID(id string) (*models.User, error) {
	ctx, cancel := databaseContext()
	defer cancel()
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, email, role, created_at
		FROM users WHERE id = $1
	`, id))
}

func (s *PostgresStore) VerifyPassword(username, password string) (bool, error) {
	user, err := s.GetUser(username)
	if err != nil {
		return false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return false, nil
	}
	return true, nil
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
		SELECT id, username, password_hash, email, role, created_at
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
		SELECT id, username, password_hash, email, role, created_at
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
		SELECT id, username, password_hash, email, role, created_at
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
		&user.Role, &user.CreatedAt,
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
