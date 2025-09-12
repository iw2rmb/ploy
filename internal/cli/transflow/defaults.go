package transflow

import (
    "os"
    "strings"
    "time"
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
    SeaweedURL    string
    PlannerTimeout  time.Duration
    ReducerTimeout  time.Duration
    LLMExecTimeout  time.Duration
    ORWApplyTimeout time.Duration
}

// ResolveDefaults resolves defaults using the provided getter (env-like).
// If a value is not provided, sensible built-ins are used.
func ResolveDefaults(get func(string) string) Defaults {
    // helper to parse durations with fallback
    parseDur := func(key, def string) time.Duration {
        if v := get(key); v != "" {
            if d, err := time.ParseDuration(v); err == nil {
                return d
            }
        }
        d, _ := time.ParseDuration(def)
        return d
    }
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
    seaweed := get("PLOY_SEAWEEDFS_URL")
    if seaweed == "" {
        seaweed = "http://seaweedfs-filer.service.consul:8888"
    }
    return Defaults{
        Registry:      reg,
        PlannerImage:  planner,
        ReducerImage:  reducer,
        LLMExecImage:  llm,
        ORWApplyImage: orw,
        DC:            dc,
        Allowlist:     allow,
        SeaweedURL:    seaweed,
        PlannerTimeout:  parseDur("TRANSFLOW_PLANNER_TIMEOUT", "15m"),
        ReducerTimeout:  parseDur("TRANSFLOW_REDUCER_TIMEOUT", "10m"),
        LLMExecTimeout:  parseDur("TRANSFLOW_LLM_EXEC_TIMEOUT", "30m"),
        ORWApplyTimeout: parseDur("TRANSFLOW_ORW_APPLY_TIMEOUT", "30m"),
    }
}

// ResolveDefaultsFromEnv resolves using os.Getenv.
func ResolveDefaultsFromEnv() Defaults { return ResolveDefaults(os.Getenv) }
