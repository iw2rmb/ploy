package config_test

import (
    "testing"

    cfg "github.com/iw2rmb/ploy/internal/config"
)

// Ensures optional Consul source does not break load when unreachable
func TestWithConsul_Optional_Unreachable_OK(t *testing.T) {
    t.Parallel()

    defaults := &cfg.Config{App: cfg.AppConfig{Name: "def"}}
    svc, err := cfg.New(
        cfg.WithDefaults(defaults),
        // unreachable or missing Consul; should not error when required=false
        cfg.WithConsul("127.0.0.1:9", "ploy/config", false),
    )
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    got := svc.Get()
    if got.App.Name != "def" {
        t.Fatalf("expected defaults preserved, got app.name=%q", got.App.Name)
    }
}

// Ensures required Consul source causes error when unreachable
func TestWithConsul_Required_Unreachable_Fails(t *testing.T) {
    t.Parallel()

    _, err := cfg.New(
        cfg.WithConsul("127.0.0.1:9", "ploy/config", true),
    )
    if err == nil {
        t.Fatalf("expected error when consul source is required and unreachable")
    }
}
