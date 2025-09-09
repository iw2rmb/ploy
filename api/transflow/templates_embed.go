package transflow

import _ "embed"

// Embed HCL templates into the API binary to avoid filesystem dependencies

//go:embed ../../roadmap/transflow/jobs/planner.hcl
var plannerHCL []byte

//go:embed ../../roadmap/transflow/jobs/llm_exec.hcl
var llmExecHCL []byte

//go:embed ../../roadmap/transflow/jobs/orw_apply.hcl
var orwApplyHCL []byte

//go:embed ../../roadmap/transflow/jobs/reducer.hcl
var reducerHCL []byte

