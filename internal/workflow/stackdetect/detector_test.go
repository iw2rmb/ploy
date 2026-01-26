package stackdetect

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestDetect_EmptyWorkspace(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "empty")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for empty workspace")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

func TestDetect_AmbiguousBothMavenGradle(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "ambiguous", "both-maven-gradle")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for ambiguous workspace")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsAmbiguous() {
		t.Errorf("expected reason 'ambiguous', got %q", detErr.Reason)
	}

	// Verify evidence includes both build files
	if len(detErr.Evidence) != 2 {
		t.Errorf("expected 2 evidence items, got %d", len(detErr.Evidence))
	}
	hasPoml := false
	hasGradle := false
	for _, e := range detErr.Evidence {
		if e.Path == "pom.xml" && e.Key == "build.file" && e.Value == "exists" {
			hasPoml = true
		}
		if e.Path == "build.gradle" && e.Key == "build.file" && e.Value == "exists" {
			hasGradle = true
		}
	}
	if !hasPoml {
		t.Error("expected evidence for pom.xml")
	}
	if !hasGradle {
		t.Error("expected evidence for build.gradle")
	}
}

func TestDetect_MavenJava11Release(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "java11-release")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "java", "maven", "11")
	assertEvidence(t, obs, "maven.compiler.release", "11")
}

func TestDetect_MavenJava17SourceTarget(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "java17-source-target")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "java", "maven", "17")
}

func TestDetect_MavenJava11Property(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "java11-property")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "java", "maven", "11")
	assertEvidence(t, obs, "java.version", "11")
}

func TestDetect_MavenJava17Parent(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "java17-parent")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "java", "maven", "17")
}

func TestDetect_MavenUnresolvedPlaceholder(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "unresolved-placeholder")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for unresolved placeholder")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

func TestDetect_MavenNoJavaVersion(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "no-java-version")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error when no Java version configured")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

func TestDetectTool_MavenNoJavaVersion_ReturnsTool(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "no-java-version")

	obs, err := DetectTool(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obs.Language != "java" {
		t.Fatalf("Language = %q, want %q", obs.Language, "java")
	}
	if obs.Tool != "maven" {
		t.Fatalf("Tool = %q, want %q", obs.Tool, "maven")
	}
	if obs.Release != nil {
		t.Fatalf("Release = %v, want nil", *obs.Release)
	}
}

func TestDetect_GradleJava17Toolchain(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "gradle", "java17-toolchain")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "java", "gradle", "17")
	assertEvidence(t, obs, "toolchain.languageVersion", "17")
}

func TestDetect_GradleJava11Compatibility(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "gradle", "java11-compatibility")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "java", "gradle", "11")
}

func TestDetect_GradleDynamicVersion(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "gradle", "dynamic-version")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for dynamic version logic")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

func TestDetect_GradleNoJavaConfig(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "gradle", "no-java-config")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error when no Java config")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

// Precedence tests

func TestDetect_MavenPrecedenceReleaseOverSourceTarget(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "precedence-release-over-source-target")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// maven.compiler.release=17 should take precedence over source/target=11
	assertObservation(t, obs, "java", "maven", "17")
	assertEvidence(t, obs, "maven.compiler.release", "17")
}

func TestDetect_MavenPrecedenceSourceTargetOverJavaVersion(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "maven", "precedence-source-target-over-java-version")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// maven.compiler.source/target=17 should take precedence over java.version=11
	assertObservation(t, obs, "java", "maven", "17")
}

func TestDetect_GradlePrecedenceToolchainOverCompatibility(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "gradle", "precedence-toolchain-over-compatibility")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// JavaLanguageVersion.of(21) should take precedence over sourceCompatibility=17
	assertObservation(t, obs, "java", "gradle", "21")
	assertEvidence(t, obs, "toolchain.languageVersion", "21")
}

// assertObservation validates the observation fields.
func assertObservation(t *testing.T, obs *Observation, language, tool, release string) {
	t.Helper()

	if obs.Language != language {
		t.Errorf("expected language %q, got %q", language, obs.Language)
	}
	if obs.Tool != tool {
		t.Errorf("expected tool %q, got %q", tool, obs.Tool)
	}
	if obs.Release == nil {
		t.Errorf("expected release %q, got nil", release)
	} else if *obs.Release != release {
		t.Errorf("expected release %q, got %q", release, *obs.Release)
	}
}

// assertEvidence checks that the observation has evidence with the given key and value.
func assertEvidence(t *testing.T, obs *Observation, key, value string) {
	t.Helper()

	for _, e := range obs.Evidence {
		if e.Key == key && e.Value == value {
			return
		}
	}
	t.Errorf("expected evidence with key=%q value=%q, got %+v", key, value, obs.Evidence)
}

// Cross-language ambiguity tests

func TestDetect_AmbiguousJavaGo(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "ambiguous", "java-go")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for ambiguous workspace")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsAmbiguous() {
		t.Errorf("expected reason 'ambiguous', got %q", detErr.Reason)
	}

	// Verify evidence includes both languages.
	hasPom := false
	hasGoMod := false
	for _, e := range detErr.Evidence {
		if e.Path == "pom.xml" && e.Key == "build.file" {
			hasPom = true
		}
		if e.Path == "go.mod" && e.Key == "build.file" {
			hasGoMod = true
		}
	}
	if !hasPom {
		t.Error("expected evidence for pom.xml")
	}
	if !hasGoMod {
		t.Error("expected evidence for go.mod")
	}
}

func TestDetect_AmbiguousPythonRust(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "ambiguous", "python-rust")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for ambiguous workspace")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsAmbiguous() {
		t.Errorf("expected reason 'ambiguous', got %q", detErr.Reason)
	}

	// Verify evidence includes both languages.
	hasPythonVersion := false
	hasCargo := false
	for _, e := range detErr.Evidence {
		if e.Path == ".python-version" && e.Key == "build.file" {
			hasPythonVersion = true
		}
		if e.Path == "Cargo.toml" && e.Key == "build.file" {
			hasCargo = true
		}
	}
	if !hasPythonVersion {
		t.Error("expected evidence for .python-version")
	}
	if !hasCargo {
		t.Error("expected evidence for Cargo.toml")
	}
}

func TestDetect_AmbiguousMultiple(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "ambiguous", "multiple")

	_, err := Detect(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for ambiguous workspace")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsAmbiguous() {
		t.Errorf("expected reason 'ambiguous', got %q", detErr.Reason)
	}

	// Should have evidence for 3+ languages.
	if len(detErr.Evidence) < 3 {
		t.Errorf("expected at least 3 evidence items for multiple languages, got %d", len(detErr.Evidence))
	}
}

// New language detection tests via Detect

func TestDetect_Go122(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "go", "go122")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "go", "go", "1.22")
	assertEvidence(t, obs, "go", "1.22")
}

func TestDetect_Rust176Cargo(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "rust", "rust176-cargo")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "rust", "cargo", "1.76")
	assertEvidence(t, obs, "rust-version", "1.76")
}

func TestDetect_Python311VersionFile(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "python", "python311-version-file")

	obs, err := Detect(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "python", "pip", "3.11")
	assertEvidence(t, obs, "python", "3.11")
}
