package main

import (
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// resolvePgDSN returns the PostgreSQL DSN from environment or config.
// Precedence: PLOY_DB_DSN > config.postgres.dsn
func resolvePgDSN(cfg config.Config) string {
	if dsn := strings.TrimSpace(os.Getenv("PLOY_DB_DSN")); dsn != "" {
		return dsn
	}
	// Treat ${...} placeholders (as written by bootstrap) as unset to avoid
	// attempting to connect with an invalid DSN when the env var is missing.
	dsn := strings.TrimSpace(cfg.Postgres.DSN)
	if strings.HasPrefix(dsn, "${") && strings.Contains(dsn, "}") {
		return ""
	}
	return dsn
}
