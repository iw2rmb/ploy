package transflow

import "os"

// LLMDefaults captures model/tools/limits JSON knobs with sensible defaults
type LLMDefaults struct {
    Model     string
    ToolsJSON string
    LimitsJSON string
}

// ResolveLLMDefaults resolves LLM knobs from the provided getter (env-like) with built-in defaults
func ResolveLLMDefaults(get func(string) string) LLMDefaults {
    model := get("TRANSFLOW_MODEL")
    if model == "" {
        model = "gpt-4o-mini@2024-08-06"
    }
    tools := get("TRANSFLOW_TOOLS")
    if tools == "" {
        tools = `{"file":{"allow":["src/**","pom.xml"]},"search":{"provider":"rg","allow":["src/**"]}}`
    }
    limits := get("TRANSFLOW_LIMITS")
    if limits == "" {
        limits = `{"max_steps":8,"max_tool_calls":12,"timeout":"30m"}`
    }
    return LLMDefaults{Model: model, ToolsJSON: tools, LimitsJSON: limits}
}

// ResolveLLMDefaultsFromEnv uses os.Getenv
func ResolveLLMDefaultsFromEnv() LLMDefaults { return ResolveLLMDefaults(os.Getenv) }

