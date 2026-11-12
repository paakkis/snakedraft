package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"time"

	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect opens a Postgres connection, retrying up to 10 times with
// exponential backoff. This tolerates the race between the backend
// container starting and Docker's internal DNS registering the "db" host.
func Connect(url string) (*sql.DB, error) {
	var (
		database *sql.DB
		err      error
	)
	for attempt := 1; attempt <= 10; attempt++ {
		database, err = sql.Open("postgres", url)
		if err == nil {
			err = database.Ping()
		}
		if err == nil {
			return database, nil
		}
		wait := time.Duration(attempt) * 500 * time.Millisecond
		log.Printf("DB not ready (attempt %d/10): %v, retrying in %s", attempt, err, wait)
		time.Sleep(wait)
	}
	return nil, fmt.Errorf("could not connect to database after 10 attempts: %w", err)
}

func Migrate(database *sql.DB) error {
	_, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ DEFAULT now()
	)`)
	if err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		version := entry.Name()

		var exists bool
		database.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`,
			version,
		).Scan(&exists)
		if exists {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", version, err)
		}
		if _, err := database.Exec(string(content)); err != nil {
			return fmt.Errorf("applying migration %s: %w", version, err)
		}
		database.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, version)
		log.Printf("Applied migration: %s", version)
	}
	return nil
}
