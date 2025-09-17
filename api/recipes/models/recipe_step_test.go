package models

import "testing"

func TestRecipeStepValidate(t *testing.T) {
	step := RecipeStep{
		Name: "openrewrite",
		Type: StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.SpringBoot3Upgrade",
		},
		OnError: ErrorActionContinue,
	}
	if err := step.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	missingName := step
	missingName.Name = ""
	if err := missingName.Validate(); err == nil {
		t.Fatalf("expected error for missing name")
	}

	invalidType := step
	invalidType.Type = "unknown"
	if err := invalidType.Validate(); err == nil {
		t.Fatalf("expected error for invalid type")
	}

	invalidConfig := RecipeStep{Name: "shell", Type: StepTypeShellScript, Config: map[string]interface{}{}}
	if err := invalidConfig.Validate(); err == nil {
		t.Fatalf("expected error for missing shell script config")
	}

	invalidOnError := step
	invalidOnError.OnError = "retry"
	if err := invalidOnError.Validate(); err == nil {
		t.Fatalf("expected error for invalid on_error action")
	}
}

func TestExecutionConditionValidate(t *testing.T) {
	cond := ExecutionCondition{Type: ConditionFileExists, Value: "pom.xml"}
	if err := cond.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	invalidType := ExecutionCondition{Type: "unknown", Value: ""}
	if err := invalidType.Validate(); err == nil {
		t.Fatalf("expected error for invalid condition type")
	}

	invalidValue := ExecutionCondition{Type: ConditionFileExists, Value: 123}
	if err := invalidValue.Validate(); err == nil {
		t.Fatalf("expected error for invalid value type")
	}

	invalidVersion := ExecutionCondition{Type: ConditionMinJavaVersion, Value: []string{"17"}}
	if err := invalidVersion.Validate(); err == nil {
		t.Fatalf("expected error for invalid version value")
	}
}
