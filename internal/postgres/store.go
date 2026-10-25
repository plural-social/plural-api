package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxm "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // Registers the "pgx" database/sql driver.
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store is the Postgres store for every domain in the monolith.
// ponytail: one store and one schema for now; split per domain when a domain
// moves into its own service.
type Store struct {
	db *sql.DB
}

// NewStore connects to Postgres and applies any pending migrations.
func NewStore(ctx context.Context, connection string) (*Store, error) {
	if err := migrateUp(connection); err != nil {
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	db, err := sql.Open("pgx", connection)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return &Store{db: db}, nil
}

// migrateUp uses its own connection because closing a migrate instance also
// closes the *sql.DB it was given.
func migrateUp(connection string) error {
	db, err := sql.Open("pgx", connection)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		db.Close()
		return fmt.Errorf("loading migrations: %w", err)
	}
	driver, err := pgxm.WithInstance(db, &pgxm.Config{})
	if err != nil {
		db.Close()
		return fmt.Errorf("creating migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		driver.Close()
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer m.Close()

	if upErr := m.Up(); upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", upErr)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Close() error {
	return s.db.Close()
}
