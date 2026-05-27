package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	//go:embed schemas/*.json
	migSchemaFS embed.FS

	migSpecSchemaOnce sync.Once
	migSpecSchemaObj  *jsonschema.Schema
	migSpecSchemaErr  error
)

const migSpecSchemaURL = "https://github.com/iw2rmb/ploy/schemas/mig.schema.json"

// MigSpecSchemaJSON returns the embedded mig JSON Schema bytes.
func MigSpecSchemaJSON() ([]byte, error) {
	raw, err := migSchemaFS.ReadFile("schemas/mig.schema.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded MIG schema: %w", err)
	}
	return append([]byte(nil), raw...), nil
}

// ValidateMigSpecJSON validates raw JSON against the embedded mig JSON Schema.
func ValidateMigSpecJSON(raw []byte) error {
	if err := loadMigSpecSchema(); err != nil {
		return err
	}

	var instance any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&instance); err != nil {
		return fmt.Errorf("parse migs spec json: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("parse migs spec json: multiple JSON values")
	}

	if err := migSpecSchemaObj.Validate(instance); err != nil {
		var validationErr *jsonschema.ValidationError
		if errors.As(err, &validationErr) {
			return fmt.Errorf("%s", formatMigSchemaError(validationErr))
		}
		return fmt.Errorf("mig schema validation failed: %w", err)
	}
	return nil
}

func loadMigSpecSchema() error {
	migSpecSchemaOnce.Do(func() {
		raw, err := MigSpecSchemaJSON()
		if err != nil {
			migSpecSchemaErr = err
			return
		}

		var schemaDoc any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&schemaDoc); err != nil {
			migSpecSchemaErr = fmt.Errorf("parse embedded MIG schema: %w", err)
			return
		}

		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft7)
		if err := compiler.AddResource(migSpecSchemaURL, schemaDoc); err != nil {
			migSpecSchemaErr = fmt.Errorf("load embedded MIG schema: %w", err)
			return
		}
		migSpecSchemaObj, err = compiler.Compile(migSpecSchemaURL)
		if err != nil {
			migSpecSchemaErr = fmt.Errorf("compile embedded MIG schema: %w", err)
			return
		}
	})
	if migSpecSchemaErr != nil {
		return fmt.Errorf("mig schema validation unavailable: %w", migSpecSchemaErr)
	}
	return nil
}

func formatMigSchemaError(err *jsonschema.ValidationError) string {
	units := collectMigSchemaErrorUnits(err.BasicOutput())
	if len(units) == 0 {
		return err.Error()
	}
	return strings.Join(units, "; ")
}

func collectMigSchemaErrorUnits(unit *jsonschema.OutputUnit) []string {
	if unit == nil {
		return nil
	}
	var out []string
	if unit.Error != nil {
		path := migSchemaInstancePath(unit.InstanceLocation)
		msg := unit.Error.String()
		if path == "" {
			out = append(out, msg)
		} else {
			out = append(out, path+": "+msg)
		}
	}
	for i := range unit.Errors {
		out = append(out, collectMigSchemaErrorUnits(&unit.Errors[i])...)
	}
	return out
}

func migSchemaInstancePath(ptr string) string {
	ptr = strings.TrimSpace(ptr)
	if ptr == "" || ptr == "/" {
		return ""
	}
	if strings.HasPrefix(ptr, "/") {
		ptr = strings.TrimPrefix(ptr, "/")
	}
	parts := strings.Split(ptr, "/")
	var b strings.Builder
	for _, part := range parts {
		part = strings.ReplaceAll(part, "~1", "/")
		part = strings.ReplaceAll(part, "~0", "~")
		if part == "" {
			continue
		}
		if isArrayIndex(part) {
			b.WriteString("[")
			b.WriteString(part)
			b.WriteString("]")
			continue
		}
		if b.Len() > 0 {
			b.WriteString(".")
		}
		b.WriteString(part)
	}
	return b.String()
}

func isArrayIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
