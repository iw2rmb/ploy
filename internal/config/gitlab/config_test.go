package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNormalizeValidConfig(t *testing.T) {
	t.Helper()
	raw := Config{
		APIBaseURL:      " https://gitlab.example.com/ ",
		AllowedProjects: []string{" group/project ", "group/project"},
		DefaultToken: Token{
			Name:   " primary ",
			Value:  "  secret-token  ",
			Scopes: []string{" api ", "read_repository", "read_repository"},
		},
		DeployTokens: []Token{
			{
				Name:   " deploy ",
				Value:  "deploy-token",
				Scopes: []string{" read_repository ", "write_repository"},
			},
		},
		BranchPolicies: []BranchPolicy{{
			Pattern:          " main ",
			Protected:        true,
			RequireApprovals: 2,
		}},
		RBAC: RBAC{
			Readers:  []string{"ops", " ops "},
			Updaters: []string{"secops", "platform"},
		},
	}

	normalized, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}

	if normalized.APIBaseURL != "https://gitlab.example.com" {
		t.Fatalf("expected APIBaseURL trimmed https URL, got %q", normalized.APIBaseURL)
	}
	if len(normalized.AllowedProjects) != 1 || normalized.AllowedProjects[0] != "group/project" {
		t.Fatalf("expected deduped AllowedProjects, got %+v", normalized.AllowedProjects)
	}
	if normalized.DefaultToken.Name != "primary" {
		t.Fatalf("expected DefaultToken.Name trimmed, got %q", normalized.DefaultToken.Name)
	}
	if normalized.DefaultToken.Value != "secret-token" {
		t.Fatalf("expected DefaultToken.Value trimmed, got %q", normalized.DefaultToken.Value)
	}
	expectedScopes := []string{"api", "read_repository"}
	if len(normalized.DefaultToken.Scopes) != len(expectedScopes) {
		t.Fatalf("expected %d default scopes, got %d", len(expectedScopes), len(normalized.DefaultToken.Scopes))
	}
	for i, scope := range expectedScopes {
		if normalized.DefaultToken.Scopes[i] != scope {
			t.Fatalf("expected scope[%d]=%q, got %q", i, scope, normalized.DefaultToken.Scopes[i])
		}
	}
	if normalized.DeployTokens[0].Name != "deploy" {
		t.Fatalf("expected DeployTokens[0].Name trimmed, got %q", normalized.DeployTokens[0].Name)
	}
	if normalized.BranchPolicies[0].Pattern != "main" {
		t.Fatalf("expected BranchPolicies pattern trimmed, got %q", normalized.BranchPolicies[0].Pattern)
	}
	if len(normalized.RBAC.Readers) != 1 || normalized.RBAC.Readers[0] != "ops" {
		t.Fatalf("expected deduped RBAC readers, got %+v", normalized.RBAC.Readers)
	}
	expectedUpdaters := []string{"platform", "secops"}
	if len(normalized.RBAC.Updaters) != len(expectedUpdaters) {
		t.Fatalf("expected %d updaters, got %d", len(expectedUpdaters), len(normalized.RBAC.Updaters))
	}
	for i, name := range expectedUpdaters {
		if normalized.RBAC.Updaters[i] != name {
			t.Fatalf("expected updater[%d]=%q, got %q", i, name, normalized.RBAC.Updaters[i])
		}
	}
}

func TestNormalizeRequiresHTTPS(t *testing.T) {
	t.Helper()
	_, err := Normalize(Config{APIBaseURL: "http://gitlab.example.com"})
	if err == nil {
		t.Fatalf("expected error for non-https API base URL")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	t.Helper()
	kv := newMemoryKV()
	store := NewStore(kv)
	ctx := context.Background()

	expiresAt := time.Date(2025, time.December, 31, 23, 59, 0, 0, time.UTC)
	cfg := Config{
		APIBaseURL:      "https://gitlab.example.com",
		AllowedProjects: []string{"group/project"},
		DefaultToken: Token{
			Name:      "primary",
			Value:     "s3cr3t",
			Scopes:    []string{"api", "read_repository"},
			ExpiresAt: &expiresAt,
		},
		RBAC: RBAC{Readers: []string{"ops"}, Updaters: []string{"secops"}},
	}

	if _, err := store.Save(ctx, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, rev, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rev == 0 {
		t.Fatalf("expected revision > 0")
	}
	if loaded.DefaultToken.Value != "s3cr3t" {
		t.Fatalf("expected token value, got %q", loaded.DefaultToken.Value)
	}
	if loaded.DefaultToken.ExpiresAt == nil || !loaded.DefaultToken.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiresAt preserved, got %+v", loaded.DefaultToken.ExpiresAt)
	}

	// Ensure the data persisted as JSON with expected schema.
	raw, ok := kv.data[defaultConfigKey]
	if !ok {
		t.Fatalf("expected data stored under %s", defaultConfigKey)
	}
	var encoded map[string]any
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil {
		t.Fatalf("stored value is not valid JSON: %v", err)
	}
}

func TestSanitizeMasksTokens(t *testing.T) {
	t.Helper()
	cfg := Config{
		APIBaseURL:      "https://gitlab.example.com",
		AllowedProjects: []string{"group/project"},
		DefaultToken: Token{
			Name:   "primary",
			Value:  "plaintext",
			Scopes: []string{"api"},
		},
		DeployTokens: []Token{{
			Name:   "deploy",
			Value:  "deploy-secret",
			Scopes: []string{"read_repository"},
		}},
		RBAC: RBAC{Readers: []string{"ops"}, Updaters: []string{"secops"}},
	}

	sanitized := cfg.Sanitize()
	if sanitized.DefaultToken.Value != "***redacted***" {
		t.Fatalf("expected default token masked, got %q", sanitized.DefaultToken.Value)
	}
	if sanitized.DeployTokens[0].Value != "***redacted***" {
		t.Fatalf("expected deploy token masked, got %q", sanitized.DeployTokens[0].Value)
	}
	if sanitized.APIBaseURL != cfg.APIBaseURL {
		t.Fatalf("expected APIBaseURL untouched in sanitize")
	}
}

type memoryKV struct {
	data     map[string]string
	revision int64
}

func newMemoryKV() *memoryKV {
	return &memoryKV{data: make(map[string]string)}
}

func (m *memoryKV) Get(_ context.Context, key string) (Value, bool, error) {
	val, ok := m.data[key]
	if !ok {
		return Value{}, false, nil
	}
	return Value{Data: val, Revision: m.revision}, true, nil
}

func (m *memoryKV) Put(_ context.Context, key, val string) (int64, error) {
	m.revision++
	m.data[key] = val
	return m.revision, nil
}
