package mods

import (
	"context"
	"testing"
)

func TestMRAppendHelpers(t *testing.T) {
	res := &ModResult{}
	mrAppendFailure(res, nil) // no-op
	if len(res.StepResults) != 0 {
		t.Fatalf("expected no steps yet")
	}
	mrAppendFailure(res, assertErr("boom"))
	if len(res.StepResults) != 1 || res.StepResults[0].StepID != "mr" {
		t.Fatalf("failure step missing")
	}
	mrAppendSuccess(res, "http://example/mr/1", true)
	if res.MRURL == "" || len(res.StepResults) != 2 {
		t.Fatalf("success step missing")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestMREmitStart_NoPanic(t *testing.T) {
	mrEmitStart(nil, context.Background(), "src", "dst") // no-op
}
