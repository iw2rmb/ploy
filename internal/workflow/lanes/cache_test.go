package lanes

import "testing"

func TestComposeCacheKeyProducesDeterministicString(t *testing.T) {
	spec := Spec{
		Name:           "node-wasm",
		RuntimeFamily:  "wasm-node",
		CacheNamespace: "node",
	}
	key, err := ComposeCacheKey(CacheKeyRequest{
		Lane: spec,
		DescribeOptions: DescribeOptions{
			CommitSHA:           "abc123",
			SnapshotFingerprint: "snapshot-456",
			ManifestVersion:     "manifest-1",
			AsterToggles:        []string{"plan", "exec"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "node/node-wasm@commit=abc123@snapshot=snapshot-456@manifest=manifest-1@aster=exec+plan"
	if key != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}

func TestComposeCacheKeyHandlesEmptyOptionalFields(t *testing.T) {
	spec := Spec{Name: "go-fast", CacheNamespace: "go", RuntimeFamily: "go-native"}
	key, err := ComposeCacheKey(CacheKeyRequest{Lane: spec})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "go/go-fast@commit=none@snapshot=none@manifest=none@aster=none"
	if key != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}
