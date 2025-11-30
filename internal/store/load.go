package store

import (
	_ "embed"
)

//go:embed schema.sql
var schemaSQL string

// getSchemaSQL returns the embedded SCHEMA.sql content.
func getSchemaSQL() string {
	return schemaSQL
}
