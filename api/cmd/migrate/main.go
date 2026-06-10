// Command migrate applies or reverts database schema migrations using the
// golang-migrate library against the project's migrations/ directory. It
// reuses the same configuration loader as the API server so it always talks
// to the same database.
//
// Usage:
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/config"
)

const (
	migrationsSource = "file://migrations"

	// pgxDriverScheme is the scheme golang-migrate's pgx/v5 database driver
	// registers itself under. The application's DSN uses the standard
	// "postgres://" scheme, so it must be rewritten before being handed to
	// migrate.New.
	pgxDriverScheme = "pgx5"
)

// postgresSchemes lists the URL schemes produced by internal/config that
// must be translated to the pgx5 driver scheme golang-migrate expects.
var postgresSchemes = []string{"postgres://", "postgresql://"}

func main() {
	if err := run(os.Args); err != nil {
		log.Fatalf("migrate: %v", err)
	}
}

// run parses the requested direction, loads configuration, opens a migrate
// instance against the configured database and applies the corresponding
// operation. It returns an error rather than calling os.Exit directly so it
// stays testable in isolation from process-level concerns.
func run(args []string) error {
	direction, err := parseDirection(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m, err := migrate.New(migrationsSource, toPgxDriverURL(cfg.DatabaseURL))
	if err != nil {
		return fmt.Errorf("opening migrator: %w", err)
	}
	defer closeMigrator(m)

	return applyDirection(m, direction)
}

// toPgxDriverURL rewrites a standard "postgres://" / "postgresql://" DSN, as
// produced by internal/config, into the "pgx5://" form golang-migrate's
// pgx/v5 database driver expects to select itself by scheme. URLs that
// already use a different scheme are returned unchanged.
func toPgxDriverURL(dsn string) string {
	for _, scheme := range postgresSchemes {
		if strings.HasPrefix(dsn, scheme) {
			return pgxDriverScheme + "://" + strings.TrimPrefix(dsn, scheme)
		}
	}
	return dsn
}

// direction identifies which migration operation to run.
type direction string

const (
	directionUp   direction = "up"
	directionDown direction = "down"
)

// parseDirection extracts and validates the subcommand from CLI arguments.
func parseDirection(args []string) (direction, error) {
	if len(args) < 2 {
		return "", errors.New("missing subcommand: expected \"up\" or \"down\"")
	}

	switch direction(args[1]) {
	case directionUp:
		return directionUp, nil
	case directionDown:
		return directionDown, nil
	default:
		return "", fmt.Errorf("unknown subcommand %q: expected \"up\" or \"down\"", args[1])
	}
}

// applyDirection runs the migration operation matching direction, treating
// migrate.ErrNoChange as a successful no-op rather than a failure.
func applyDirection(m *migrate.Migrate, d direction) error {
	var err error
	switch d {
	case directionUp:
		err = m.Up()
	case directionDown:
		err = m.Down()
	default:
		return fmt.Errorf("unsupported direction %q", d)
	}

	if err == nil {
		log.Printf("migrate: %s completed", d)
		return nil
	}

	if errors.Is(err, migrate.ErrNoChange) {
		log.Printf("migrate: %s: no change", d)
		return nil
	}

	return fmt.Errorf("running %s: %w", d, err)
}

// closeMigrator releases the source and database connections held by m,
// logging (rather than swallowing) any error encountered while doing so.
func closeMigrator(m *migrate.Migrate) {
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		log.Printf("migrate: closing source: %v", srcErr)
	}
	if dbErr != nil {
		log.Printf("migrate: closing database: %v", dbErr)
	}
}
