package orchestration

import (
	"os"
	"strings"
	"testing"
)

func TestRenderTemplate_UsesDockerLane(t *testing.T) {
	data := RenderData{App: "myapp", DockerImage: "registry.dev/myapp:latest", Lane: "D"}
	outPath, err := RenderTemplate("d", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outPath) })

	rendered, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read rendered template: %v", err)
	}
	out := string(rendered)
	if !strings.Contains(out, "job \"myapp-lane-d\"") {
		t.Fatalf("expected docker lane template, got: %s", out)
	}
	if !strings.Contains(out, "driver = \"docker\"") {
		t.Fatalf("expected docker driver in template, got: %s", out)
	}
}
