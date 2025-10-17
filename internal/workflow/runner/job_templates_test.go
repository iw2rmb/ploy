package runner

import (
	"context"
	"testing"
)

func TestStaticJobComposerReturnsTemplateCopy(t *testing.T) {
	composer := NewStaticJobComposer()

	job, err := composer.Compose(context.Background(), JobComposeRequest{Stage: Stage{Lane: "mods-plan"}})
	if err != nil {
		t.Fatalf("compose error: %v", err)
	}
	if job.Image != "registry.dev/ploy/mods-plan:latest" {
		t.Fatalf("unexpected image: %s", job.Image)
	}
	if job.Runtime != "docker" {
		t.Fatalf("unexpected runtime: %s", job.Runtime)
	}
	if job.Metadata["runtime_family"] != "mods" {
		t.Fatalf("missing runtime family metadata: %#v", job.Metadata)
	}

	// Ensure the template is cloned rather than referenced.
	job.Env["MODS_PLAN_CACHE"] = "/tmp/cache"
	job.Metadata["priority"] = "high"

	original, err := composer.Compose(context.Background(), JobComposeRequest{Stage: Stage{Lane: "mods-plan"}})
	if err != nil {
		t.Fatalf("compose error: %v", err)
	}
	if original.Env["MODS_PLAN_CACHE"] != "/workspace/cache" {
		t.Fatal("expected template env to remain unchanged")
	}
	if original.Metadata["priority"] != "standard" {
		t.Fatal("expected template metadata to remain unchanged")
	}
}

func TestStaticJobComposerRejectsUnknownLane(t *testing.T) {
	composer := NewStaticJobComposer()
	if _, err := composer.Compose(context.Background(), JobComposeRequest{Stage: Stage{Lane: "unknown"}}); err == nil {
		t.Fatal("expected error for unknown lane")
	}
}
