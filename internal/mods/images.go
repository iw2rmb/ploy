package mods

import "os"

// Images holds resolved container image references.
type Images struct {
	Registry string
	Planner  string
	Reducer  string
	LLMExec  string
	ORWApply string
}

// ResolveImages resolves image references from environment with sensible defaults.
// Uses the Defaults resolver to centralize registry and per-image fallbacks.
func ResolveImages(get func(string) string) Images {
	d := ResolveDefaults(get)
	planner := get("TRANSFLOW_PLANNER_IMAGE")
	if planner == "" {
		planner = d.PlannerImage
	}
	reducer := get("TRANSFLOW_REDUCER_IMAGE")
	if reducer == "" {
		reducer = d.ReducerImage
	}
	llm := get("TRANSFLOW_LLM_EXEC_IMAGE")
	if llm == "" {
		llm = d.LLMExecImage
	}
	orw := get("TRANSFLOW_ORW_APPLY_IMAGE")
	if orw == "" {
		orw = d.ORWApplyImage
	}
	return Images{Registry: d.Registry, Planner: planner, Reducer: reducer, LLMExec: llm, ORWApply: orw}
}

// ResolveImagesFromEnv resolves using os.Getenv.
func ResolveImagesFromEnv() Images { return ResolveImages(getenv) }

// indirection for testability
var getenv = func(k string) string { return defaultGetenv(k) }

// defaultGetenv defined in same package to avoid import cycle; set by init in files where needed.
func defaultGetenv(k string) string { return os.Getenv(k) }
