package api_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPIRootSchemas verifies that shared API schemas remain discoverable
// from the root document. Runtime route registration is checked in handlers.
func TestOpenAPIRootSchemas(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "OpenAPI.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI.yaml: %v", err)
	}

	var spec map[string]interface{}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse OpenAPI.yaml: %v", err)
	}

	components, ok := spec["components"].(map[string]interface{})
	if !ok {
		t.Fatal("components not found in OpenAPI.yaml")
	}

	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		t.Fatal("schemas not found in components")
	}

	requiredSchemas := []string{
		"GlobalEnvTarget",
		"GlobalEnvListItem",
		"GlobalEnvVar",
		"GlobalEnvPutRequest",
		"Run",
		"RunSummary",
		"RunCounts",
		"CreateRunRequest",
		"CreateRunResponse",
		"RunSubmitRequest",
		"RunSpecOverrides",
		"RunBuildGateForcedStack",
		"RunRestartRequest",
		"MigsRunSummary", // Canonical Migs run status schema (POST/GET /v1/migs responses, SSE events).
		"StageStatus",    // Job execution state within RunSummary.stages map.
		"NodeClaimResponse",
		"Event",
		"Stage",
		"RepoSummary",
		"RepoRunSummary",
		"NamedSpecCatalogEntry",
		"NamedSpecListResponse",
	}

	for _, schema := range requiredSchemas {
		t.Run("schema:"+schema, func(t *testing.T) {
			if _, ok := schemas[schema]; !ok {
				t.Errorf("schema %s not found in OpenAPI spec", schema)
			}
		})
	}
}

// TestConfigEnvKeyResponseSemantics verifies that the OpenAPI contract for
// /v1/config/env/{key} retains ambiguity (409) and target-validation (400)
// response codes on GET and DELETE operations.
func TestConfigEnvKeyResponseSemantics(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "paths", "config_env_key.yaml"))
	if err != nil {
		t.Fatalf("read config_env_key.yaml: %v", err)
	}

	var spec map[string]interface{}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse config_env_key.yaml: %v", err)
	}

	methods := []struct {
		method string
		codes  []string
	}{
		{"get", []string{"400", "409"}},
		{"delete", []string{"400", "409"}},
	}

	for _, m := range methods {
		t.Run(m.method, func(t *testing.T) {
			op, ok := spec[m.method].(map[string]interface{})
			if !ok {
				t.Fatalf("method %s not found in config_env_key.yaml", m.method)
			}

			responses, ok := op["responses"].(map[string]interface{})
			if !ok {
				t.Fatalf("responses not found for method %s", m.method)
			}

			for _, code := range m.codes {
				t.Run("status_"+code, func(t *testing.T) {
					if _, ok := responses[code]; !ok {
						t.Errorf("%s /v1/config/env/{key} missing response %s", m.method, code)
					}
				})
			}
		})
	}
}

// TestSchemaFilesValid verifies that all schema files are valid YAML.
func TestSchemaFilesValid(t *testing.T) {
	schemaFiles := []string{
		"components/schemas/common.yaml",
		"components/schemas/config.yaml",
		"components/schemas/controlplane.yaml",
	}

	for _, file := range schemaFiles {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join(".", file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}

			var content map[string]interface{}
			if err := yaml.Unmarshal(data, &content); err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}

			if len(content) == 0 {
				t.Errorf("%s is empty", file)
			}
		})
	}
}

// TestPathFilesExist verifies that all path files referenced in OpenAPI.yaml exist.
func TestPathFilesExist(t *testing.T) {
	specPath := filepath.Join(".", "OpenAPI.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read OpenAPI.yaml: %v", err)
	}

	var spec map[string]interface{}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse OpenAPI.yaml: %v", err)
	}

	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		t.Fatal("paths not found in OpenAPI.yaml")
	}

	// Check each path reference
	for path, pathItem := range paths {
		t.Run(path, func(t *testing.T) {
			pathMap, ok := pathItem.(map[string]interface{})
			if !ok {
				t.Skipf("path %s has direct definition (not a $ref)", path)
				return
			}

			// Check if it's a $ref
			if ref, ok := pathMap["$ref"].(string); ok {
				// Extract the path file from the reference.
				refPath := filepath.Join(".", ref)
				if _, err := os.Stat(refPath); err != nil {
					t.Errorf("referenced file %s does not exist", refPath)
				}
			}
		})
	}
}
