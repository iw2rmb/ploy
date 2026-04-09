package handlers

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestSBOMStackTupleFromSpec(t *testing.T) {
	t.Parallel()

	spec := &contracts.MigSpec{
		BuildGate: &contracts.BuildGateConfig{
			Pre: &contracts.BuildGatePhaseConfig{
				Stack: &contracts.BuildGateStackConfig{
					Enabled:  true,
					Language: "Java",
					Tool:     "Maven",
					Release:  "17",
				},
			},
			Post: &contracts.BuildGatePhaseConfig{
				Stack: &contracts.BuildGateStackConfig{
					Enabled:  true,
					Language: "Java",
					Tool:     "Gradle",
					Release:  "21",
				},
			},
		},
	}

	lang, tool, release := sbomStackTupleFromSpec(spec, contracts.SBOMPhasePre)
	if lang != "java" || tool != "maven" || release != "17" {
		t.Fatalf("pre tuple = (%q,%q,%q), want (java,maven,17)", lang, tool, release)
	}

	lang, tool, release = sbomStackTupleFromSpec(spec, contracts.SBOMPhasePost)
	if lang != "java" || tool != "gradle" || release != "21" {
		t.Fatalf("post tuple = (%q,%q,%q), want (java,gradle,21)", lang, tool, release)
	}
}
