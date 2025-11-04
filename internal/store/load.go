package store

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// loadMigrations reads all .sql files from the migrations directory and returns them sorted by version.
func loadMigrations() ([]Migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Parse version from filename (e.g., "001_initial.sql" -> version 1).
		var version int
		var name string
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			return nil, fmt.Errorf("parse version from %s: %w", entry.Name(), err)
		}

		// Extract name after the version and underscore, without .sql extension.
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration filename: %s", entry.Name())
		}
		name = strings.TrimSuffix(parts[1], ".sql")

		// Read file content.
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{Version: version, Name: name, SQL: string(content)})
	}

	// Sort by version.
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })

	return migrations, nil
}
