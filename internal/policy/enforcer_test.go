package policy

import (
    "os"
    "testing"
)

func TestEnforcer_StrictRequiresSignatureAndSBOM(t *testing.T) {
    os.Setenv("PLOY_POLICY_STRICT_ENVS", "prod")
    e := NewDefaultEnforcer()
    err := e.Enforce(ArtifactInput{Env: "prod", Signed: false, SBOMPresent: true, Lane: "C", ImageSizeMB: 100})
    if err == nil { t.Fatalf("expected error for unsigned in strict env") }
    err = e.Enforce(ArtifactInput{Env: "prod", Signed: true, SBOMPresent: false, Lane: "C", ImageSizeMB: 100})
    if err == nil { t.Fatalf("expected error for missing sbom in strict env") }
    // break-glass bypasses
    err = e.Enforce(ArtifactInput{Env: "prod", Signed: false, SBOMPresent: false, BreakGlass: true})
    if err != nil { t.Fatalf("expected no error with break-glass, got %v", err) }
}

func TestEnforcer_SizeCapsPerLane(t *testing.T) {
    os.Setenv("PLOY_POLICY_STRICT_ENVS", "prod,staging")
    os.Setenv("PLOY_POLICY_MAX_SIZE_MB_C", "500")
    e := NewDefaultEnforcer()
    // Over cap
    if err := e.Enforce(ArtifactInput{Env: "dev", Lane: "C", ImageSizeMB: 600}); err == nil {
        t.Fatalf("expected size cap error for lane C")
    }
    // Under cap
    if err := e.Enforce(ArtifactInput{Env: "dev", Lane: "C", ImageSizeMB: 100}); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func TestEnforcer_VulnScanRequiredForImagesInStrict(t *testing.T) {
    os.Setenv("PLOY_POLICY_STRICT_ENVS", "prod")
    e := NewDefaultEnforcer()
    // Missing vuln pass in strict env with docker image
    if err := e.Enforce(ArtifactInput{Env: "prod", DockerImage: "repo/image:tag", Signed: true, SBOMPresent: true}); err == nil {
        t.Fatalf("expected vuln scan failure in strict env")
    }
    // Pass when VulnScanPassed
    if err := e.Enforce(ArtifactInput{Env: "prod", DockerImage: "repo/image:tag", Signed: true, SBOMPresent: true, VulnScanPassed: true}); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

