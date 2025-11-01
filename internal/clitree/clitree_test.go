package clitree

import "testing"

func TestTreeBasicShape(t *testing.T) {
	nodes := Tree()
	if len(nodes) == 0 {
		t.Fatalf("expected non-empty CLI tree")
	}
	// Expect some well-known top-level commands to exist.
	want := map[string]bool{"mod": false, "mods": false, "jobs": false, "server": false, "node": false}
	for _, n := range nodes {
		if _, ok := want[n.Name]; ok {
			want[n.Name] = true
		}
	}
	for name, ok := range want {
		if !ok {
			t.Fatalf("missing top-level command %s", name)
		}
	}
}
