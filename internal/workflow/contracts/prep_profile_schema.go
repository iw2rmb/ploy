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
	prepProfileSchemaOnce sync.Once
	prepProfileSchemaObj  *gojsonschema.Schema
	prepProfileSchemaErr  error
)

func validatePrepProfileJSON(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("prep schema validation failed: empty output")
	}
	if !json.Valid(raw) {
		return fmt.Errorf("prep schema validation failed: output is not valid JSON")
	}
	if err := loadPrepProfileSchema(); err != nil {
		return err
	}
	result, err := prepProfileSchemaObj.Validate(gojsonschema.NewBytesLoader(raw))
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

func ValidatePrepProfileJSONForSchema(raw []byte, schemaID string) error {
	switch strings.TrimSpace(schemaID) {
	case PrepProfileCandidateSchemaID:
		return validatePrepProfileJSON(raw)
	default:
		return fmt.Errorf("prep schema validation failed: unsupported schema id %q", schemaID)
	}
}

func loadPrepProfileSchema() error {
	prepProfileSchemaOnce.Do(func() {
		path, err := prepProfileSchemaPath()
		if err != nil {
			prepProfileSchemaErr = err
			return
		}
		schemaBytes, err := os.ReadFile(path)
		if err != nil {
			prepProfileSchemaErr = fmt.Errorf("read prep schema %q: %w", path, err)
			return
		}
		prepProfileSchemaObj, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(schemaBytes))
		if err != nil {
			prepProfileSchemaErr = fmt.Errorf("compile prep schema %q: %w", path, err)
			return
		}
	})
	if prepProfileSchemaErr != nil {
		return fmt.Errorf("prep schema validation unavailable: %w", prepProfileSchemaErr)
	}
	return nil
}

func prepProfileSchemaPath() (string, error) {
	candidates := make([]string, 0, 3)
	_, file, _, ok := runtime.Caller(0)
	if ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "schemas", "prep_profile.schema.json"))
	}
	candidates = append(candidates,
		filepath.Join("docs", "schemas", "prep_profile.schema.json"),
		filepath.Join("/etc", "ploy", "schemas", "prep_profile.schema.json"),
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
