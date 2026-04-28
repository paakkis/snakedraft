package db

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"

	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Connect(url string) (*sql.DB, error) {
	database, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	if err := database.Ping(); err != nil {
		return nil, err
	}
	return database, nil
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
		database.Exec(
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version,
		)
	}
	return nil
}
