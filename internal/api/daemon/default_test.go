package daemon

import (
	"os"
	"testing"

	apiconfig "github.com/iw2rmb/ploy/internal/api/config"
)

func withEnv(k, v string, fn func()) {
	old, had := os.LookupEnv(k)
	if v == "" {
		_ = os.Unsetenv(k)
	} else {
		_ = os.Setenv(k, v)
	}
	defer func() {
		if had {
			_ = os.Setenv(k, old)
		} else {
			_ = os.Unsetenv(k)
		}
	}()
	fn()
}

func TestResolvePostgresDSN_PrefersServerEnv(t *testing.T) {
	cfg := apiconfig.Config{Postgres: apiconfig.PostgresConfig{DSN: "cfg-dsn"}}
	withEnv(envServerPgDSN, "server-dsn", func() {
		withEnv(envPostgresDSN, "compat-dsn", func() {
			got := resolvePostgresDSN(cfg)
			if got != "server-dsn" {
				t.Fatalf("expected server env to win, got %q", got)
			}
		})
	})
}

func TestResolvePostgresDSN_FallsBackToCompatEnv(t *testing.T) {
	cfg := apiconfig.Config{Postgres: apiconfig.PostgresConfig{DSN: "cfg-dsn"}}
	withEnv(envServerPgDSN, "", func() {
		withEnv(envPostgresDSN, "compat-dsn", func() {
			got := resolvePostgresDSN(cfg)
			if got != "compat-dsn" {
				t.Fatalf("expected compat env to win, got %q", got)
			}
		})
	})
}

func TestResolvePostgresDSN_FallsBackToConfig(t *testing.T) {
	cfg := apiconfig.Config{Postgres: apiconfig.PostgresConfig{DSN: "cfg-dsn"}}
	withEnv(envServerPgDSN, "", func() {
		withEnv(envPostgresDSN, "", func() {
			got := resolvePostgresDSN(cfg)
			if got != "cfg-dsn" {
				t.Fatalf("expected cfg dsn, got %q", got)
			}
		})
	})
}
