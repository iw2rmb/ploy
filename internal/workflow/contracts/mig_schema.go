package contracts

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/xeipuuv/gojsonschema"
)

var (
	//go:embed schemas/*.json
	migSchemaFS embed.FS

	migSpecSchemaOnce sync.Once
	migSpecSchemaObj  *gojsonschema.Schema
	migSpecSchemaErr  error
)

func validateMigSpecSchema(raw []byte) error {
	if err := loadMigSpecSchema(); err != nil {
		return err
	}
	result, err := migSpecSchemaObj.Validate(gojsonschema.NewBytesLoader(raw))
	if err != nil {
		return fmt.Errorf("mig schema validation failed: %w", err)
	}
	if result.Valid() {
		return nil
	}
	msgs := make([]string, 0, len(result.Errors()))
	for _, schemaErr := range result.Errors() {
		msgs = append(msgs, formatMigSchemaError(schemaErr))
	}
	return fmt.Errorf("mig schema validation failed: %s", strings.Join(msgs, "; "))
}

func loadMigSpecSchema() error {
	migSpecSchemaOnce.Do(func() {
		raw, err := migSchemaFS.ReadFile("schemas/mig.schema.json")
		if err != nil {
			migSpecSchemaErr = fmt.Errorf("read embedded MIG schema: %w", err)
			return
		}
		migSpecSchemaObj, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(raw))
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

func formatMigSchemaError(err gojsonschema.ResultError) string {
	path := toContractPath(err.Field())
	if path == "" {
		path = "(root)"
	}
	if err.Type() == "false" {
		return path + ": forbidden"
	}
	return err.String()
}

func toContractPath(field string) string {
	field = strings.TrimSpace(field)
	if field == "" || field == "(root)" {
		return ""
	}
	parts := strings.Split(field, ".")
	var b strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		if idx, convErr := strconv.Atoi(part); convErr == nil {
			b.WriteString("[")
			b.WriteString(strconv.Itoa(idx))
			b.WriteString("]")
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteString(".")
		}
		b.WriteString(part)
	}
	return b.String()
}
