package nodeagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNodeConfigParses(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "ployd-node.yaml")
	if err := os.WriteFile(configPath, []byte(`server_url: "http://server:8080"
node_id: "local1"
cluster_id: "local-runtime"

http:
  listen: ":8444"

heartbeat:
  interval: 30s
  timeout: 10s
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig(%s): %v", configPath, err)
	}
	if got := cfg.NodeID.String(); len(got) != 6 {
		t.Fatalf("config node_id %q length = %d, want 6", got, len(got))
	}
}
