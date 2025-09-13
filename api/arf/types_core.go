package arf

// TransformationType defines types of transformations available
type TransformationType string

const (
	TransformationTypeCleanup   TransformationType = "cleanup"
	TransformationTypeModernize TransformationType = "modernize"
	TransformationTypeMigration TransformationType = "migration"
	TransformationTypeSecurity  TransformationType = "security"
	TransformationTypeRefactor  TransformationType = "refactor"
	TransformationTypeOptimize  TransformationType = "optimize"
	TransformationTypeWASM      TransformationType = "wasm"
)

// CodeChange represents a specific change to be made to code
type CodeChange struct {
	Type        string `json:"type"`
	StartByte   int    `json:"start_byte,omitempty"`
	EndByte     int    `json:"end_byte,omitempty"`
	OldText     string `json:"old_text,omitempty"`
	NewText     string `json:"new_text,omitempty"`
	Explanation string `json:"explanation,omitempty"`
}
