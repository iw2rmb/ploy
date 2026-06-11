package store

import _ "embed"

// schemaSQL is the current schema snapshot used by sqlc and schema-shape tests.
// Runtime migrations are loaded from internal/store/migrations/tern.
//
//go:embed schema.sql
var schemaSQL string
