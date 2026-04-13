package nodeagent

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveManifestStack_PrefersDetectedStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      StartRunRequest
		fallback contracts.MigStack
		want     contracts.MigStack
	}{
		{
			name: "maven from detected stack",
			req: StartRunRequest{
				DetectedStack: &contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			},
			fallback: contracts.MigStackJavaGradle,
			want:     contracts.MigStackJavaMaven,
		},
		{
			name: "gradle from detected stack",
			req: StartRunRequest{
				DetectedStack: &contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "17"},
			},
			fallback: contracts.MigStackJavaMaven,
			want:     contracts.MigStackJavaGradle,
		},
		{
			name: "unknown tuple falls back",
			req: StartRunRequest{
				DetectedStack: &contracts.StackExpectation{Language: "python", Tool: "pip", Release: "3.11"},
			},
			fallback: contracts.MigStackJavaGradle,
			want:     contracts.MigStackJavaGradle,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveManifestStack(tc.req, tc.fallback); got != tc.want {
				t.Fatalf("resolveManifestStack()=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestInjectStackTupleEnv(t *testing.T) {
	t.Parallel()

	env := map[string]string{}
	injectStackTupleEnv(env, &contracts.StackExpectation{
		Language: "java",
		Tool:     "maven",
		Release:  "21",
	})

	if got := env[contracts.PLOYStackLanguageEnv]; got != "java" {
		t.Fatalf("%s=%q, want java", contracts.PLOYStackLanguageEnv, got)
	}
	if got := env[contracts.PLOYStackToolEnv]; got != "maven" {
		t.Fatalf("%s=%q, want maven", contracts.PLOYStackToolEnv, got)
	}
	if got := env[contracts.PLOYStackReleaseEnv]; got != "21" {
		t.Fatalf("%s=%q, want 21", contracts.PLOYStackReleaseEnv, got)
	}
}
