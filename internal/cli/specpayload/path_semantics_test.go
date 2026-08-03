package specpayload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSpecPayload_IncludeAndRefPathsRemainLiteral(t *testing.T) {
	t.Setenv("PLOY_LITERAL_SPEC_PATH", "redirected.yaml")

	tests := []struct {
		name        string
		path        string
		literalPath string
		ref         bool
	}{
		{name: "include environment placeholder", path: "$PLOY_LITERAL_SPEC_PATH", literalPath: "$PLOY_LITERAL_SPEC_PATH"},
		{name: "include home prefix", path: "~/fragment.yaml", literalPath: "~/fragment.yaml"},
		{name: "ref environment placeholder", path: "$PLOY_LITERAL_SPEC_PATH", literalPath: "$PLOY_LITERAL_SPEC_PATH", ref: true},
		{name: "ref home prefix", path: "~/fragment.yaml", literalPath: "~/fragment.yaml", ref: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			literalPath := filepath.Join(dir, test.literalPath)
			if err := os.MkdirAll(filepath.Dir(literalPath), 0o755); err != nil {
				t.Fatalf("create literal path directory: %v", err)
			}
			rootSpec := "steps:\n  - !include " + test.path + "\n"
			literalSpec := "name: rewrite\nimage: docker.io/test/literal:latest\n"
			if test.ref {
				rootSpec = "steps:\n  - ref: " + test.path + ":rewrite\n"
				literalSpec = "steps:\n  - name: rewrite\n    image: docker.io/test/literal:latest\n"
			}
			writeFile(t, literalPath, literalSpec)
			writeFile(t, filepath.Join(dir, "redirected.yaml"), "steps:\n  - name: rewrite\n    image: docker.io/test/redirected:latest\n")

			result := buildAndParseSpec(t, dir, rootSpec, ".yaml", specPayloadOpts{})
			steps := mustSteps(t, result, 1)
			assertField(t, steps[0], "image", "docker.io/test/literal:latest")
		})
	}
}
