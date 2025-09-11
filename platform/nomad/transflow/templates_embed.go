package transflow

import _ "embed"

// Embed HCL templates from platform/nomad/transflow into the binary

//go:embed planner.hcl
var plannerHCL []byte

//go:embed llm_exec.hcl
var llmExecHCL []byte

//go:embed orw_apply.hcl
var orwApplyHCL []byte

//go:embed reducer.hcl
var reducerHCL []byte

// Export helpers
func GetPlannerTemplate() []byte  { return plannerHCL }
func GetLLMExecTemplate() []byte  { return llmExecHCL }
func GetORWApplyTemplate() []byte { return orwApplyHCL }
func GetReducerTemplate() []byte  { return reducerHCL }
