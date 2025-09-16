package mods

import "testing"

func TestShouldUseKBSuggestions(t *testing.T) {
    kb := &KBIntegration{config: &KBConfig{Enabled: true, ReadThreshold: 0.7}}

    if kb.ShouldUseKBSuggestions(&KBContext{HasData: false, MatchConfidence: 1.0}) {
        t.Fatal("expected false when HasData is false")
    }
    if kb.ShouldUseKBSuggestions(&KBContext{HasData: true, MatchConfidence: 0.69}) {
        t.Fatal("expected false when confidence < threshold")
    }
    if !kb.ShouldUseKBSuggestions(&KBContext{HasData: true, MatchConfidence: 0.70}) {
        t.Fatal("expected true when confidence == threshold and enabled")
    }
    // Disabled config
    kb.config.Enabled = false
    if kb.ShouldUseKBSuggestions(&KBContext{HasData: true, MatchConfidence: 1.0}) {
        t.Fatal("expected false when KB is disabled")
    }
}

func TestConvertKBFixesToBranchSpecs(t *testing.T) {
    kb := &KBIntegration{config: &KBConfig{Enabled: true}}
    fixes := []PromotedFix{
        {Kind: "orw_recipe", Ref: "org.openrewrite.java.AddMissingImports", Score: 0.9},
        {Kind: "patch_fingerprint", Ref: "patch-abcdef", Score: 0.8},
    }
    branches := kb.ConvertKBFixesToBranchSpecs(fixes)
    if len(branches) != 2 {
        t.Fatalf("expected 2 branches, got %d", len(branches))
    }
    if branches[0].Type != string(StepTypeORWApply) {
        t.Fatalf("expected first branch type %q, got %q", StepTypeORWApply, branches[0].Type)
    }
    if branches[1].Type != "patch-apply" {
        t.Fatalf("expected second branch type 'patch-apply', got %q", branches[1].Type)
    }
    if branches[0].Inputs["recipe"] != "org.openrewrite.java.AddMissingImports" {
        t.Fatalf("expected recipe in first branch inputs")
    }
    if branches[1].Inputs["patch_fingerprint"] != "patch-abcdef" {
        t.Fatalf("expected patch_fingerprint in second branch inputs")
    }
}

