package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEnvFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("basic file read", func(t *testing.T) {
		testPath := filepath.Join(tmpDir, "test.txt")
		testContent := "secret-content-123"
		if err := os.WriteFile(testPath, []byte(testContent), 0o644); err != nil {
			t.Fatalf("write test file: %v", err)
		}

		content, err := resolveEnvFromFile(testPath)
		if err != nil {
			t.Fatalf("resolveEnvFromFile error: %v", err)
		}
		if content != testContent {
			t.Errorf("expected content %q, got %q", testContent, content)
		}
	})

	t.Run("tilde expansion", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("skip tilde test: cannot get home dir: %v", err)
		}

		// Create a test file in a temp subdir of home (if writable)
		testSubdir := filepath.Join(home, ".ploy-test-"+filepath.Base(tmpDir))
		if err := os.MkdirAll(testSubdir, 0o755); err != nil {
			t.Skipf("skip tilde test: cannot create test dir in home: %v", err)
		}
		defer func() {
			_ = os.RemoveAll(testSubdir)
		}()

		testFile := filepath.Join(testSubdir, "auth.json")
		testContent := `{"token":"xyz"}`
		if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
			t.Skipf("skip tilde test: cannot write test file: %v", err)
		}

		// Use ~ path
		tildeRelPath := filepath.Join(".ploy-test-"+filepath.Base(tmpDir), "auth.json")
		tildePath := "~/" + tildeRelPath

		content, err := resolveEnvFromFile(tildePath)
		if err != nil {
			t.Fatalf("resolveEnvFromFile with ~ error: %v", err)
		}
		if content != testContent {
			t.Errorf("expected content %q, got %q", testContent, content)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := resolveEnvFromFile("/nonexistent/path/file.txt")
		if err == nil {
			t.Errorf("expected error for nonexistent file")
		}
		// Ensure error message does not leak sensitive path details
		errMsg := err.Error()
		if !strings.Contains(errMsg, "path redacted") {
			t.Errorf("expected redacted error message, got: %s", errMsg)
		}
	})
}

func TestResolveEnvFromFileInPlace(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("env_from_file only", func(t *testing.T) {
		file1 := filepath.Join(tmpDir, "secret1.txt")
		file2 := filepath.Join(tmpDir, "secret2.txt")
		if err := os.WriteFile(file1, []byte("content1"), 0o644); err != nil {
			t.Fatalf("write file1: %v", err)
		}
		if err := os.WriteFile(file2, []byte("content2"), 0o644); err != nil {
			t.Fatalf("write file2: %v", err)
		}

		spec := map[string]any{
			"env_from_file": map[string]any{
				"KEY1": file1,
				"KEY2": file2,
			},
		}

		if err := resolveEnvFromFileInPlace(spec); err != nil {
			t.Fatalf("resolveEnvFromFileInPlace error: %v", err)
		}

		// env_from_file should be removed
		if _, ok := spec["env_from_file"]; ok {
			t.Errorf("expected env_from_file to be removed after resolution")
		}

		// env should contain resolved values
		env, ok := spec["env"].(map[string]any)
		if !ok {
			t.Fatalf("expected env map in spec")
		}
		if env["KEY1"] != "content1" {
			t.Errorf("expected env.KEY1=content1, got %v", env["KEY1"])
		}
		if env["KEY2"] != "content2" {
			t.Errorf("expected env.KEY2=content2, got %v", env["KEY2"])
		}
	})

	t.Run("env with inline from_file", func(t *testing.T) {
		file1 := filepath.Join(tmpDir, "inline.txt")
		if err := os.WriteFile(file1, []byte("inline-content"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		spec := map[string]any{
			"env": map[string]any{
				"LITERAL": "literal-value",
				"FROM_FILE": map[string]any{
					"from_file": file1,
				},
			},
		}

		if err := resolveEnvFromFileInPlace(spec); err != nil {
			t.Fatalf("resolveEnvFromFileInPlace error: %v", err)
		}

		env, ok := spec["env"].(map[string]any)
		if !ok {
			t.Fatalf("expected env map in spec")
		}
		if env["LITERAL"] != "literal-value" {
			t.Errorf("expected env.LITERAL=literal-value, got %v", env["LITERAL"])
		}
		if env["FROM_FILE"] != "inline-content" {
			t.Errorf("expected env.FROM_FILE=inline-content, got %v", env["FROM_FILE"])
		}
	})

	t.Run("both env_from_file and env with mixed values", func(t *testing.T) {
		file1 := filepath.Join(tmpDir, "mixed1.txt")
		file2 := filepath.Join(tmpDir, "mixed2.txt")
		if err := os.WriteFile(file1, []byte("mixed-content1"), 0o644); err != nil {
			t.Fatalf("write file1: %v", err)
		}
		if err := os.WriteFile(file2, []byte("mixed-content2"), 0o644); err != nil {
			t.Fatalf("write file2: %v", err)
		}

		spec := map[string]any{
			"env_from_file": map[string]any{
				"KEY_FROM_FILE": file1,
			},
			"env": map[string]any{
				"KEY_LITERAL": "literal",
				"KEY_INLINE":  map[string]any{"from_file": file2},
			},
		}

		if err := resolveEnvFromFileInPlace(spec); err != nil {
			t.Fatalf("resolveEnvFromFileInPlace error: %v", err)
		}

		// env_from_file should be removed
		if _, ok := spec["env_from_file"]; ok {
			t.Errorf("expected env_from_file to be removed")
		}

		// env should contain all merged values
		env, ok := spec["env"].(map[string]any)
		if !ok {
			t.Fatalf("expected env map in spec")
		}
		if env["KEY_FROM_FILE"] != "mixed-content1" {
			t.Errorf("expected env.KEY_FROM_FILE=mixed-content1, got %v", env["KEY_FROM_FILE"])
		}
		if env["KEY_LITERAL"] != "literal" {
			t.Errorf("expected env.KEY_LITERAL=literal, got %v", env["KEY_LITERAL"])
		}
		if env["KEY_INLINE"] != "mixed-content2" {
			t.Errorf("expected env.KEY_INLINE=mixed-content2, got %v", env["KEY_INLINE"])
		}
	})

	t.Run("invalid from_file map structure", func(t *testing.T) {
		spec := map[string]any{
			"env": map[string]any{
				"BAD_KEY": map[string]any{
					"invalid": "structure",
				},
			},
		}

		err := resolveEnvFromFileInPlace(spec)
		if err == nil {
			t.Errorf("expected error for invalid from_file structure")
		}
	})

	t.Run("env value with unsupported type", func(t *testing.T) {
		spec := map[string]any{
			"env": map[string]any{
				"BAD_KEY": 123, // int instead of string or map
			},
		}

		err := resolveEnvFromFileInPlace(spec)
		if err == nil {
			t.Errorf("expected error for unsupported env value type")
		}
	})

	t.Run("env_from_file with non-string path", func(t *testing.T) {
		spec := map[string]any{
			"env_from_file": map[string]any{
				"BAD_KEY": 123, // int instead of string path
			},
		}

		err := resolveEnvFromFileInPlace(spec)
		if err == nil {
			t.Errorf("expected error for non-string path in env_from_file")
		}
	})
}

func TestBuildSpecPayloadWithEnvFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("top-level env_from_file in spec file", func(t *testing.T) {
		authFile := filepath.Join(tmpDir, "auth.json")
		authContent := `{"token":"secret-xyz"}`
		if err := os.WriteFile(authFile, []byte(authContent), 0o644); err != nil {
			t.Fatalf("write auth file: %v", err)
		}

		specPath := filepath.Join(tmpDir, "spec.yaml")
		specContent := `
steps:
  - image: docker.io/test/mig:latest
env:
  LITERAL_KEY: literal-value
env_from_file:
  AUTH_JSON: ` + authFile + `
`
		if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
			t.Fatalf("write spec file: %v", err)
		}

		payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
		if err != nil {
			t.Fatalf("buildSpecPayload error: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(payload, &result); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}

		// env_from_file should be removed
		if _, ok := result["env_from_file"]; ok {
			t.Errorf("expected env_from_file to be removed from final payload")
		}

		// env should contain both literal and file-resolved values
		env, ok := result["env"].(map[string]any)
		if !ok {
			t.Fatalf("expected env map in result")
		}
		if env["LITERAL_KEY"] != "literal-value" {
			t.Errorf("expected env.LITERAL_KEY=literal-value, got %v", env["LITERAL_KEY"])
		}
		if env["AUTH_JSON"] != authContent {
			t.Errorf("expected env.AUTH_JSON=%q, got %v", authContent, env["AUTH_JSON"])
		}
	})

	t.Run("inline from_file in env", func(t *testing.T) {
		secretFile := filepath.Join(tmpDir, "secret.txt")
		secretContent := "super-secret"
		if err := os.WriteFile(secretFile, []byte(secretContent), 0o644); err != nil {
			t.Fatalf("write secret file: %v", err)
		}

		specPath := filepath.Join(tmpDir, "spec-inline.yaml")
		specContent := `
steps:
  - image: docker.io/test/mig:latest
env:
  LITERAL_KEY: literal-value
  SECRET_KEY:
    from_file: ` + secretFile + `
`
		if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
			t.Fatalf("write spec file: %v", err)
		}

		payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
		if err != nil {
			t.Fatalf("buildSpecPayload error: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(payload, &result); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}

		env, ok := result["env"].(map[string]any)
		if !ok {
			t.Fatalf("expected env map in result")
		}
		if env["LITERAL_KEY"] != "literal-value" {
			t.Errorf("expected env.LITERAL_KEY=literal-value, got %v", env["LITERAL_KEY"])
		}
		if env["SECRET_KEY"] != secretContent {
			t.Errorf("expected env.SECRET_KEY=%q, got %v", secretContent, env["SECRET_KEY"])
		}
	})

	t.Run("env_from_file in build_gate_healing (flattened)", func(t *testing.T) {
		healingAuthFile := filepath.Join(tmpDir, "healing-auth.json")
		healingAuthContent := `{"healing":"token"}`
		if err := os.WriteFile(healingAuthFile, []byte(healingAuthContent), 0o644); err != nil {
			t.Fatalf("write healing auth file: %v", err)
		}

		specPath := filepath.Join(tmpDir, "spec-healing.yaml")
		specContent := `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: docker.io/test/healer:latest
        env:
          HEALER_KEY: literal
        env_from_file:
          HEALER_AUTH: ` + healingAuthFile + `
  router:
    image: docker.io/test/router:latest
`
		if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
			t.Fatalf("write spec file: %v", err)
		}

		payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
		if err != nil {
			t.Fatalf("buildSpecPayload error: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(payload, &result); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}

		// Navigate to build_gate.healing (flattened — no "mig" nesting)
		buildGate, ok := result["build_gate"].(map[string]any)
		if !ok {
			t.Fatalf("expected build_gate in result")
		}
		healing, ok := buildGate["healing"].(map[string]any)
		if !ok {
			t.Fatalf("expected build_gate.healing in result")
		}

		byErrorKind, ok := healing["by_error_kind"].(map[string]any)
		if !ok {
			t.Fatalf("expected build_gate.healing.by_error_kind in result")
		}
		infra, ok := byErrorKind["infra"].(map[string]any)
		if !ok {
			t.Fatalf("expected build_gate.healing.by_error_kind.infra in result")
		}

		// env_from_file should be removed from healing action
		if _, ok := infra["env_from_file"]; ok {
			t.Errorf("expected env_from_file to be removed from healing action")
		}

		// env should contain merged values
		env, ok := infra["env"].(map[string]any)
		if !ok {
			t.Fatalf("expected env map in healing action")
		}
		if env["HEALER_KEY"] != "literal" {
			t.Errorf("expected env.HEALER_KEY=literal, got %v", env["HEALER_KEY"])
		}
		if env["HEALER_AUTH"] != healingAuthContent {
			t.Errorf("expected env.HEALER_AUTH=%q, got %v", healingAuthContent, env["HEALER_AUTH"])
		}
	})

	t.Run("env_from_file with nonexistent file", func(t *testing.T) {
		specPath := filepath.Join(tmpDir, "spec-bad.yaml")
		specContent := `
steps:
  - image: docker.io/test/mig:latest
env_from_file:
  BAD_KEY: /nonexistent/path/file.txt
`
		if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
			t.Fatalf("write spec file: %v", err)
		}

		_, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
		if err == nil {
			t.Errorf("expected error for nonexistent env file")
		}
		// Ensure error is informative
		if !strings.Contains(err.Error(), "resolve env from file") {
			t.Errorf("expected error to mention env file resolution, got: %v", err)
		}
	})

	t.Run("tilde expansion in env_from_file", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("skip tilde test: cannot get home dir: %v", err)
		}

		// Create a test file in home
		testSubdir := filepath.Join(home, ".ploy-test-tilde-"+filepath.Base(tmpDir))
		if err := os.MkdirAll(testSubdir, 0o755); err != nil {
			t.Skipf("skip tilde test: cannot create test dir in home: %v", err)
		}
		defer func() {
			_ = os.RemoveAll(testSubdir)
		}()

		testFile := filepath.Join(testSubdir, "tilde-auth.json")
		testContent := `{"tilde":"token"}`
		if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
			t.Skipf("skip tilde test: cannot write test file: %v", err)
		}

		tildeRelPath := filepath.Join(".ploy-test-tilde-"+filepath.Base(tmpDir), "tilde-auth.json")
		tildePath := "~/" + tildeRelPath

		specPath := filepath.Join(tmpDir, "spec-tilde.yaml")
		specContent := `
steps:
  - image: docker.io/test/mig:latest
env_from_file:
  TILDE_AUTH: ` + tildePath + `
`
		if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
			t.Fatalf("write spec file: %v", err)
		}

		payload, err := buildSpecPayload(specPath, nil, "", false, "", "", "", false, false)
		if err != nil {
			t.Fatalf("buildSpecPayload error: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(payload, &result); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}

		env, ok := result["env"].(map[string]any)
		if !ok {
			t.Fatalf("expected env map in result")
		}
		if env["TILDE_AUTH"] != testContent {
			t.Errorf("expected env.TILDE_AUTH=%q, got %v", testContent, env["TILDE_AUTH"])
		}
	})
}
