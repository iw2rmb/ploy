package store

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestConfigEnv_CRUD(t *testing.T) {
	ctx, db := newTestStore(t)

	upsert := func(key, target, value string, secret bool) ConfigEnv {
		t.Helper()
		if err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: key, Target: target, Value: value, Secret: secret,
		}); err != nil {
			t.Fatalf("UpsertGlobalEnv(%s/%s) failed: %v", key, target, err)
		}
		env, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: key, Target: target})
		if err != nil {
			t.Fatalf("GetGlobalEnv(%s/%s) failed: %v", key, target, err)
		}
		return env
	}

	first := upsert("TEST_CONFIG_ENV", "gates", "first-value", true)
	if first.Value != "first-value" || !first.Secret || !first.UpdatedAt.Valid {
		t.Fatalf("inserted env = {value:%q secret:%v updated:%v}, want first-value/true/valid",
			first.Value, first.Secret, first.UpdatedAt.Valid)
	}

	time.Sleep(10 * time.Millisecond)
	updated := upsert("TEST_CONFIG_ENV", "gates", "second-value", false)
	if updated.Value != "second-value" || updated.Secret {
		t.Fatalf("updated env = {value:%q secret:%v}, want second-value/false", updated.Value, updated.Secret)
	}
	if !updated.UpdatedAt.Time.After(first.UpdatedAt.Time) {
		t.Fatalf("updated_at did not advance: before=%v after=%v", first.UpdatedAt.Time, updated.UpdatedAt.Time)
	}

	steps := upsert("TEST_CONFIG_ENV", "steps", "steps-value", true)
	if steps.Value != "steps-value" || !steps.Secret {
		t.Fatalf("second target env = {value:%q secret:%v}, want steps-value/true", steps.Value, steps.Secret)
	}
	gates, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_CONFIG_ENV", Target: "gates"})
	if err != nil {
		t.Fatalf("GetGlobalEnv(TEST_CONFIG_ENV/gates) failed: %v", err)
	}
	if gates.Value != "second-value" {
		t.Fatalf("gates target changed after steps upsert: got %q", gates.Value)
	}

	if err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
		Key: "TEST_LIST_A", Target: "server", Value: "a", Secret: false,
	}); err != nil {
		t.Fatalf("UpsertGlobalEnv(TEST_LIST_A/server) failed: %v", err)
	}
	if err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
		Key: "TEST_LIST_A", Target: "gates", Value: "b", Secret: true,
	}); err != nil {
		t.Fatalf("UpsertGlobalEnv(TEST_LIST_A/gates) failed: %v", err)
	}
	envs, err := db.ListGlobalEnv(ctx)
	if err != nil {
		t.Fatalf("ListGlobalEnv() failed: %v", err)
	}
	for i := 1; i < len(envs); i++ {
		prev := envs[i-1].Key + ":" + envs[i-1].Target
		curr := envs[i].Key + ":" + envs[i].Target
		if prev > curr {
			t.Fatalf("ListGlobalEnv not ordered by key,target: %q > %q", prev, curr)
		}
	}

	if err := db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "TEST_CONFIG_ENV", Target: "gates"}); err != nil {
		t.Fatalf("DeleteGlobalEnv(TEST_CONFIG_ENV/gates) failed: %v", err)
	}
	if _, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_CONFIG_ENV", Target: "gates"}); err != pgx.ErrNoRows {
		t.Fatalf("GetGlobalEnv after delete got %v, want pgx.ErrNoRows", err)
	}
	if _, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_CONFIG_ENV", Target: "steps"}); err != nil {
		t.Fatalf("deleting gates target removed steps target: %v", err)
	}
	if err := db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "TEST_CONFIG_ENV", Target: "missing"}); err != nil {
		t.Fatalf("DeleteGlobalEnv missing target failed: %v", err)
	}
}
