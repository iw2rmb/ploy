package step

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("PLOY_CONTAINER_REGISTRY") == "" {
		_ = os.Setenv("PLOY_CONTAINER_REGISTRY", "ghcr.io/iw2rmb/ploy")
	}
	os.Exit(m.Run())
}
