package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestGatePhasePolicyForJobSpec_StrictStackLookup(t *testing.T) {
	t.Parallel()

	spec := []byte(`{
		"steps":[{"image":"img:1"}],
		"build_gate":{
			"pre":{"stack":{"enabled":true,"language":"java","tool":"maven","release":"17","default":false}}
		}
	}`)

	policy, err := gatePhasePolicyForJobSpec(spec, domaintypes.JobTypePreGate)
	if err != nil {
		t.Fatalf("gatePhasePolicyForJobSpec() error = %v", err)
	}
	if policy.LookupConstraints.StrictStack == nil {
		t.Fatal("expected strict stack lookup constraints")
	}
	if got := policy.LookupConstraints.StrictStack.Language; got != "java" {
		t.Fatalf("strict language = %q, want %q", got, "java")
	}
	if got := policy.LookupConstraints.StrictStack.Tool; got != "maven" {
		t.Fatalf("strict tool = %q, want %q", got, "maven")
	}
	if got := policy.LookupConstraints.StrictStack.Release; got != "17" {
		t.Fatalf("strict release = %q, want %q", got, "17")
	}
}

func TestGatePhasePolicyForJobSpec_NonStrictStackSkipsLookupConstraint(t *testing.T) {
	t.Parallel()

	spec := []byte(`{
		"steps":[{"image":"img:1"}],
		"build_gate":{
			"post":{"stack":{"enabled":true,"language":"java","tool":"gradle","release":"17","default":true}}
		}
	}`)

	policy, err := gatePhasePolicyForJobSpec(spec, domaintypes.JobTypePostGate)
	if err != nil {
		t.Fatalf("gatePhasePolicyForJobSpec() error = %v", err)
	}
	if policy.LookupConstraints.StrictStack != nil {
		t.Fatalf("expected nil strict stack constraints, got %+v", policy.LookupConstraints.StrictStack)
	}
}
