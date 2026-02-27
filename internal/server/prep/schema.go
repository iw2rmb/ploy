package prep

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
	schemaOnce sync.Once
	schemaObj  *gojsonschema.Schema
	schemaErr  error
)

func validateProfileJSON(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("prep schema validation failed: empty output")
	}
	if !json.Valid(raw) {
		return fmt.Errorf("prep schema validation failed: output is not valid JSON")
	}

	if err := loadSchema(); err != nil {
		return err
	}

	result, err := schemaObj.Validate(gojsonschema.NewBytesLoader(raw))
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

func loadSchema() error {
	schemaOnce.Do(func() {
		path, err := prepProfileSchemaPath()
		if err != nil {
			schemaErr = err
			return
		}
		schemaBytes, err := os.ReadFile(path)
		if err != nil {
			schemaErr = fmt.Errorf("read prep schema %q: %w", path, err)
			return
		}
		schemaObj, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(schemaBytes))
		if err != nil {
			schemaErr = fmt.Errorf("compile prep schema %q: %w", path, err)
			return
		}
	})
	if schemaErr != nil {
		return fmt.Errorf("prep schema validation unavailable: %w", schemaErr)
	}
	return nil
}

func prepProfileSchemaPath() (string, error) {
	candidates := make([]string, 0, 3)

	_, file, _, ok := runtime.Caller(0)
	if ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "schemas", "prep_profile.schema.json"))
	}

	// Fall back to repository-relative and image-provisioned locations.
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
