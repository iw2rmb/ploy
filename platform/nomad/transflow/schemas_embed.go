package transflow

import _ "embed"

//go:embed schemas/plan.schema.json
var planSchema []byte

//go:embed schemas/next.schema.json
var nextSchema []byte

// Optional additional schemas exposed on demand
//go:embed schemas/tools.schema.json
var toolsSchema []byte

//go:embed schemas/limits.schema.json
var limitsSchema []byte

// Exported accessors (used by CLI for validation)
func GetPlanSchema() []byte { return planSchema }
func GetNextSchema() []byte { return nextSchema }
func GetToolsSchema() []byte { return toolsSchema }
func GetLimitsSchema() []byte { return limitsSchema }

