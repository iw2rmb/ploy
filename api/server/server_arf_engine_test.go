package server

import (
	"testing"
)

// Verifies ARF engine is initialized and attached to service dependencies
func TestServer_ARFEngineInitialized(t *testing.T) {
	t.Parallel()
	srv, err := NewServer(&ControllerConfig{StorageConfigPath: ""})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	if srv == nil || srv.dependencies == nil || srv.dependencies.ARFEngine == nil {
		t.Fatalf("ARFEngine should be initialized and attached to dependencies")
	}
}
