package transflow

import (
    "encoding/json"
    "fmt"
    "path/filepath"

    jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// validateJSONAgainstSchema validates a JSON document against a schema file path (relative to repo root)
func validateJSONAgainstSchema(data []byte, schemaRelPath string) error {
    abs, err := filepath.Abs(schemaRelPath)
    if err != nil { return fmt.Errorf("resolve schema path: %w", err) }
    compiler := jsonschema.NewCompiler()
    if err := compiler.AddResource("schema.json", mustFileURL(abs)); err != nil {
        return fmt.Errorf("add schema: %w", err)
    }
    sch, err := compiler.Compile("schema.json")
    if err != nil { return fmt.Errorf("compile schema: %w", err) }
    var v any
    if err := json.Unmarshal(data, &v); err != nil {
        return fmt.Errorf("invalid json: %w", err)
    }
    if err := sch.Validate(v); err != nil {
        return fmt.Errorf("schema validation failed: %w", err)
    }
    return nil
}

func mustFileURL(abs string) string {
    // Construct a file:// URL for the schema path
    return "file://" + abs
}

func validatePlanJSON(data []byte) error {
    return validateJSONAgainstSchema(data, filepath.Join("roadmap", "transflow", "jobs", "schemas", "plan.schema.json"))
}

func validateNextJSON(data []byte) error {
    return validateJSONAgainstSchema(data, filepath.Join("roadmap", "transflow", "jobs", "schemas", "next.schema.json"))
}

