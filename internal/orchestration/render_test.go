package orchestration

import (
	"os"
	"strings"
	"testing"
)

// TestRenderTemplate_IgnoresTemplateDir ensures embedded templates are always used
func TestRenderTemplate_IgnoresTemplateDir(t *testing.T) {
	t.Setenv("PLOY_TEMPLATE_DIR", t.TempDir())

	data := RenderData{App: "myapp", ImagePath: "/tmp/image", DockerImage: "busybox"}
	outPath, err := RenderTemplate("c", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "traefik.http.routers.myapp-c.rule=Host(`myapp-c.dev.ployd.app`)") {
		t.Fatalf("expected embedded lane C template content, got: %s", s)
	}
}

func TestRenderKanikoBuilder_Defaults(t *testing.T) {
	t.Setenv("PLOY_KANIKO_IMAGE", "")
	t.Setenv("PLOY_CONTROLLER", "")
	t.Setenv("PLOY_PLATFORM_DOMAIN", "")

	path, err := RenderKanikoBuilder("demo", "v1", "registry.dev/demo:latest", "https://context.example.com/archive.tar", "", "go")
	if err != nil {
		t.Fatalf("RenderKanikoBuilder failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read kaniko render output: %v", err)
	}
	out := string(content)
	if !strings.Contains(out, "DOCKERFILE_PATH = \"Dockerfile\"") {
		t.Fatalf("expected default Dockerfile path, got: %s", out)
	}
	if !strings.Contains(out, "image = \"gcr.io/kaniko-project/executor:debug\"") {
		t.Fatalf("expected default kaniko image, got: %s", out)
	}
	if strings.Contains(out, "memory = 2048") {
		t.Fatalf("expected memory override to remove 2048 default, got: %s", out)
	}
	if !strings.Contains(out, "memory = 512") {
		t.Fatalf("expected memory override to 512MB, got: %s", out)
	}
}

func TestRenderKanikoBuilder_LanguageOverrides(t *testing.T) {
	t.Setenv("PLOY_KANIKO_MEMORY_MB", "512")
	t.Setenv("PLOY_KANIKO_MEMORY_DOTNET_MB", "3072")
	t.Setenv("PLOY_CONTROLLER", "https://api.dev.ployman.app/v1")

	path, err := RenderKanikoBuilder("sample", "20240101", "registry.dev/sample:latest", "https://context.example.com/src.tar", "Dockerfile", ".NET")
	if err != nil {
		t.Fatalf("RenderKanikoBuilder failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read kaniko render output: %v", err)
	}
	out := string(content)
	if !strings.Contains(out, "image = \"registry.dev.ployman.app/kaniko-executor:debug\"") {
		t.Fatalf("expected internal registry kaniko image, got: %s", out)
	}
	if !strings.Contains(out, "memory = 3072") {
		t.Fatalf("expected dotnet memory bump to 3072MB, got: %s", out)
	}
	if strings.Contains(out, "memory = 512") {
		t.Fatalf("unexpected base memory value lingering in output: %s", out)
	}
}

func TestRenderTemplate_SelectsDistrolessRunnerForLaneG(t *testing.T) {
	t.Setenv("PLOY_WASM_DISTROLESS", "1")

	data := RenderData{
		App:              "wasmapp",
		Lane:             "G",
		WasmRuntimeImage: "registry.dev/runner:latest",
		WasmModuleURL:    "https://filer.dev/wasmapp/module.wasm",
	}
	path, err := RenderTemplate("g", data)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	rendered, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read rendered template: %v", err)
	}
	out := string(rendered)
	if !strings.Contains(out, "image = \"registry.dev/runner:latest\"") {
		t.Fatalf("expected distroless runner image in template, got: %s", out)
	}
	if !strings.Contains(out, "entrypoint = [\"/runner\"]") {
		t.Fatalf("expected runner entrypoint for distroless template, got: %s", out)
	}
}
