package contracts

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/xeipuuv/gojsonschema"
)

var (
	gateProfileSchemaOnce sync.Once
	gateProfileSchemaObj  *gojsonschema.Schema
	gateProfileSchemaErr  error
)

func validateGateProfileJSON(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("prep schema validation failed: empty output")
	}
	if !json.Valid(raw) {
		return fmt.Errorf("prep schema validation failed: output is not valid JSON")
	}
	if err := loadGateProfileSchema(); err != nil {
		return err
	}
	result, err := gateProfileSchemaObj.Validate(gojsonschema.NewBytesLoader(raw))
	if err != nil {
		return fmt.Errorf("prep schema validation failed: %w", err)
	}
	if result.Valid() {
		return nil
	}
	msgs := make([]string, 0, len(result.Errors()))
	for _, e := range result.Errors() {
		msgs = append(msgs, e.String())
	}
	return fmt.Errorf("prep schema validation failed: %s", strings.Join(msgs, "; "))
}

func ValidateGateProfileJSONForSchema(raw []byte, schemaID string) error {
	switch strings.TrimSpace(schemaID) {
	case GateProfileCandidateSchemaID:
		return validateGateProfileJSON(raw)
	default:
		return fmt.Errorf("prep schema validation failed: unsupported schema id %q", schemaID)
	}
}

func ReadGateProfileSchemaJSON() ([]byte, error) {
	schemaBytes, err := migSchemaFS.ReadFile("schemas/gate_profile.schema.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded prep schema: %w", err)
	}
	return schemaBytes, nil
}

func loadGateProfileSchema() error {
	gateProfileSchemaOnce.Do(func() {
		schemaBytes, err := ReadGateProfileSchemaJSON()
		if err != nil {
			gateProfileSchemaErr = err
			return
		}
		gateProfileSchemaObj, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(schemaBytes))
		if err != nil {
			gateProfileSchemaErr = fmt.Errorf("compile prep schema: %w", err)
			return
		}
	})
	if gateProfileSchemaErr != nil {
		return fmt.Errorf("prep schema validation unavailable: %w", gateProfileSchemaErr)
	}
	return nil
}
