package mods

import "testing"

func TestNormalizeStepType(t *testing.T) {
	cases := []struct {
		in    string
		out   StepType
		valid bool
	}{
		{"llm-exec", StepTypeLLMExec, true},
		{"orw-gen", StepTypeORWGen, true},
		{"orw-apply", StepTypeORWApply, true},
		{"human-step", StepTypeHumanStep, true},
		{"human", StepTypeHumanStep, true}, // alias normalized
		{"unknown-x", StepType("unknown-x"), false},
	}

	for _, c := range cases {
		got := NormalizeStepType(c.in)
		if got != c.out {
			t.Fatalf("NormalizeStepType(%q) = %q, want %q", c.in, got, c.out)
		}
		if got.IsValid() != c.valid {
			t.Fatalf("IsValid(%q) = %v, want %v", got, got.IsValid(), c.valid)
		}
	}
}
