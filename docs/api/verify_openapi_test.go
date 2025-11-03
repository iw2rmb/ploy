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

	// List of endpoints implemented in cmd/ployd/server.go (as of ROADMAP line 99)
	implementedEndpoints := []struct {
		path   string
		method string
	}{
		// PKI
		{"/v1/pki/sign", "post"},
		{"/v1/pki/sign/admin", "post"},
		{"/v1/pki/sign/client", "post"},
		// Repos
		{"/v1/repos", "post"},
		{"/v1/repos", "get"},
		{"/v1/repos/{id}", "get"},
		{"/v1/repos/{id}", "delete"},
		// Mods (Ticket submit + status/events)
		{"/v1/mods", "post"},
		{"/v1/mods/{id}", "get"},
		{"/v1/mods/{id}/events", "get"},
		{"/v1/mods/{id}/artifact_bundles", "post"},
		// Runs (legacy write/management endpoints only; read/stream moved to /v1/mods)
		{"/v1/runs/{id}", "delete"},
		{"/v1/runs/{id}/timing", "get"},
		{"/v1/runs/{id}/logs", "post"},
		{"/v1/runs/{id}/diffs", "post"},
		{"/v1/runs/{id}/artifact_bundles", "post"},
		// Node heartbeat
		{"/v1/nodes/{id}/heartbeat", "post"},
		// Node management
		{"/v1/nodes", "get"},
		{"/v1/nodes/{id}/drain", "post"},
		{"/v1/nodes/{id}/undrain", "post"},
		// Node claim
		{"/v1/nodes/{id}/claim", "post"},
		// Node ack
		{"/v1/nodes/{id}/ack", "post"},
		// Node complete
		{"/v1/nodes/{id}/complete", "post"},
		// Node events
		{"/v1/nodes/{id}/events", "post"},
		// Node logs
		{"/v1/nodes/{id}/logs", "post"},
		// Node diff
		{"/v1/nodes/{id}/stage/{stage}/diff", "post"},
		// Node artifact
		{"/v1/nodes/{id}/stage/{stage}/artifact", "post"},
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
		"PKISignRequest",
		"PKISignResponse",
		"PKIAdminSignRequest",
		"PKIClientSignRequest",
		"Repo",
		"CreateRepoRequest",
		"Run",
		"CreateRunRequest",
		"CreateRunResponse",
		"TicketSubmitRequest",
		"TicketSummary",
		"TicketStatus",
		"NodeClaimResponse",
		"Event",
		"Stage",
	}

	for _, schema := range requiredSchemas {
		t.Run("schema:"+schema, func(t *testing.T) {
			if _, ok := schemas[schema]; !ok {
				t.Errorf("schema %s not found in OpenAPI spec", schema)
			}
		})
	}
}

// TestSchemaFilesValid verifies that all schema files are valid YAML.
func TestSchemaFilesValid(t *testing.T) {
	schemaFiles := []string{
		"components/schemas/common.yaml",
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
