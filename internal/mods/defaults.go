package mods

import (
	"os"
	"strings"
	"time"
)

// Defaults holds resolved Mods defaults from environment and built-ins
type Defaults struct {
	Registry          string
	PlannerImage      string
	ReducerImage      string
	LLMExecImage      string
	ORWApplyImage     string
	DC                string
	Allowlist         []string
	SeaweedURL        string
	JetStreamURL      string
	PlannerTimeout    time.Duration
	ReducerTimeout    time.Duration
	LLMExecTimeout    time.Duration
	ORWApplyTimeout   time.Duration
	BuildApplyTimeout time.Duration
	AllowPartialORW   bool
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
	reg := get("MODS_REGISTRY")
	if reg == "" {
		reg = "registry.dev.ployman.app"
	}
	planner := get("MODS_PLANNER_IMAGE")
	if planner == "" {
		planner = reg + "/langgraph-runner:latest"
	}
	reducer := get("MODS_REDUCER_IMAGE")
	if reducer == "" {
		reducer = planner
	}
	llm := get("MODS_LLM_EXEC_IMAGE")
	if llm == "" {
		llm = planner
	}
	orw := get("MODS_ORW_APPLY_IMAGE")
	if orw == "" {
		orw = reg + "/openrewrite-jvm:latest"
	}
	dc := get("NOMAD_DC")
	if dc == "" {
		dc = "dc1"
	}
	allowCSV := get("MODS_ALLOWLIST")
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
		seaweed = "http://seaweedfs-filer.storage.ploy.local:8888"
	}
	jetstream := get("PLOY_JETSTREAM_URL")
	if jetstream == "" {
		jetstream = "nats://nats.ploy.local:4222"
	}
	allowPartial := func() bool {
		v := strings.ToLower(strings.TrimSpace(get("MODS_ALLOW_PARTIAL_ORW")))
		return v == "1" || v == "true" || v == "yes"
	}()
	return Defaults{
		Registry:          reg,
		PlannerImage:      planner,
		ReducerImage:      reducer,
		LLMExecImage:      llm,
		ORWApplyImage:     orw,
		DC:                dc,
		Allowlist:         allow,
		SeaweedURL:        seaweed,
		JetStreamURL:      jetstream,
		PlannerTimeout:    parseDur("MODS_PLANNER_TIMEOUT", "15m"),
		ReducerTimeout:    parseDur("MODS_REDUCER_TIMEOUT", "10m"),
		LLMExecTimeout:    parseDur("MODS_LLM_EXEC_TIMEOUT", "30m"),
		ORWApplyTimeout:   parseDur("MODS_ORW_APPLY_TIMEOUT", "30m"),
		BuildApplyTimeout: parseDur("MODS_BUILD_APPLY_TIMEOUT", "10m"),
		AllowPartialORW:   allowPartial,
	}
}

// ResolveDefaultsFromEnv resolves using os.Getenv.
func ResolveDefaultsFromEnv() Defaults { return ResolveDefaults(os.Getenv) }
