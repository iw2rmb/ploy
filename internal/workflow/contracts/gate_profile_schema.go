package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func loadGateProfileSchema() error {
	gateProfileSchemaOnce.Do(func() {
		path, err := gateProfileSchemaPath()
		if err != nil {
			gateProfileSchemaErr = err
			return
		}
		schemaBytes, err := os.ReadFile(path)
		if err != nil {
			gateProfileSchemaErr = fmt.Errorf("read prep schema %q: %w", path, err)
			return
		}
		gateProfileSchemaObj, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(schemaBytes))
		if err != nil {
			gateProfileSchemaErr = fmt.Errorf("compile prep schema %q: %w", path, err)
			return
		}
	})
	if gateProfileSchemaErr != nil {
		return fmt.Errorf("prep schema validation unavailable: %w", gateProfileSchemaErr)
	}
	return nil
}

func gateProfileSchemaPath() (string, error) {
	candidates := make([]string, 0, 3)
	_, file, _, ok := runtime.Caller(0)
	if ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "schemas", "gate_profile.schema.json"))
	}
	candidates = append(candidates,
		filepath.Join("docs", "schemas", "gate_profile.schema.json"),
		filepath.Join("/etc", "ploy", "schemas", "gate_profile.schema.json"),
	)
	var errs []string
	for _, path := range candidates {
		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", path, err))
	}
	return "", fmt.Errorf("resolve schema path: no candidates found (%s)", strings.Join(errs, "; "))
}
