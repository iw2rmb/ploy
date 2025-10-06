package runner_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

const (
	modsPlanStage     = mods.StageNamePlan
	buildGateStage    = "build-gate"
	staticChecksStage = "static-checks"
)

func setRunnerModsOptions(t *testing.T, opts *runner.Options, planTimeout time.Duration, maxParallel int) {
	t.Helper()
	value := reflect.ValueOf(opts).Elem()
	modsField := value.FieldByName("Mods")
	if !modsField.IsValid() {
		t.Fatalf("runner.Options missing Mods field: %#v", opts)
	}
	if modsField.Kind() != reflect.Struct {
		t.Fatalf("runner.Options Mods field is not a struct: %s", modsField.Kind())
	}
	planTimeoutField := modsField.FieldByName("PlanTimeout")
	if !planTimeoutField.IsValid() {
		t.Fatalf("runner.ModsOptions missing PlanTimeout: %#v", modsField.Interface())
	}
	if !planTimeoutField.CanSet() {
		t.Fatalf("runner.ModsOptions PlanTimeout not settable")
	}
	if planTimeoutField.Type() != reflect.TypeOf(time.Duration(0)) {
		t.Fatalf("runner.ModsOptions PlanTimeout has unexpected type: %s", planTimeoutField.Type())
	}
	planTimeoutField.Set(reflect.ValueOf(planTimeout))

	maxParallelField := modsField.FieldByName("MaxParallel")
	if !maxParallelField.IsValid() {
		t.Fatalf("runner.ModsOptions missing MaxParallel: %#v", modsField.Interface())
	}
	if !maxParallelField.CanSet() {
		t.Fatalf("runner.ModsOptions MaxParallel not settable")
	}
	if maxParallelField.Kind() != reflect.Int {
		t.Fatalf("runner.ModsOptions MaxParallel has unexpected kind: %s", maxParallelField.Kind())
	}
	maxParallelField.SetInt(int64(maxParallel))

	for field, value := range map[string]string{
		"PlanLane":        "mods-plan",
		"OpenRewriteLane": "mods-java",
		"LLMPlanLane":     "mods-llm",
		"LLMExecLane":     "mods-llm",
		"HumanLane":       "mods-human",
	} {
		laneField := modsField.FieldByName(field)
		if laneField.IsValid() && laneField.CanSet() && laneField.Kind() == reflect.String {
			laneField.SetString(value)
		}
	}
}
