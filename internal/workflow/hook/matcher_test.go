package hook

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestMatch_StackFilterSemantics(t *testing.T) {
	t.Parallel()

	base := Spec{
		ID:    "stack-filter",
		Steps: []Step{{Image: contracts.JobImage{Universal: "ghcr.io/example/hook:1"}}},
	}

	tests := []struct {
		name   string
		filter StackFilter
		stack  RuntimeStack
		want   bool
	}{
		{
			name: "exact match ignores language and tool case",
			filter: StackFilter{
				Language: "java",
				Tool:     "maven",
				Release:  "17",
			},
			stack: RuntimeStack{Language: "JAVA", Tool: "MAVEN", Release: "17"},
			want:  true,
		},
		{
			name: "empty release in filter is wildcard",
			filter: StackFilter{
				Language: "java",
				Tool:     "gradle",
			},
			stack: RuntimeStack{Language: "java", Tool: "gradle", Release: "21"},
			want:  true,
		},
		{
			name: "release comparison is exact after trim",
			filter: StackFilter{
				Language: "java",
				Tool:     "maven",
				Release:  "17-LTS",
			},
			stack: RuntimeStack{Language: "java", Tool: "maven", Release: "17-lts"},
			want:  false,
		},
		{
			name: "tool mismatch blocks execution",
			filter: StackFilter{
				Language: "java",
				Tool:     "maven",
			},
			stack: RuntimeStack{Language: "java", Tool: "gradle", Release: "17"},
			want:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := base
			spec.Stack = tc.filter

			got, err := Match(spec, MatchInput{Stack: tc.stack})
			if err != nil {
				t.Fatalf("Match() error = %v", err)
			}
			if got.ShouldRun != tc.want {
				t.Fatalf("ShouldRun = %v, want %v", got.ShouldRun, tc.want)
			}
			if got.StackMatched != tc.want {
				t.Fatalf("StackMatched = %v, want %v", got.StackMatched, tc.want)
			}
			if !got.SBOMMatched {
				t.Fatalf("SBOMMatched = false, want true when no sbom predicates are configured")
			}
		})
	}
}

func TestMatch_SBOMPredicates_AllTransitionTypes(t *testing.T) {
	t.Parallel()

	spec := Spec{
		ID: "sbom-predicates",
		SBOM: SBOMConditions{
			OnMatch: []SBOMPackageCondition{{Name: "pkg-match", Version: ">=1.10.0"}},
			OnAdd:   []SBOMPackageCondition{{Name: "pkg-added", Version: ">=2.0.0"}},
			OnRemove: []SBOMPackageCondition{{
				Name: "pkg-removed",
			}},
			OnChange: []SBOMChangeCondition{{
				Name: "pkg-change",
				From: "<2.0.0",
				To:   ">=2.0.0",
			}},
		},
		Steps: []Step{{Image: contracts.JobImage{Universal: "ghcr.io/example/hook:1"}}},
	}

	current := []SBOMPackage{
		{Name: "PKG-MATCH", Version: "1.10.0"},
		{Name: "pkg-added", Version: "2.0.0"},
		{Name: "pkg-change", Version: "2.1.0"},
	}
	previous := []SBOMPackage{
		{Name: "pkg-match", Version: "1.2.0"},
		{Name: "pkg-removed", Version: "1.0.0"},
		{Name: "pkg-change", Version: "1.9.9"},
	}

	got, err := Match(spec, MatchInput{
		Stack:        RuntimeStack{Language: "java", Tool: "maven", Release: "17"},
		CurrentSBOM:  current,
		PreviousSBOM: previous,
	})
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}

	if !got.ShouldRun {
		t.Fatalf("ShouldRun = false, want true")
	}
	if !got.SBOMMatched {
		t.Fatalf("SBOMMatched = false, want true")
	}
	if !got.Predicates.OnMatch {
		t.Fatalf("Predicates.OnMatch = false, want true")
	}
	if !got.Predicates.OnAdd {
		t.Fatalf("Predicates.OnAdd = false, want true")
	}
	if !got.Predicates.OnRemove {
		t.Fatalf("Predicates.OnRemove = false, want true")
	}
	if !got.Predicates.OnChange {
		t.Fatalf("Predicates.OnChange = false, want true")
	}
}

func TestMatch_OnChangeUsesNormalizedVersionComparison(t *testing.T) {
	t.Parallel()

	spec := Spec{
		ID: "on-change-normalized",
		SBOM: SBOMConditions{
			OnChange: []SBOMChangeCondition{{
				Name: "pkg-change",
				From: ">=1.0.0",
				To:   ">=1.0.0",
			}},
		},
		Steps: []Step{{Image: contracts.JobImage{Universal: "ghcr.io/example/hook:1"}}},
	}

	got, err := Match(spec, MatchInput{
		Stack: RuntimeStack{Language: "java", Tool: "maven", Release: "17"},
		CurrentSBOM: []SBOMPackage{
			{Name: "pkg-change", Version: "1.0.0"},
		},
		PreviousSBOM: []SBOMPackage{
			{Name: "pkg-change", Version: "1.0"},
		},
	})
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}

	if got.Predicates.OnChange {
		t.Fatalf("Predicates.OnChange = true, want false for normalized-equal versions")
	}
	if got.SBOMMatched {
		t.Fatalf("SBOMMatched = true, want false")
	}
	if got.ShouldRun {
		t.Fatalf("ShouldRun = true, want false")
	}
}

func TestMatch_OnChange_GradlePluginCoordinateTransition(t *testing.T) {
	t.Parallel()

	spec := Spec{
		ID: "openapi-gradle-plugin-upgrade",
		SBOM: SBOMConditions{
			OnChange: []SBOMChangeCondition{{
				Name: "org.openapi.generator:org.openapi.generator.gradle.plugin",
				From: "<5.0.0",
				To:   ">=5.0.0",
			}},
		},
		Steps: []Step{{Image: contracts.JobImage{Universal: "ghcr.io/example/hook:1"}}},
	}

	got, err := Match(spec, MatchInput{
		Stack: RuntimeStack{Language: "java", Tool: "gradle", Release: "17"},
		CurrentSBOM: []SBOMPackage{
			{Name: "org.openapi.generator:org.openapi.generator.gradle.plugin", Version: "6.6.0"},
		},
		PreviousSBOM: []SBOMPackage{
			{Name: "org.openapi.generator:org.openapi.generator.gradle.plugin", Version: "4.3.0"},
		},
	})
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if !got.Predicates.OnChange {
		t.Fatalf("Predicates.OnChange = false, want true")
	}
	if !got.SBOMMatched {
		t.Fatalf("SBOMMatched = false, want true")
	}
	if !got.ShouldRun {
		t.Fatalf("ShouldRun = false, want true")
	}
}

func TestMatch_DeterministicHashAndOnceEligibility(t *testing.T) {
	t.Parallel()

	spec := Spec{
		ID:   " hook-deterministic ",
		Once: true,
		Stack: StackFilter{
			Language: " java ",
			Tool:     " maven ",
			Release:  " 17 ",
		},
		SBOM: SBOMConditions{
			OnMatch: []SBOMPackageCondition{{
				Name:    " lib-a ",
				Version: " >=1.0.0 ",
			}},
		},
		Steps: []Step{{
			Name:    " scan ",
			Image:   contracts.JobImage{Universal: " ghcr.io/example/hook:1 "},
			Command: []string{" run ", " --flag "},
			Envs: map[string]string{
				" B ": "2",
				" A ": "1",
			},
		}},
		Source: "/tmp/hooks/hook.yaml",
	}

	input := MatchInput{
		Stack: RuntimeStack{Language: "java", Tool: "maven", Release: "17"},
		CurrentSBOM: []SBOMPackage{
			{Name: "lib-a", Version: "1.2.0"},
		},
	}

	first, err := Match(spec, input)
	if err != nil {
		t.Fatalf("first Match() error = %v", err)
	}
	second, err := Match(spec, input)
	if err != nil {
		t.Fatalf("second Match() error = %v", err)
	}

	if first.HookHash == "" {
		t.Fatal("HookHash is empty")
	}
	if first.HookHash != second.HookHash {
		t.Fatalf("HookHash changed across identical evaluations: %q vs %q", first.HookHash, second.HookHash)
	}
	if !first.Once.Enabled {
		t.Fatalf("Once.Enabled = false, want true")
	}
	if !first.Once.Eligible {
		t.Fatalf("Once.Eligible = false, want true")
	}
	if first.Once.PersistenceKey != first.HookHash {
		t.Fatalf("Once.PersistenceKey = %q, want hook hash %q", first.Once.PersistenceKey, first.HookHash)
	}

	notEligible, err := Match(spec, MatchInput{
		Stack: RuntimeStack{Language: "go", Tool: "", Release: "1.22"},
		CurrentSBOM: []SBOMPackage{
			{Name: "lib-a", Version: "1.2.0"},
		},
	})
	if err != nil {
		t.Fatalf("stack-mismatch Match() error = %v", err)
	}
	if notEligible.ShouldRun {
		t.Fatalf("ShouldRun = true, want false")
	}
	if notEligible.Once.Eligible {
		t.Fatalf("Once.Eligible = true, want false")
	}
	if notEligible.Once.PersistenceKey != first.HookHash {
		t.Fatalf("Once.PersistenceKey changed: %q vs %q", notEligible.Once.PersistenceKey, first.HookHash)
	}

	mutated := spec
	mutated.ID = "hook-deterministic-v2"
	mutatedHash, err := HookHash(mutated)
	if err != nil {
		t.Fatalf("HookHash(mutated) error = %v", err)
	}
	if mutatedHash == first.HookHash {
		t.Fatalf("HookHash should change when canonical hook content changes")
	}
}
