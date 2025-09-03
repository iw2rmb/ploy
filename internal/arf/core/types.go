package core

import (
    "context"
    "time"
)

// Engine is the main ARF engine interface (initial slice)
type Engine interface {
    Analyze(ctx context.Context, codebase Codebase) (*AnalysisResult, error)
    Transform(ctx context.Context, req TransformRequest) (*TransformResult, error)
}

type Codebase struct {
    Repository string
    Branch     string
    Path       string
    Language   string
    Metadata   map[string]string
}

type AnalysisResult struct {
    ID        string
    Timestamp time.Time
    Issues    []string
    Metadata  map[string]interface{}
}

type TransformRequest struct {
    Codebase Codebase
    Type     string
    Options  map[string]interface{}
    DryRun   bool
}

type TransformResult struct {
    ID        string
    Timestamp time.Time
    Success   bool
    Summary   string
}

