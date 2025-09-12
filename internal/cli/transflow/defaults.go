package transflow

import (
	"os"
	"strings"
)

// Defaults holds resolved transflow defaults from environment and built-ins
type Defaults struct {
	Registry      string
	PlannerImage  string
	ReducerImage  string
	LLMExecImage  string
	ORWApplyImage string
	DC            string
	Allowlist     []string
}

// ResolveDefaults resolves defaults using the provided getter (env-like).
// If a value is not provided, sensible built-ins are used.
func ResolveDefaults(get func(string) string) Defaults {
	reg := get("TRANSFLOW_REGISTRY")
	if reg == "" {
		reg = "registry.dev.ployman.app"
	}
	planner := get("TRANSFLOW_PLANNER_IMAGE")
	if planner == "" {
		planner = reg + "/langgraph-runner:py-0.1.0"
	}
	reducer := get("TRANSFLOW_REDUCER_IMAGE")
	if reducer == "" {
		reducer = planner
	}
	llm := get("TRANSFLOW_LLM_EXEC_IMAGE")
	if llm == "" {
		llm = planner
	}
	orw := get("TRANSFLOW_ORW_APPLY_IMAGE")
	if orw == "" {
		orw = reg + "/openrewrite-jvm:latest"
	}
	dc := get("NOMAD_DC")
	if dc == "" {
		dc = "dc1"
	}
	allowCSV := get("TRANSFLOW_ALLOWLIST")
	var allow []string
	if allowCSV != "" {
		for _, part := range strings.Split(allowCSV, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				allow = append(allow, p)
			}
		}
	} else {
		allow = []string{"src/**", "pom.xml"}
	}
	return Defaults{
		Registry:      reg,
		PlannerImage:  planner,
		ReducerImage:  reducer,
		LLMExecImage:  llm,
		ORWApplyImage: orw,
		DC:            dc,
		Allowlist:     allow,
	}
}

// ResolveDefaultsFromEnv resolves using os.Getenv.
func ResolveDefaultsFromEnv() Defaults { return ResolveDefaults(os.Getenv) }
