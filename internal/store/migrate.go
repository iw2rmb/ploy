// Package store provides PostgreSQL-backed data persistence using pgx and sqlc.
package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migration represents a single database migration script.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// RunMigrations ensures the database schema is present and up-to-date.
// It creates a schema_version table if missing, then applies all pending migrations.
// Each migration is executed in a transaction and logged. The current version is logged on success.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Create schema_version table if it doesn't exist.
	if err := ensureVersionTable(ctx, pool); err != nil {
		return fmt.Errorf("ensure version table: %w", err)
	}

	// Load all migration files.
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	// Get current schema version.
	currentVersion, err := getCurrentVersion(ctx, pool)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// Apply pending migrations.
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		slog.Info("applying migration", "version", m.Version, "name", m.Name)

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin transaction for migration %d: %w", m.Version, err)
		}

		// Execute migration SQL (supports multi-statement files).
		if err := execMigrationSQL(ctx, tx, m.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute migration %d (%s): %w", m.Version, m.Name, err)
		}

		// Update schema_version.
		if _, err := tx.Exec(ctx, "INSERT INTO ploy.schema_version (version, name) VALUES ($1, $2)", m.Version, m.Name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}

		slog.Info("migration applied", "version", m.Version, "name", m.Name)
		currentVersion = m.Version
	}

	slog.Info("schema up-to-date", "version", currentVersion)
	return nil
}

// ensureVersionTable creates the schema_version table if it doesn't exist.
func ensureVersionTable(ctx context.Context, pool *pgxpool.Pool) error {
	// Use separate Exec calls to avoid multi-statement execution issues
	// with the extended protocol.
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS ploy`); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS ploy.schema_version (
        version INTEGER PRIMARY KEY,
        name TEXT NOT NULL,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
    )`)
	return err
}

// getCurrentVersion returns the highest applied migration version.
// Returns 0 if no migrations have been applied.
func getCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var version int
	err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM ploy.schema_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("query schema_version: %w", err)
	}
	return version, nil
}

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
		name = strings.TrimSuffix(strings.TrimPrefix(entry.Name(), fmt.Sprintf("%03d_", version)), ".sql")

		// Read migration SQL.
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	// Sort by version.
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// execMigrationSQL splits the provided SQL into statements and executes them
// sequentially within the provided transaction. It supports semicolon-delimited
// statements and accounts for comments and common quoting modes (single quotes,
// double quotes, and dollar-quoted strings) so that semicolons inside those
// constructs do not split statements.
func execMigrationSQL(ctx context.Context, tx pgx.Tx, sql string) error {
	stmts := splitSQLStatements(sql)
	for _, s := range stmts {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// splitSQLStatements splits a SQL script into individual statements by
// semicolons, ignoring semicolons that appear inside string literals,
// identifiers, dollar-quoted strings, or comments. Returned statements do not
// include the terminating semicolon.
func splitSQLStatements(script string) []string {
	var stmts []string
	var b strings.Builder

	inSingle := false    // '...'
	inDouble := false    // "..."
	inLineC := false     // -- ... \n
	inBlockC := false    // /* ... */
	inDollar := false    // $tag$ ... $tag$ or $$ ... $$
	var dollarTag string // tag part between the $...

	// helper to flush current buffer to statements
	flush := func() {
		s := strings.TrimSpace(b.String())
		if s != "" {
			stmts = append(stmts, s)
		}
		b.Reset()
	}

	// Iterate runes to properly handle multibyte tags
	rs := []rune(script)
	for i := 0; i < len(rs); i++ {
		r := rs[i]

		// Handle end of line for line comments
		if inLineC {
			b.WriteRune(r)
			if r == '\n' {
				inLineC = false
			}
			continue
		}

		// Handle block comments
		if inBlockC {
			b.WriteRune(r)
			if r == '*' && i+1 < len(rs) && rs[i+1] == '/' {
				b.WriteRune('/')
				i++
				inBlockC = false
			}
			continue
		}

		// Inside dollar-quoted string
		if inDollar {
			b.WriteRune(r)
			if r == '$' {
				if dollarTag == "" {
					// tagless $$ ... $$
					if i+1 < len(rs) && rs[i+1] == '$' {
						b.WriteRune('$')
						i++
						inDollar = false
					}
				} else {
					// Try to match closing $tag$
					tag := []rune(dollarTag)
					n := len(tag)
					if i+1+n < len(rs) { // need $ + tag + $
						match := true
						for j := 0; j < n; j++ {
							if rs[i+1+j] != tag[j] {
								match = false
								break
							}
						}
						if match && rs[i+1+n] == '$' {
							// consume tag and trailing $
							for j := 0; j < n+1; j++ {
								b.WriteRune(rs[i+1+j])
							}
							i += n + 1
							inDollar = false
							dollarTag = ""
						}
					}
				}
			}
			continue
		}

		// Not in any special region
		switch r {
		case '\'', '"':
			b.WriteRune(r)
			if r == '\'' && !inDouble {
				if inSingle {
					// Check for escaped ''
					if i+1 < len(rs) && rs[i+1] == '\'' {
						// stay in single quote, write the escape
						b.WriteRune('\'')
						i++
					} else {
						inSingle = false
					}
				} else if !inSingle {
					inSingle = true
				}
				continue
			}
			if r == '"' && !inSingle {
				inDouble = !inDouble
				continue
			}
		case '-':
			// Start of line comment? "--"
			if !inSingle && !inDouble && i+1 < len(rs) && rs[i+1] == '-' {
				b.WriteString("--")
				i++
				inLineC = true
				continue
			}
			b.WriteRune(r)
			continue
		case '/':
			// Start of block comment? "/*"
			if !inSingle && !inDouble && i+1 < len(rs) && rs[i+1] == '*' {
				b.WriteString("/*")
				i++
				inBlockC = true
				continue
			}
			b.WriteRune(r)
			continue
		case '$':
			// Dollar-quoted string start? $tag$
			if !inSingle && !inDouble {
				// Handle tagless $$
				if i+1 < len(rs) && rs[i+1] == '$' {
					b.WriteString("$$")
					i++
					inDollar = true
					dollarTag = ""
					continue
				}
				// collect tag characters (letters, digits, underscore)
				j := i + 1
				var tag strings.Builder
				for j < len(rs) && ((rs[j] >= 'a' && rs[j] <= 'z') || (rs[j] >= 'A' && rs[j] <= 'Z') || (rs[j] >= '0' && rs[j] <= '9') || rs[j] == '_') {
					tag.WriteRune(rs[j])
					j++
				}
				if j < len(rs) && rs[j] == '$' { // found $tag$
					// write the full $tag$
					b.WriteRune('$')
					for k := i + 1; k <= j; k++ {
						b.WriteRune(rs[k])
					}
					i = j
					inDollar = true
					dollarTag = tag.String()
					continue
				}
			}
			b.WriteRune(r)
			continue
		case ';':
			// Only split when not inside quotes
			if !inSingle && !inDouble {
				// finalize current statement without the semicolon
				flush()
				continue
			}
			b.WriteRune(r)
			continue
		}

		// default: append rune
		b.WriteRune(r)
	}

	// flush remaining
	flush()
	return stmts
}
