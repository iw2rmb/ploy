package nodeagent

import "testing"

func TestLocalDeployNodeConfigParses(t *testing.T) {
	cfg, err := LoadConfig("../../local/node/ployd-node.yaml")
	if err != nil {
		t.Fatalf("LoadConfig(local/node/ployd-node.yaml): %v", err)
	}
	if got := cfg.NodeID.String(); len(got) != 6 {
		t.Fatalf("config node_id %q length = %d, want 6", got, len(got))
	}
}
