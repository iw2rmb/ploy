package validation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// SchemaValidator validates recipes against a schema definition
type SchemaValidator struct {
	version string
}

// NewSchemaValidator creates a new schema validator
func NewSchemaValidator() *SchemaValidator {
	return &SchemaValidator{
		version: "1.0.0",
	}
}

// GetVersion returns the schema version
func (v *SchemaValidator) GetVersion() string {
	return v.version
}

// Validate validates a recipe against the schema
func (v *SchemaValidator) Validate(recipe *models.Recipe) error {
	if recipe == nil {
		return fmt.Errorf("recipe is nil")
	}

	// Validate required fields
	if err := v.validateRequiredFields(recipe); err != nil {
		return err
	}

	// Validate field types
	if err := v.validateFieldTypes(recipe); err != nil {
		return err
	}

	// Validate field constraints
	if err := v.validateFieldConstraints(recipe); err != nil {
		return err
	}

	// Validate metadata
	if err := v.validateMetadata(&recipe.Metadata); err != nil {
		return fmt.Errorf("metadata validation failed: %w", err)
	}

	// Validate steps
	if err := v.validateSteps(recipe.Steps); err != nil {
		return fmt.Errorf("steps validation failed: %w", err)
	}

	// Validate execution config
	if err := v.validateExecutionConfig(&recipe.Execution); err != nil {
		return fmt.Errorf("execution config validation failed: %w", err)
	}

	// Validate validation rules
	if err := v.validateValidationRules(&recipe.Validation); err != nil {
		return fmt.Errorf("validation rules validation failed: %w", err)
	}

	return nil
}

// validateRequiredFields checks that all required fields are present
func (v *SchemaValidator) validateRequiredFields(recipe *models.Recipe) error {
	// Check metadata required fields
	if recipe.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if recipe.Metadata.Description == "" {
		return fmt.Errorf("metadata.description is required")
	}

	if recipe.Metadata.Author == "" {
		return fmt.Errorf("metadata.author is required")
	}

	// Check steps
	if len(recipe.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	return nil
}

// validateFieldTypes checks that fields have correct types
func (v *SchemaValidator) validateFieldTypes(recipe *models.Recipe) error {
	// Use reflection to check field types
	recipeType := reflect.TypeOf(*recipe)
	recipeValue := reflect.ValueOf(*recipe)

	for i := 0; i < recipeType.NumField(); i++ {
		field := recipeType.Field(i)
		value := recipeValue.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Check for nil pointers
		if value.Kind() == reflect.Ptr && value.IsNil() {
			// Some fields can be nil
			continue
		}

		// Validate specific field types
		switch field.Name {
		case "Steps":
			if value.Kind() != reflect.Slice {
				return fmt.Errorf("steps must be an array")
			}
		case "Metadata":
			if value.Kind() != reflect.Struct {
				return fmt.Errorf("metadata must be an object")
			}
		}
	}

	return nil
}

// validateFieldConstraints checks field value constraints
func (v *SchemaValidator) validateFieldConstraints(recipe *models.Recipe) error {
	// Validate name length
	if len(recipe.Metadata.Name) < 2 || len(recipe.Metadata.Name) > 63 {
		return fmt.Errorf("metadata.name must be between 2 and 63 characters")
	}

	// Validate description length
	if len(recipe.Metadata.Description) > 500 {
		return fmt.Errorf("metadata.description must not exceed 500 characters")
	}

	// Validate step count
	if len(recipe.Steps) > 100 {
		return fmt.Errorf("recipe cannot have more than 100 steps")
	}

	// Validate tag count
	if len(recipe.Metadata.Tags) > 20 {
		return fmt.Errorf("recipe cannot have more than 20 tags")
	}

	// Validate category count
	if len(recipe.Metadata.Categories) > 10 {
		return fmt.Errorf("recipe cannot have more than 10 categories")
	}

	return nil
}

// validateMetadata validates recipe metadata
func (v *SchemaValidator) validateMetadata(metadata *models.RecipeMetadata) error {
	// Validate name format
	if !isValidName(metadata.Name) {
		return fmt.Errorf("invalid name format: must be lowercase alphanumeric with hyphens")
	}

	// Validate version format if provided
	if metadata.Version != "" && !isValidVersion(metadata.Version) {
		return fmt.Errorf("invalid version format: must follow semantic versioning")
	}

	// Validate URLs if provided
	if metadata.Homepage != "" && !isValidURL(metadata.Homepage) {
		return fmt.Errorf("invalid homepage URL format")
	}

	if metadata.Repository != "" && !isValidURL(metadata.Repository) {
		return fmt.Errorf("invalid repository URL format")
	}

	// Validate tags
	for _, tag := range metadata.Tags {
		if !isValidTag(tag) {
			return fmt.Errorf("invalid tag format: %s", tag)
		}
	}

	// Validate categories
	for _, category := range metadata.Categories {
		if !isValidCategory(category) {
			return fmt.Errorf("invalid category: %s", category)
		}
	}

	// Validate languages
	for _, language := range metadata.Languages {
		if !isValidLanguage(language) {
			return fmt.Errorf("invalid language: %s", language)
		}
	}

	return nil
}

// validateSteps validates recipe steps
func (v *SchemaValidator) validateSteps(steps []models.RecipeStep) error {
	if len(steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	stepNames := make(map[string]bool)

	for i, step := range steps {
		// Check for duplicate step names
		if step.Name != "" {
			if stepNames[step.Name] {
				return fmt.Errorf("duplicate step name: %s", step.Name)
			}
			stepNames[step.Name] = true
		}

		// Validate step type
		if !isValidStepType(step.Type) {
			return fmt.Errorf("step %d has invalid type: %s", i+1, step.Type)
		}

		// Validate step configuration
		if len(step.Config) == 0 {
			return fmt.Errorf("step %d (%s) missing configuration", i+1, step.Name)
		}

		// Validate error handling action
		if step.OnError != "" && !isValidErrorAction(step.OnError) {
			return fmt.Errorf("step %d has invalid error action: %s", i+1, step.OnError)
		}

		// Validate timeout
		if step.Timeout.Duration < 0 {
			return fmt.Errorf("step %d has negative timeout", i+1)
		}
	}

	return nil
}

// validateExecutionConfig validates execution configuration
func (v *SchemaValidator) validateExecutionConfig(config *models.ExecutionConfig) error {
	// Validate parallelism
	if config.Parallelism < 0 || config.Parallelism > 10 {
		return fmt.Errorf("parallelism must be between 0 and 10")
	}

	// Validate max duration
	if config.MaxDuration.Duration < 0 {
		return fmt.Errorf("max_duration cannot be negative")
	}

	// Validate sandbox config if enabled
	if config.Sandbox.Enabled {
		if err := v.validateSandboxConfig(&config.Sandbox); err != nil {
			return fmt.Errorf("sandbox config validation failed: %w", err)
		}
	}

	return nil
}

// validateSandboxConfig validates sandbox configuration
func (v *SchemaValidator) validateSandboxConfig(sandbox *models.SandboxConfig) error {
	// Validate memory limit format
	if sandbox.MaxMemory != "" && !isValidMemorySize(sandbox.MaxMemory) {
		return fmt.Errorf("invalid memory size format: %s", sandbox.MaxMemory)
	}

	// Validate CPU limit
	if sandbox.MaxCPU < 0 || sandbox.MaxCPU > 16 {
		return fmt.Errorf("max_cpu must be between 0 and 16")
	}

	// Validate disk usage format
	if sandbox.MaxDiskUsage != "" && !isValidDiskSize(sandbox.MaxDiskUsage) {
		return fmt.Errorf("invalid disk size format: %s", sandbox.MaxDiskUsage)
	}

	// Validate isolation level
	if sandbox.IsolationLevel != "" && !isValidIsolationLevel(sandbox.IsolationLevel) {
		return fmt.Errorf("invalid isolation level: %s", sandbox.IsolationLevel)
	}

	return nil
}

// validateValidationRules validates validation rules
func (v *SchemaValidator) validateValidationRules(rules *models.ValidationRules) error {
	// Validate file count
	if rules.MinFileCount < 0 {
		return fmt.Errorf("min_file_count cannot be negative")
	}

	// Validate repo size
	if rules.MaxRepoSize < 0 {
		return fmt.Errorf("max_repo_size cannot be negative")
	}

	// Validate language detection confidence
	if rules.LanguageDetection.MinConfidence < 0 || rules.LanguageDetection.MinConfidence > 1 {
		return fmt.Errorf("min_confidence must be between 0 and 1")
	}

	return nil
}

// Helper validation functions

func isValidName(name string) bool {
	if len(name) < 2 || len(name) > 63 {
		return false
	}

	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}

	return true
}

func isValidVersion(version string) bool {
	// Simple semantic version check
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}

	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}

	return true
}

func isValidURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func isValidTag(tag string) bool {
	if len(tag) == 0 || len(tag) > 50 {
		return false
	}

	for _, r := range tag {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != ' ' {
			return false
		}
	}

	return true
}

func isValidCategory(category string) bool {
	validCategories := []string{
		"language-upgrade",
		"framework-migration",
		"security-fix",
		"code-cleanup",
		"performance-optimization",
		"api-migration",
		"dependency-update",
		"modernization",
		"refactoring",
		"testing",
		"documentation",
		"formatting",
		"best-practices",
	}

	for _, valid := range validCategories {
		if strings.EqualFold(category, valid) {
			return true
		}
	}

	return false
}

func isValidLanguage(language string) bool {
	validLanguages := []string{
		"java", "kotlin", "groovy", "scala",
		"go", "golang",
		"python", "python2", "python3",
		"javascript", "typescript", "js", "ts",
		"c", "cpp", "c++",
		"rust",
		"ruby",
		"php",
		"swift",
		"csharp", "c#",
		"shell", "bash", "sh",
	}

	for _, valid := range validLanguages {
		if strings.EqualFold(language, valid) {
			return true
		}
	}

	return false
}

func isValidStepType(stepType models.RecipeStepType) bool {
	validTypes := []models.RecipeStepType{
		models.StepTypeOpenRewrite,
		models.StepTypeShellScript,
		models.StepTypeFileOperation,
		models.StepTypeRegexReplace,
		models.StepTypeASTTransform,
		models.StepTypeComposite,
	}

	for _, valid := range validTypes {
		if stepType == valid {
			return true
		}
	}

	return false
}

func isValidErrorAction(action models.ErrorHandlingAction) bool {
	validActions := []models.ErrorHandlingAction{
		models.ErrorActionContinue,
		models.ErrorActionRollback,
		models.ErrorActionFail,
	}

	for _, valid := range validActions {
		if action == valid {
			return true
		}
	}

	return false
}

func isValidMemorySize(size string) bool {
	// Check format like "512MB", "2GB"
	if len(size) < 2 {
		return false
	}

	validUnits := []string{"B", "KB", "MB", "GB", "TB"}
	for _, unit := range validUnits {
		if strings.HasSuffix(size, unit) {
			numStr := size[:len(size)-len(unit)]
			for _, r := range numStr {
				if r < '0' || r > '9' {
					return false
				}
			}
			return true
		}
	}

	return false
}

func isValidDiskSize(size string) bool {
	// Same format as memory size
	return isValidMemorySize(size)
}

func isValidIsolationLevel(level string) bool {
	validLevels := []string{"none", "low", "medium", "high", "strict"}
	for _, valid := range validLevels {
		if level == valid {
			return true
		}
	}
	return false
}
