package stackdetect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_Success(t *testing.T) {
	cases := []struct {
		name     string
		workspace string
		wantLang  string
		wantTool  string
		wantRel   string
		evidence  map[string]string // key → value pairs to assert
	}{
		{
			name:      "maven/java11-release",
			workspace: filepath.Join("testdata", "maven", "java11-release"),
			wantLang:  "java", wantTool: "maven", wantRel: "11",
			evidence: map[string]string{"maven.compiler.release": "11"},
		},
		{
			name:      "maven/java17-source-target",
			workspace: filepath.Join("testdata", "maven", "java17-source-target"),
			wantLang:  "java", wantTool: "maven", wantRel: "17",
		},
		{
			name:      "maven/java11-property",
			workspace: filepath.Join("testdata", "maven", "java11-property"),
			wantLang:  "java", wantTool: "maven", wantRel: "11",
			evidence: map[string]string{"java.version": "11"},
		},
		{
			name:      "maven/java17-parent",
			workspace: filepath.Join("testdata", "maven", "java17-parent"),
			wantLang:  "java", wantTool: "maven", wantRel: "17",
		},
		{
			name:      "maven/precedence-release-over-source-target",
			workspace: filepath.Join("testdata", "maven", "precedence-release-over-source-target"),
			wantLang:  "java", wantTool: "maven", wantRel: "17",
			evidence: map[string]string{"maven.compiler.release": "17"},
		},
		{
			name:      "maven/precedence-source-target-over-java-version",
			workspace: filepath.Join("testdata", "maven", "precedence-source-target-over-java-version"),
			wantLang:  "java", wantTool: "maven", wantRel: "17",
		},
		{
			name:      "gradle/java17-compatibility-javaversion",
			workspace: filepath.Join("testdata", "gradle", "java17-compatibility-javaversion"),
			wantLang:  "java", wantTool: "gradle", wantRel: "17",
			evidence: map[string]string{"sourceCompatibility": "17"},
		},
		{
			name:      "gradle/java11-compatibility",
			workspace: filepath.Join("testdata", "gradle", "java11-compatibility"),
			wantLang:  "java", wantTool: "gradle", wantRel: "11",
		},
		{
			name:      "gradle/kotlin-jvmtarget-javaversion",
			workspace: filepath.Join("testdata", "gradle", "kotlin-jvmtarget-javaversion"),
			wantLang:  "java", wantTool: "gradle", wantRel: "17",
			evidence: map[string]string{"kotlinOptions.jvmTarget": "17"},
		},
		{
			name:      "gradle/precedence-compatibility-over-kotlin-jvmtarget",
			workspace: filepath.Join("testdata", "gradle", "precedence-compatibility-over-kotlin-jvmtarget"),
			wantLang:  "java", wantTool: "gradle", wantRel: "11",
			evidence: map[string]string{"sourceCompatibility": "11"},
		},
		{
			name:      "go/go122",
			workspace: filepath.Join("testdata", "go", "go122"),
			wantLang:  "go", wantTool: "go", wantRel: "1.22",
			evidence: map[string]string{"go": "1.22"},
		},
		{
			name:      "rust/rust176-cargo",
			workspace: filepath.Join("testdata", "rust", "rust176-cargo"),
			wantLang:  "rust", wantTool: "cargo", wantRel: "1.76",
			evidence: map[string]string{"rust-version": "1.76"},
		},
		{
			name:      "python/python311-version-file",
			workspace: filepath.Join("testdata", "python", "python311-version-file"),
			wantLang:  "python", wantTool: "pip", wantRel: "3.11",
			evidence: map[string]string{"python": "3.11"},
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := Detect(ctx, tc.workspace)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertObservation(t, obs, tc.wantLang, tc.wantTool, tc.wantRel)
			for k, v := range tc.evidence {
				assertEvidence(t, obs, k, v)
			}
		})
	}
}

func TestDetect_Unknown(t *testing.T) {
	cases := []struct {
		name      string
		workspace string
	}{
		{"empty-workspace", filepath.Join("testdata", "empty")},
		{"maven/unresolved-placeholder", filepath.Join("testdata", "maven", "unresolved-placeholder")},
		{"maven/no-java-version", filepath.Join("testdata", "maven", "no-java-version")},
		{"gradle/dynamic-version", filepath.Join("testdata", "gradle", "dynamic-version")},
		{"gradle/no-java-config", filepath.Join("testdata", "gradle", "no-java-config")},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Detect(ctx, tc.workspace)
			assertDetectionError(t, err, "unknown")
		})
	}
}

func TestDetect_Ambiguous(t *testing.T) {
	type detectFunc = func(context.Context, string) (*Observation, error)

	cases := []struct {
		name         string
		fn           detectFunc
		workspace    string
		tempFiles    map[string]string // if set, creates temp dir with these files
		wantEvidence []string          // evidence paths to verify (empty = skip check)
		minEvidence  int               // minimum evidence count (0 = skip check)
	}{
		{
			name:         "Detect/both-maven-gradle",
			fn:           Detect,
			workspace:    filepath.Join("testdata", "ambiguous", "both-maven-gradle"),
			wantEvidence: []string{"pom.xml", "build.gradle"},
		},
		{
			name:         "Detect/both-maven-gradle-kts",
			fn:           Detect,
			tempFiles:    map[string]string{"pom.xml": "<project/>", "build.gradle.kts": "plugins {}"},
			wantEvidence: []string{"pom.xml", "build.gradle.kts"},
		},
		{
			name:         "Detect/java-go",
			fn:           Detect,
			workspace:    filepath.Join("testdata", "ambiguous", "java-go"),
			wantEvidence: []string{"pom.xml", goModuleFile},
		},
		{
			name:         "Detect/python-rust",
			fn:           Detect,
			workspace:    filepath.Join("testdata", "ambiguous", "python-rust"),
			wantEvidence: []string{".python-version", "Cargo.toml"},
		},
		{
			name:        "Detect/multiple",
			fn:          Detect,
			workspace:   filepath.Join("testdata", "ambiguous", "multiple"),
			minEvidence: 3,
		},
		{
			name:         "DetectTool/both-maven-gradle",
			fn:           DetectTool,
			workspace:    filepath.Join("testdata", "ambiguous", "both-maven-gradle"),
			wantEvidence: []string{"pom.xml", "build.gradle"},
		},
		{
			name:         "DetectTool/both-maven-gradle-kts",
			fn:           DetectTool,
			tempFiles:    map[string]string{"pom.xml": "<project/>", "build.gradle.kts": "plugins {}"},
			wantEvidence: []string{"pom.xml", "build.gradle.kts"},
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := tc.workspace
			if tc.tempFiles != nil {
				workspace = t.TempDir()
				for name, body := range tc.tempFiles {
					writeDetectFile(t, workspace, name, body)
				}
			}

			_, err := tc.fn(ctx, workspace)
			detErr := assertDetectionError(t, err, "ambiguous")

			for _, path := range tc.wantEvidence {
				assertEvidencePath(t, detErr.Evidence, path)
			}
			if tc.minEvidence > 0 && len(detErr.Evidence) < tc.minEvidence {
				t.Errorf("expected at least %d evidence items, got %d", tc.minEvidence, len(detErr.Evidence))
			}
		})
	}
}

func TestDetectTool_Success(t *testing.T) {
	cases := []struct {
		name      string
		workspace string
		wantLang  string
		wantTool  string
	}{
		{
			name:      "maven/no-java-version",
			workspace: filepath.Join("testdata", "maven", "no-java-version"),
			wantLang:  "java", wantTool: "maven",
		},
		{
			name:      "python/python311-poetry",
			workspace: filepath.Join("testdata", "python", "python311-poetry"),
			wantLang:  "python", wantTool: "poetry",
		},
		{
			name:      "python/python310-pyproject",
			workspace: filepath.Join("testdata", "python", "python310-pyproject"),
			wantLang:  "python", wantTool: "pip",
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := DetectTool(ctx, tc.workspace)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if obs.Language != tc.wantLang {
				t.Errorf("Language = %q, want %q", obs.Language, tc.wantLang)
			}
			if obs.Tool != tc.wantTool {
				t.Errorf("Tool = %q, want %q", obs.Tool, tc.wantTool)
			}
			if obs.Release != nil {
				t.Errorf("Release = %v, want nil", *obs.Release)
			}
		})
	}
}

// --- helpers ---

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

// assertDetectionError asserts the error is a *DetectionError with the expected reason.
func assertDetectionError(t *testing.T, err error, reason string) *DetectionError {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error with reason %q, got nil", reason)
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	switch reason {
	case "unknown":
		if !detErr.IsUnknown() {
			t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
		}
	case "ambiguous":
		if !detErr.IsAmbiguous() {
			t.Errorf("expected reason 'ambiguous', got %q", detErr.Reason)
		}
	default:
		t.Fatalf("unsupported reason %q in assertDetectionError", reason)
	}

	return detErr
}

// assertEvidencePath checks that evidence contains an entry with the given path and key "build.file".
func assertEvidencePath(t *testing.T, evidence []EvidenceItem, path string) {
	t.Helper()

	for _, e := range evidence {
		if e.Path == path && e.Key == "build.file" {
			return
		}
	}
	t.Errorf("expected evidence for path %q, got %+v", path, evidence)
}

func writeDetectFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
