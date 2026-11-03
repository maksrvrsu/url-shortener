package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"url-shortener/internal/storage"
	"url-shortener/migrations"
)

type Storage struct {
	db *sql.DB
}

// New creates a new PostgreSQL storage instance, establishes a connection,
// and automatically runs pending migrations from embedded SQL files.
func New(storagePath string) (*Storage, error) {
	const op = "storage.postgres.New"

	db, err := sql.Open("pgx", storagePath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: ping failed: %w", op, err)
	}

	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: migrations failed: %w", op, err)
	}

	return &Storage{db: db}, nil
}

// NewWithDB wraps an existing *sql.DB into Storage (useful for tests).
func NewWithDB(db *sql.DB) *Storage {
	return &Storage{db: db}
}

func runMigrations(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("could not create postgres driver: %w", err)
	}

	d, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("could not create iofs source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", d, "postgres", driver)
	if err != nil {
		return fmt.Errorf("could not create migration instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up failed: %w", err)
	}

	return nil
}

// SaveURL inserts a new URL with the given alias into PostgreSQL.
// If the alias already exists, it returns storage.ErrURLExists.
func (s *Storage) SaveURL(ctx context.Context, urlToSave, alias string) error {
	const op = "storage.postgres.SaveURL"

	stmt, err := s.db.PrepareContext(ctx, "INSERT INTO url(url, alias) VALUES($1, $2)")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, urlToSave, alias)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return storage.ErrURLExists
		}
		if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
			return storage.ErrURLExists
		}
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// GetURL retrieves the original URL by its alias.
// If not found, it returns storage.ErrURLNotFound.
func (s *Storage) GetURL(ctx context.Context, alias string) (string, error) {
	const op = "storage.postgres.GetURL"

	stmt, err := s.db.PrepareContext(ctx, "SELECT url FROM url WHERE alias = $1")
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	var resURL string
	err = stmt.QueryRowContext(ctx, alias).Scan(&resURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", storage.ErrURLNotFound
		}
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return resURL, nil
}

// UpdateURL updates the destination URL for the given alias.
// If the alias does not exist, it returns storage.ErrURLNotFound.
func (s *Storage) UpdateURL(ctx context.Context, alias, newURL string) error {
	const op = "storage.postgres.UpdateURL"

	stmt, err := s.db.PrepareContext(ctx, "UPDATE url SET url = $1 WHERE alias = $2")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	res, err := stmt.ExecContext(ctx, newURL, alias)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if rowsAffected == 0 {
		return storage.ErrURLNotFound
	}

	return nil
}

// DeleteURL deletes the record with the given alias.
// If the alias does not exist, it returns storage.ErrURLNotFound.
func (s *Storage) DeleteURL(ctx context.Context, alias string) error {
	const op = "storage.postgres.DeleteURL"

	stmt, err := s.db.PrepareContext(ctx, "DELETE FROM url WHERE alias = $1")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()

	res, err := stmt.ExecContext(ctx, alias)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if rowsAffected == 0 {
		return storage.ErrURLNotFound
	}

	return nil
}

// Close closes the underlying database connection pool.
func (s *Storage) Close() error {
	return s.db.Close()
}
