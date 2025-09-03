package core

import (
    "context"
    "testing"
)

func TestNewEngine_NoOp(t *testing.T) {
    eng := NewEngine(EngineConfig{})
    if eng == nil {
        t.Fatalf("expected engine instance")
    }
    res, err := eng.Analyze(context.Background(), Codebase{Repository: "", Path: "."})
    if err != nil || res == nil {
        t.Fatalf("analyze failed: %v", err)
    }
    tr, err := eng.Transform(context.Background(), TransformRequest{Codebase: Codebase{Path: "."}})
    if err != nil || tr == nil || !tr.Success {
        t.Fatalf("transform failed: %v", err)
    }
}

