package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RecipeStepType defines supported transformation types
type RecipeStepType string

const (
	StepTypeOpenRewrite   RecipeStepType = "openrewrite"
	StepTypeShellScript   RecipeStepType = "shell"
	StepTypeFileOperation RecipeStepType = "file_op"
	StepTypeRegexReplace  RecipeStepType = "regex"
	StepTypeASTTransform  RecipeStepType = "ast_transform"
	StepTypeComposite     RecipeStepType = "composite"
)

// ErrorHandlingAction defines how to handle step errors
type ErrorHandlingAction string

const (
	ErrorActionContinue ErrorHandlingAction = "continue"
	ErrorActionRollback ErrorHandlingAction = "rollback"
	ErrorActionFail     ErrorHandlingAction = "fail"
)

// RecipeStep defines a single transformation operation
type RecipeStep struct {
	Name       string                 `json:"name" yaml:"name"`
	Type       RecipeStepType         `json:"type" yaml:"type"`
	Config     map[string]interface{} `json:"config" yaml:"config"`
	Conditions []ExecutionCondition   `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	OnError    ErrorHandlingAction    `json:"on_error,omitempty" yaml:"on_error,omitempty"`
	Timeout    Duration               `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// ExecutionCondition defines when a step should be executed
type ExecutionCondition struct {
	Type  ConditionType `json:"type" yaml:"type"`
	Value interface{}   `json:"value" yaml:"value"`
}

// ConditionType defines types of execution conditions
type ConditionType string

const (
	ConditionFileExists     ConditionType = "file_exists"
	ConditionFileNotExists  ConditionType = "file_not_exists"
	ConditionLanguage       ConditionType = "language"
	ConditionMinJavaVersion ConditionType = "min_java_version"
	ConditionMaxJavaVersion ConditionType = "max_java_version"
	ConditionFramework      ConditionType = "framework"
	ConditionEnvVar         ConditionType = "env_var"
	ConditionCustom         ConditionType = "custom"
)

// Duration wraps time.Duration to provide custom YAML marshaling
type Duration struct {
	time.Duration
}

// MarshalYAML converts Duration to a string for YAML
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// UnmarshalYAML parses a duration string from YAML
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}

	d.Duration = duration
	return nil
}

// MarshalJSON converts Duration to a string for JSON
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON parses a duration string from JSON
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}

	d.Duration = duration
	return nil
}

// Validate validates a recipe step
func (s *RecipeStep) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("step name is required")
	}

	if !s.Type.IsValid() {
		return fmt.Errorf("invalid step type: %s", s.Type)
	}

	// Validate configuration based on step type
	if err := s.validateConfig(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Validate error handling action
	if s.OnError != "" && !s.OnError.IsValid() {
		return fmt.Errorf("invalid error handling action: %s", s.OnError)
	}

	// Validate conditions
	for _, condition := range s.Conditions {
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("condition validation failed: %w", err)
		}
	}

	// Validate timeout
	if s.Timeout.Duration < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}

	return nil
}

// validateConfig validates step configuration based on step type
func (s *RecipeStep) validateConfig() error {
	switch s.Type {
	case StepTypeOpenRewrite:
		return s.validateOpenRewriteConfig()
	case StepTypeShellScript:
		return s.validateShellScriptConfig()
	case StepTypeFileOperation:
		return s.validateFileOperationConfig()
	case StepTypeRegexReplace:
		return s.validateRegexConfig()
	case StepTypeASTTransform:
		return s.validateASTConfig()
	case StepTypeComposite:
		return s.validateCompositeConfig()
	default:
		return fmt.Errorf("unknown step type: %s", s.Type)
	}
}

func (s *RecipeStep) validateOpenRewriteConfig() error {
	recipe, ok := s.Config["recipe"].(string)
	if !ok || recipe == "" {
		return fmt.Errorf("openrewrite step requires 'recipe' in config")
	}
	return nil
}

func (s *RecipeStep) validateShellScriptConfig() error {
	script, ok := s.Config["script"].(string)
	if !ok || script == "" {
		return fmt.Errorf("shell step requires 'script' in config")
	}
	return nil
}

func (s *RecipeStep) validateFileOperationConfig() error {
	operation, ok := s.Config["operation"].(string)
	if !ok || operation == "" {
		return fmt.Errorf("file_op step requires 'operation' in config")
	}

	validOps := []string{"create", "delete", "copy", "move", "chmod"}
	valid := false
	for _, op := range validOps {
		if operation == op {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid file operation: %s", operation)
	}

	return nil
}

func (s *RecipeStep) validateRegexConfig() error {
	pattern, ok := s.Config["pattern"].(string)
	if !ok || pattern == "" {
		return fmt.Errorf("regex step requires 'pattern' in config")
	}

	_, ok = s.Config["replacement"].(string)
	if !ok {
		return fmt.Errorf("regex step requires 'replacement' in config")
	}

	return nil
}

func (s *RecipeStep) validateASTConfig() error {
	language, ok := s.Config["language"].(string)
	if !ok || language == "" {
		return fmt.Errorf("ast_transform step requires 'language' in config")
	}

	transform, ok := s.Config["transform"].(string)
	if !ok || transform == "" {
		return fmt.Errorf("ast_transform step requires 'transform' in config")
	}

	return nil
}

func (s *RecipeStep) validateCompositeConfig() error {
	recipes, ok := s.Config["recipes"].([]interface{})
	if !ok || len(recipes) == 0 {
		return fmt.Errorf("composite step requires 'recipes' array in config")
	}
	return nil
}

// IsValid checks if the step type is valid
func (t RecipeStepType) IsValid() bool {
	validTypes := []RecipeStepType{
		StepTypeOpenRewrite,
		StepTypeShellScript,
		StepTypeFileOperation,
		StepTypeRegexReplace,
		StepTypeASTTransform,
		StepTypeComposite,
	}

	for _, valid := range validTypes {
		if t == valid {
			return true
		}
	}
	return false
}

// IsValid checks if the error handling action is valid
func (a ErrorHandlingAction) IsValid() bool {
	validActions := []ErrorHandlingAction{
		ErrorActionContinue,
		ErrorActionRollback,
		ErrorActionFail,
	}

	for _, valid := range validActions {
		if a == valid {
			return true
		}
	}
	return false
}

// Validate validates an execution condition
func (c *ExecutionCondition) Validate() error {
	if !c.Type.IsValid() {
		return fmt.Errorf("invalid condition type: %s", c.Type)
	}

	// Validate value based on condition type
	switch c.Type {
	case ConditionFileExists, ConditionFileNotExists:
		if _, ok := c.Value.(string); !ok {
			return fmt.Errorf("file condition requires string value")
		}
	case ConditionLanguage, ConditionFramework:
		if _, ok := c.Value.(string); !ok {
			return fmt.Errorf("%s condition requires string value", c.Type)
		}
	case ConditionMinJavaVersion, ConditionMaxJavaVersion:
		switch v := c.Value.(type) {
		case string:
			// Validate version format
			if !strings.Contains(v, ".") && !isNumericVersion(v) {
				return fmt.Errorf("invalid version format: %s", v)
			}
		case int, float64:
			// Numeric versions are OK
		default:
			return fmt.Errorf("version condition requires string or number value")
		}
	}

	return nil
}

// IsValid checks if the condition type is valid
func (t ConditionType) IsValid() bool {
	validTypes := []ConditionType{
		ConditionFileExists,
		ConditionFileNotExists,
		ConditionLanguage,
		ConditionMinJavaVersion,
		ConditionMaxJavaVersion,
		ConditionFramework,
		ConditionEnvVar,
		ConditionCustom,
	}

	for _, valid := range validTypes {
		if t == valid {
			return true
		}
	}
	return false
}

func isNumericVersion(v string) bool {
	// Check if it's a simple numeric version like "11", "17", "21"
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
