package main

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/handlers"
)

func TestTrinoConfigReadsProjectedServerCredentials(t *testing.T) {
	t.Setenv("PLOY_TRINO_USER", "process-user")
	t.Setenv("PLOY_TRINO_PASSWORD", "process-password")
	applyServerTargetEnv(map[string][]handlers.GlobalEnvVar{
		"PLOY_TRINO_USER": {
			{Value: "server-user", Target: domaintypes.GlobalEnvTargetServer, Secret: true},
		},
		"PLOY_TRINO_PASSWORD": {
			{Value: " server-password ", Target: domaintypes.GlobalEnvTargetServer, Secret: true},
			{Value: "steps-password", Target: domaintypes.GlobalEnvTargetSteps, Secret: true},
		},
	})
	got := config.LoadTrinoFromEnv()
	if got.User != "server-user" || got.Password != " server-password " {
		t.Fatal("Trino config did not use the projected server credentials")
	}
}
