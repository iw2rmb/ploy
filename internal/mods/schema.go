package mods

import (
	"encoding/json"
	"fmt"
	"strings"

	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/transflow"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// validateJSONAgainstSchemaBytes validates a JSON document against an embedded schema
func validateJSONAgainstSchemaBytes(data []byte, schemaBytes []byte) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", strings.NewReader(string(schemaBytes))); err != nil {
		return fmt.Errorf("add schema: %w", err)
	}
	sch, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	if err := sch.Validate(v); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	return nil
}

func validatePlanJSON(data []byte) error {
	return validateJSONAgainstSchemaBytes(data, nomadtpl.GetPlanSchema())
}
func validateNextJSON(data []byte) error {
	return validateJSONAgainstSchemaBytes(data, nomadtpl.GetNextSchema())
}
