package core

import (
    "context"
    "time"
)

// EngineConfig configures the ARF engine (initial slice)
type EngineConfig struct {
    // Placeholder for injected dependencies to be added incrementally
}

// DefaultEngine is a minimal, no-op implementation to enable gradual migration.
type DefaultEngine struct{
    cfg EngineConfig
}

func NewEngine(cfg EngineConfig) *DefaultEngine { return &DefaultEngine{cfg: cfg} }

func (e *DefaultEngine) Analyze(ctx context.Context, codebase Codebase) (*AnalysisResult, error) {
    return &AnalysisResult{ID: "noop-analyze", Timestamp: time.Now(), Issues: nil, Metadata: map[string]interface{}{"engine": "default"}}, nil
}

func (e *DefaultEngine) Transform(ctx context.Context, req TransformRequest) (*TransformResult, error) {
    return &TransformResult{ID: "noop-transform", Timestamp: time.Now(), Success: true, Summary: "noop"}, nil
}

