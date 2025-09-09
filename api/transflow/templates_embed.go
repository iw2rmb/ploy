package transflow

import _ "embed"

// Embed HCL templates into the API binary to avoid filesystem dependencies

//go:embed templates/planner.hcl
var plannerHCL []byte

//go:embed templates/llm_exec.hcl
var llmExecHCL []byte

//go:embed templates/orw_apply.hcl
var orwApplyHCL []byte

//go:embed templates/reducer.hcl
var reducerHCL []byte
