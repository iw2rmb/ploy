package validation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/controller/arf/models"
	"github.com/iw2rmb/ploy/controller/arf/storage"
	"gopkg.in/yaml.v3"
)

// RecipeValidator ensures recipe safety and correctness
type RecipeValidator struct {
	securityRules   *storage.SecurityRuleSet
	schemaValidator *SchemaValidator
	enforceRules    bool
}

// NewRecipeValidator creates a new recipe validator
func NewRecipeValidator(securityRules *storage.SecurityRuleSet, enforceRules bool) *RecipeValidator {
	if securityRules == nil {
		// Set default security rules
		securityRules = GetDefaultSecurityRules()
	}

	return &RecipeValidator{
		securityRules:   securityRules,
		schemaValidator: NewSchemaValidator(),
		enforceRules:    enforceRules,
	}
}

// ValidateRecipe validates a complete recipe
func (v *RecipeValidator) ValidateRecipe(recipe *models.Recipe) error {
	// Basic validation
	if err := recipe.Validate(); err != nil {
		return fmt.Errorf("recipe validation failed: %w", err)
	}

	// Schema validation
	if err := v.ValidateSchema(recipe); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	// Security validation
	if v.enforceRules {
		if err := v.ValidateSecurityRules(recipe); err != nil {
			return fmt.Errorf("security validation failed: %w", err)
		}
	}

	return nil
}

// ValidateRecipeYAML validates and parses YAML recipe content
func (v *RecipeValidator) ValidateRecipeYAML(yamlContent []byte) (*models.Recipe, error) {
	var recipe models.Recipe
	if err := yaml.Unmarshal(yamlContent, &recipe); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := v.ValidateRecipe(&recipe); err != nil {
		return nil, err
	}

	return &recipe, nil
}

// ValidateRecipeJSON validates and parses JSON recipe content
func (v *RecipeValidator) ValidateRecipeJSON(jsonContent []byte) (*models.Recipe, error) {
	var recipe models.Recipe
	if err := json.Unmarshal(jsonContent, &recipe); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if err := v.ValidateRecipe(&recipe); err != nil {
		return nil, err
	}

	return &recipe, nil
}

// ValidateSecurityRules validates recipe against security rules
func (v *RecipeValidator) ValidateSecurityRules(recipe *models.Recipe) error {
	// Check execution time limits
	if recipe.Execution.MaxDuration.Duration > v.securityRules.MaxExecutionTime {
		return fmt.Errorf("recipe execution time exceeds maximum allowed: %v > %v",
			recipe.Execution.MaxDuration.Duration, v.securityRules.MaxExecutionTime)
	}

	// Check network access
	if recipe.Execution.Sandbox.AllowNetwork && !v.securityRules.AllowNetworkAccess {
		return fmt.Errorf("recipe requires network access which is not allowed")
	}

	// Check file system write
	if recipe.Execution.Sandbox.AllowFileWrite && !v.securityRules.AllowFileSystemWrite {
		return fmt.Errorf("recipe requires file system write which is not allowed")
	}

	// Check sandbox requirement
	if v.securityRules.SandboxRequired && !recipe.Execution.Sandbox.Enabled {
		return fmt.Errorf("recipe must be executed in a sandbox")
	}

	// Validate each step
	for i, step := range recipe.Steps {
		if err := v.ValidateStepSecurity(&step); err != nil {
			return fmt.Errorf("step %d (%s) failed security validation: %w", i+1, step.Name, err)
		}
	}

	return nil
}

// ValidateStepSecurity validates a single step's security
func (v *RecipeValidator) ValidateStepSecurity(step *models.RecipeStep) error {
	switch step.Type {
	case models.StepTypeShellScript:
		return v.validateShellScriptSecurity(step)
	case models.StepTypeFileOperation:
		return v.validateFileOperationSecurity(step)
	case models.StepTypeOpenRewrite:
		// OpenRewrite recipes are generally safe
		return nil
	case models.StepTypeRegexReplace:
		// Regex operations are generally safe
		return nil
	case models.StepTypeASTTransform:
		// AST transformations are generally safe
		return nil
	case models.StepTypeComposite:
		// Composite steps need recursive validation
		return v.validateCompositeSecurity(step)
	default:
		return fmt.Errorf("unknown step type: %s", step.Type)
	}
}

// validateShellScriptSecurity validates shell script security
func (v *RecipeValidator) validateShellScriptSecurity(step *models.RecipeStep) error {
	script, ok := step.Config["script"].(string)
	if !ok {
		return fmt.Errorf("shell step missing script configuration")
	}

	// Check for forbidden commands
	for _, forbidden := range v.securityRules.ForbiddenCommands {
		if strings.Contains(script, forbidden) {
			return fmt.Errorf("script contains forbidden command: %s", forbidden)
		}
	}

	// Check for dangerous patterns
	dangerousPatterns := []string{
		"rm -rf /",
		"rm -rf /*",
		"dd if=/dev/zero",
		":(){ :|:& };:",  // Fork bomb
		"fork bomb",
		"> /dev/sda",
		"mkfs.",
		"fdisk",
		"sudo",
		"su -",
		"chmod 777",
		"eval",
		"exec",
		"source /dev/stdin",
		"curl | sh",
		"wget -O - | sh",
	}

	scriptLower := strings.ToLower(script)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(scriptLower, strings.ToLower(pattern)) {
			return fmt.Errorf("script contains dangerous pattern: %s", pattern)
		}
	}

	// Check for command injection attempts
	if containsCommandInjection(script) {
		return fmt.Errorf("script may contain command injection vulnerability")
	}

	// If allowed commands are specified, check against whitelist
	if len(v.securityRules.AllowedCommands) > 0 {
		if !isCommandAllowed(script, v.securityRules.AllowedCommands) {
			return fmt.Errorf("script uses commands not in allowed list")
		}
	}

	return nil
}

// validateFileOperationSecurity validates file operation security
func (v *RecipeValidator) validateFileOperationSecurity(step *models.RecipeStep) error {
	operation, ok := step.Config["operation"].(string)
	if !ok {
		return fmt.Errorf("file operation missing operation type")
	}

	// Check for dangerous operations
	if operation == "delete" {
		path, _ := step.Config["path"].(string)
		if isDangerousPath(path) {
			return fmt.Errorf("dangerous file path: %s", path)
		}
	}

	// Check file system write permission
	if !v.securityRules.AllowFileSystemWrite {
		if operation != "read" && operation != "list" {
			return fmt.Errorf("file system write operations not allowed")
		}
	}

	return nil
}

// validateCompositeSecurity validates composite step security
func (v *RecipeValidator) validateCompositeSecurity(step *models.RecipeStep) error {
	recipes, ok := step.Config["recipes"].([]interface{})
	if !ok {
		return fmt.Errorf("composite step missing recipes configuration")
	}

	// Each sub-recipe should be validated separately
	// For now, just check count
	if len(recipes) > 10 {
		return fmt.Errorf("composite step has too many sub-recipes: %d", len(recipes))
	}

	return nil
}

// ValidateSchema validates recipe against schema
func (v *RecipeValidator) ValidateSchema(recipe *models.Recipe) error {
	if v.schemaValidator == nil {
		return nil
	}
	return v.schemaValidator.Validate(recipe)
}

// GetSchemaVersion returns the current schema version
func (v *RecipeValidator) GetSchemaVersion() string {
	if v.schemaValidator == nil {
		return "1.0.0"
	}
	return v.schemaValidator.GetVersion()
}

// Helper functions

func containsCommandInjection(script string) bool {
	// Check for common command injection patterns
	injectionPatterns := []string{
		"$(",
		"`",
		"${IFS}",
		"&&",
		"||",
		";",
		"|",
		">>",
		"2>&1",
	}

	// Allow some patterns in controlled contexts
	// This is a simplified check - a real implementation would use AST parsing
	for _, pattern := range injectionPatterns {
		if strings.Count(script, pattern) > 3 {
			// Too many occurrences might indicate injection
			return true
		}
	}

	return false
}

func isCommandAllowed(script string, allowedCommands []string) bool {
	// Extract commands from script (simplified)
	// A real implementation would parse the shell script properly
	words := strings.Fields(script)
	for _, word := range words {
		// Skip flags and arguments
		if strings.HasPrefix(word, "-") {
			continue
		}

		// Check if this looks like a command
		isAllowed := false
		for _, allowed := range allowedCommands {
			if word == allowed || strings.HasPrefix(word, allowed+"/") {
				isAllowed = true
				break
			}
		}

		if !isAllowed && looksLikeCommand(word) {
			return false
		}
	}

	return true
}

func looksLikeCommand(word string) bool {
	// Simple heuristic to identify commands
	// Skip variables, paths, etc.
	if strings.Contains(word, "=") || strings.HasPrefix(word, "$") || strings.HasPrefix(word, "/") {
		return false
	}

	// Common commands that might not be in allowed list
	commonCommands := []string{
		"ls", "cd", "pwd", "echo", "cat", "grep", "sed", "awk",
		"find", "sort", "uniq", "head", "tail", "cp", "mv", "rm",
		"mkdir", "rmdir", "touch", "chmod", "chown", "tar", "zip",
		"git", "docker", "kubectl", "npm", "yarn", "pip", "gem",
	}

	for _, cmd := range commonCommands {
		if word == cmd {
			return true
		}
	}

	return false
}

func isDangerousPath(path string) bool {
	if path == "" {
		return false
	}

	dangerousPaths := []string{
		"/",
		"/etc",
		"/bin",
		"/sbin",
		"/usr",
		"/var",
		"/boot",
		"/dev",
		"/proc",
		"/sys",
		"/root",
		"/home",
		"~",
		"..",
		"*",
	}

	for _, dangerous := range dangerousPaths {
		if path == dangerous || strings.HasPrefix(path, dangerous+"/") {
			return true
		}
	}

	return false
}

// GetDefaultSecurityRules returns default security rules
func GetDefaultSecurityRules() *storage.SecurityRuleSet {
	return &storage.SecurityRuleSet{
		AllowedCommands: []string{
			"echo", "printf", "cat", "grep", "sed", "awk",
			"find", "ls", "pwd", "cd", "mkdir", "cp", "mv",
			"git", "npm", "yarn", "mvn", "gradle", "go",
			"java", "javac", "python", "pip", "node",
		},
		ForbiddenCommands: []string{
			"rm -rf /",
			"sudo",
			"su",
			"passwd",
			"shutdown",
			"reboot",
			"systemctl",
			"service",
			"kill",
			"pkill",
			"dd",
			"mkfs",
			"fdisk",
			"mount",
			"umount",
		},
		MaxExecutionTime:     30 * 60, // 30 minutes
		AllowNetworkAccess:   false,
		AllowFileSystemWrite: true,
		SandboxRequired:      true,
		MaxMemoryUsage:       2 * 1024 * 1024 * 1024, // 2GB
		MaxCPUUsage:          2.0,
	}
}