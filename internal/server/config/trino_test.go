package config_test

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestLoadTrinoFromEnvPreservesCredentials(t *testing.T) {
	for _, tt := range []struct {
		name, user, password string
	}{
		{name: "missing credentials remain empty"},
		{name: "missing password remains empty", user: "service-account"},
		{name: "missing user remains empty", password: "password"},
		{name: "credentials retain special characters and spaces", user: "service@account", password: " p@ss:+/word "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLOY_TRINO_USER", tt.user)
			t.Setenv("PLOY_TRINO_PASSWORD", tt.password)
			got := config.LoadTrinoFromEnv()
			if got.User != tt.user || got.Password != tt.password {
				t.Fatal("Trino credentials differ from environment values")
			}
		})
	}
}
