package catalog

import (
	"context"
	"strings"
	"testing"

	providers_memory "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

func TestStorageBackedRegistry_ListAndGet(t *testing.T) {
	t.Parallel()

	mem := providers_memory.NewMemoryStorage(0)
	catalog := `[
      {"id":"org.openrewrite.java.cleanup.Cleanup","display_name":"Java Cleanup","description":"Cleanup rules","tags":["cleanup","java"],"pack":"rewrite-java","version":"1.2.3"},
      {"id":"org.openrewrite.java.format.AutoFormat","display_name":"Auto Format","description":"Formatting","tags":["format","java"],"pack":"rewrite-java","version":"1.0.0"}
    ]`
	if err := mem.Put(context.Background(), catalogKey, strings.NewReader(catalog)); err != nil {
		t.Fatalf("put catalog: %v", err)
	}

	reg := NewStorageBacked(mem)
	if reg == nil {
		t.Fatalf("expected registry instance")
	}

	// List with tag filter
	list, err := reg.List(context.Background(), Filters{Tag: "cleanup"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 recipe with tag cleanup, got %d", len(list))
	}

	// Get by id
	rec, err := reg.Get(context.Background(), "org.openrewrite.java.cleanup.Cleanup")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec == nil || rec.ID == "" {
		t.Fatalf("expected non-nil recipe")
	}
}
