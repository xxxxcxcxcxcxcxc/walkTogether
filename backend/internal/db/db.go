package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

func Connect(dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("database dsn is empty")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return db, nil
}

func ApplyMigrations(dsn string, migrationDir string) error {
	if strings.TrimSpace(dsn) == "" {
		return nil
	}

	if strings.TrimSpace(migrationDir) == "" {
		return fmt.Errorf("database migration directory is empty")
	}

	db, err := Connect(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	files, err := migrationFiles(migrationDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		version := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))

		var applied bool
		if err := db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)",
			version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationDir, file.Name()))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file.Name(), err)
		}

		if _, err = db.Exec(string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file.Name(), err)
		}

		if _, err = db.Exec("INSERT INTO schema_migrations(version) VALUES ($1)", version); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}

	return nil
}

func ResolveMigrationsDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	candidates := []string{
		filepath.Join(cwd, "migrations"),
		filepath.Join(cwd, "..", "migrations"),
		filepath.Join(cwd, "backend", "migrations"),
		filepath.Join(cwd, "..", "backend", "migrations"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func migrationFiles(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read database migration directory %s: %w", dir, err)
	}

	filtered := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		filtered = append(filtered, entry)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Name() < filtered[j].Name()
	})

	return filtered, nil
}
