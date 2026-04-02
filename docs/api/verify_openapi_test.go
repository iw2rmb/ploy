package api_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPICompleteness verifies that all implemented endpoints are documented
// in the OpenAPI specification and that the schemas are valid.
func TestOpenAPICompleteness(t *testing.T) {
	// Load OpenAPI spec
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

	// List of endpoints as registered in internal/server/handlers/register.go
	implementedEndpoints := []struct {
		path   string
		method string
	}{
		// Config — Global Env
		{"/v1/config/env", "get"},
		{"/v1/config/env/{key}", "get"},
		{"/v1/config/env/{key}", "put"},
		{"/v1/config/env/{key}", "delete"},
		// PKI
		{"/v1/pki/sign", "post"},
		{"/v1/pki/sign/admin", "post"},
		{"/v1/pki/sign/client", "post"},
		// Runs (single-repo submit + batch lifecycle)
		{"/v1/runs", "post"},
		{"/v1/runs/{id}/logs", "get"},
		// Migs (mig project CRUD)
		{"/v1/migs", "get"},
		{"/v1/migs", "post"},
		{"/v1/migs/{mig_id}", "delete"},
		{"/v1/migs/{mig_id}/archive", "patch"},
		{"/v1/migs/{mig_id}/unarchive", "patch"},
		{"/v1/migs/{mig_id}/runs", "post"},
		// Batch runs lifecycle
		{"/v1/runs", "get"},
		{"/v1/runs/{id}", "get"},
		{"/v1/runs/{id}/status", "get"},
		{"/v1/runs/{id}/cancel", "post"},
		// RunRepo handlers (repos within a batch)
		{"/v1/runs/{id}/repos", "get"},
		{"/v1/runs/{id}/repos", "post"},
		{"/v1/runs/{id}/repos/{repo_id}/restart", "post"},
		// v1 repo-scoped endpoints
		{"/v1/runs/{run_id}/repos/{repo_id}/diffs", "get"},
		{"/v1/runs/{run_id}/repos/{repo_id}/logs", "get"},
		{"/v1/runs/{run_id}/repos/{repo_id}/artifacts", "get"},
		{"/v1/runs/{run_id}/repos/{repo_id}/cancel", "post"},
		{"/v1/runs/{run_id}/pull", "post"},
		// Repo-centric endpoints
		{"/v1/repos", "get"},
		{"/v1/repos/{repo_id}/runs", "get"},
		// Node heartbeat
		{"/v1/nodes/{id}/heartbeat", "post"},
		// Node management
		{"/v1/nodes", "get"},
		{"/v1/nodes/{id}/drain", "post"},
		{"/v1/nodes/{id}/undrain", "post"},
		// Node claim (also handles run status transition and SSE event publishing;
		// the separate /v1/nodes/{id}/ack endpoint has been removed)
		{"/v1/nodes/{id}/claim", "post"},
		// Job-level completion (node-based /v1/nodes/{id}/complete has been removed)
		{"/v1/jobs/{job_id}/complete", "post"},
		// Job-level status polling for worker-side cancellation detection
		{"/v1/jobs/{job_id}/status", "get"},
		// Job-level runtime image persistence
		{"/v1/jobs/{job_id}/image", "post"},
		// Node events
		{"/v1/nodes/{id}/events", "post"},
		// Node logs
		{"/v1/nodes/{id}/logs", "post"},
		// Job diff upload (run-scoped, no node ID)
		{"/v1/runs/{run_id}/jobs/{job_id}/diff", "post"},
		// Job artifact upload (run-scoped, no node ID)
		{"/v1/runs/{run_id}/jobs/{job_id}/artifact", "post"},
	}

	// Verify each implemented endpoint is documented
	for _, ep := range implementedEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			pathItem, ok := paths[ep.path]
			if !ok {
				t.Errorf("endpoint %s not found in OpenAPI spec", ep.path)
				return
			}

			pathMap, ok := pathItem.(map[string]interface{})
			if !ok {
				t.Errorf("invalid path item for %s", ep.path)
				return
			}

			// Check if it's a $ref - resolve it
			var methodsMap map[string]interface{}
			if ref, ok := pathMap["$ref"].(string); ok {
				// Load the referenced file
				refPath := filepath.Join(".", ref)
				refData, err := os.ReadFile(refPath)
				if err != nil {
					t.Errorf("failed to read referenced file %s: %v", refPath, err)
					return
				}

				if err := yaml.Unmarshal(refData, &methodsMap); err != nil {
					t.Errorf("failed to parse referenced file %s: %v", refPath, err)
					return
				}
			} else {
				// Direct definition (not a $ref)
				methodsMap = pathMap
			}

			// Check if method is documented
			if _, ok := methodsMap[ep.method]; !ok {
				t.Errorf("method %s not documented for path %s", ep.method, ep.path)
			}
		})
	}

	// Verify critical schemas exist
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
		"PKISignRequest",
		"PKISignResponse",
		"PKIAdminSignRequest",
		"PKIClientSignRequest",
		"Run",
		"RunSummary",
		"RunRepoCounts",
		"RunRepoStatus",         // Per-repo execution status enum.
		"RunBatchDerivedStatus", // Batch-level aggregate status enum.
		"CreateRunRequest",
		"CreateRunResponse",
		"RunSubmitRequest",
		"MigsRunSummary", // Canonical Migs run status schema (POST/GET /v1/migs responses, SSE events).
		"StageStatus",    // Job execution state within RunSummary.stages map.
		"NodeClaimResponse",
		"Event",
		"Stage",
		"RunRepo",
		"RepoSummary",
		"RepoRunSummary",
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
		"components/schemas/pki.yaml",
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

// TestPKISignRequestNodeIDShape ensures PKISignRequest.node_id matches node ID semantics
// and is not documented as a UUID.
func TestPKISignRequestNodeIDShape(t *testing.T) {
	path := filepath.Join(".", "components", "schemas", "pki.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var content map[string]interface{}
	if err := yaml.Unmarshal(data, &content); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	rawSchema, ok := content["PKISignRequest"]
	if !ok {
		t.Fatalf("PKISignRequest schema not found in %s", path)
	}

	schema, ok := rawSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("PKISignRequest schema has unexpected type %T", rawSchema)
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("PKISignRequest.properties has unexpected type %T", schema["properties"])
	}

	rawNodeID, ok := props["node_id"]
	if !ok {
		t.Fatalf("PKISignRequest.properties.node_id not found")
	}

	nodeID, ok := rawNodeID.(map[string]interface{})
	if !ok {
		t.Fatalf("PKISignRequest.properties.node_id has unexpected type %T", rawNodeID)
	}

	if typ, _ := nodeID["type"].(string); typ != "string" {
		t.Fatalf("PKISignRequest.node_id.type = %q, want %q", typ, "string")
	}

	if format, ok := nodeID["format"]; ok {
		t.Fatalf("PKISignRequest.node_id.format = %v, want no explicit format (NanoID string, not UUID)", format)
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
				// Extract file path from $ref (e.g., './paths/pki_sign.yaml')
				refPath := filepath.Join(".", ref)
				if _, err := os.Stat(refPath); err != nil {
					t.Errorf("referenced file %s does not exist", refPath)
				}
			}
		})
	}
}
