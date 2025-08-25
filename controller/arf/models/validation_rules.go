package models

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidationRules define codebase compatibility checks
type ValidationRules struct {
	RequiredFiles     []string          `json:"required_files,omitempty" yaml:"required_files,omitempty"`
	ForbiddenFiles    []string          `json:"forbidden_files,omitempty" yaml:"forbidden_files,omitempty"`
	FilePatterns      []string          `json:"file_patterns,omitempty" yaml:"file_patterns,omitempty"`
	MinFileCount      int               `json:"min_file_count,omitempty" yaml:"min_file_count,omitempty"`
	MaxRepoSize       int64             `json:"max_repo_size,omitempty" yaml:"max_repo_size,omitempty"`
	LanguageDetection LanguageDetection `json:"language_detection,omitempty" yaml:"language_detection,omitempty"`
	CustomRules       []CustomRule      `json:"custom_rules,omitempty" yaml:"custom_rules,omitempty"`
}

// LanguageDetection defines language-specific validation
type LanguageDetection struct {
	Primary       string   `json:"primary,omitempty" yaml:"primary,omitempty"`
	Secondary     []string `json:"secondary,omitempty" yaml:"secondary,omitempty"`
	MinConfidence float64  `json:"min_confidence,omitempty" yaml:"min_confidence,omitempty"`
	Required      bool     `json:"required,omitempty" yaml:"required,omitempty"`
}

// CustomRule defines a custom validation rule
type CustomRule struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Type        string `json:"type" yaml:"type"`
	Value       string `json:"value" yaml:"value"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// Validate validates the validation rules
func (v *ValidationRules) Validate() error {
	// Validate required files
	for _, file := range v.RequiredFiles {
		if file == "" {
			return fmt.Errorf("required file path cannot be empty")
		}
		if !isValidFilePath(file) {
			return fmt.Errorf("invalid required file path: %s", file)
		}
	}

	// Validate forbidden files
	for _, file := range v.ForbiddenFiles {
		if file == "" {
			return fmt.Errorf("forbidden file path cannot be empty")
		}
		if !isValidFilePath(file) {
			return fmt.Errorf("invalid forbidden file path: %s", file)
		}
	}

	// Validate file patterns
	for _, pattern := range v.FilePatterns {
		if pattern == "" {
			return fmt.Errorf("file pattern cannot be empty")
		}
		// Test if pattern is valid glob
		if _, err := filepath.Match(pattern, "test.txt"); err != nil {
			return fmt.Errorf("invalid file pattern: %s", pattern)
		}
	}

	// Validate file count
	if v.MinFileCount < 0 {
		return fmt.Errorf("min_file_count cannot be negative")
	}

	// Validate repo size
	if v.MaxRepoSize < 0 {
		return fmt.Errorf("max_repo_size cannot be negative")
	}

	// Validate language detection
	if err := v.LanguageDetection.Validate(); err != nil {
		return fmt.Errorf("language detection validation failed: %w", err)
	}

	// Validate custom rules
	for i, rule := range v.CustomRules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("custom rule %d validation failed: %w", i+1, err)
		}
	}

	return nil
}

// Validate validates language detection settings
func (l *LanguageDetection) Validate() error {
	// Validate primary language if specified
	if l.Primary != "" && !isValidLanguage(l.Primary) {
		return fmt.Errorf("invalid primary language: %s", l.Primary)
	}

	// Validate secondary languages
	for _, lang := range l.Secondary {
		if !isValidLanguage(lang) {
			return fmt.Errorf("invalid secondary language: %s", lang)
		}
	}

	// Validate confidence
	if l.MinConfidence < 0 || l.MinConfidence > 1 {
		return fmt.Errorf("min_confidence must be between 0 and 1")
	}

	return nil
}

// Validate validates a custom rule
func (r *CustomRule) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("custom rule name is required")
	}

	if r.Type == "" {
		return fmt.Errorf("custom rule type is required")
	}

	// Validate rule type
	validTypes := []string{
		"file_exists",
		"file_content",
		"directory_exists",
		"command_output",
		"env_var",
		"regex_match",
		"json_path",
		"xml_path",
	}

	valid := false
	for _, t := range validTypes {
		if r.Type == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid custom rule type: %s", r.Type)
	}

	if r.Value == "" {
		return fmt.Errorf("custom rule value is required")
	}

	return nil
}

// CheckCompatibility checks if a codebase meets the validation rules
type CodebaseInfo struct {
	Files       []string
	TotalSize   int64
	Languages   map[string]float64
	Directories []string
}

// CheckCompatibility validates a codebase against the rules
func (v *ValidationRules) CheckCompatibility(info *CodebaseInfo) error {
	// Check required files
	for _, required := range v.RequiredFiles {
		found := false
		for _, file := range info.Files {
			if matchesPath(file, required) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("required file not found: %s", required)
		}
	}

	// Check forbidden files
	for _, forbidden := range v.ForbiddenFiles {
		for _, file := range info.Files {
			if matchesPath(file, forbidden) {
				return fmt.Errorf("forbidden file found: %s", forbidden)
			}
		}
	}

	// Check file patterns
	if len(v.FilePatterns) > 0 {
		matchFound := false
		for _, pattern := range v.FilePatterns {
			for _, file := range info.Files {
				if matched, _ := filepath.Match(pattern, filepath.Base(file)); matched {
					matchFound = true
					break
				}
			}
			if matchFound {
				break
			}
		}
		if !matchFound {
			return fmt.Errorf("no files matching required patterns found")
		}
	}

	// Check minimum file count
	if v.MinFileCount > 0 && len(info.Files) < v.MinFileCount {
		return fmt.Errorf("insufficient files: found %d, required %d", len(info.Files), v.MinFileCount)
	}

	// Check maximum repo size
	if v.MaxRepoSize > 0 && info.TotalSize > v.MaxRepoSize {
		return fmt.Errorf("repository too large: %d bytes exceeds limit of %d bytes", info.TotalSize, v.MaxRepoSize)
	}

	// Check language requirements
	if v.LanguageDetection.Required {
		if err := v.checkLanguageRequirements(info.Languages); err != nil {
			return err
		}
	}

	return nil
}

func (v *ValidationRules) checkLanguageRequirements(languages map[string]float64) error {
	ld := v.LanguageDetection

	// Check primary language
	if ld.Primary != "" {
		confidence, exists := languages[ld.Primary]
		if !exists {
			return fmt.Errorf("required primary language not detected: %s", ld.Primary)
		}
		if confidence < ld.MinConfidence {
			return fmt.Errorf("primary language confidence too low: %s (%.2f < %.2f)",
				ld.Primary, confidence, ld.MinConfidence)
		}
	}

	// Check secondary languages
	for _, lang := range ld.Secondary {
		if _, exists := languages[lang]; !exists {
			return fmt.Errorf("required secondary language not detected: %s", lang)
		}
	}

	return nil
}

// Helper functions

func isValidFilePath(path string) bool {
	// Basic validation - no absolute paths, no parent directory references
	if filepath.IsAbs(path) {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	return true
}

func matchesPath(filePath, pattern string) bool {
	// Check exact match
	if filePath == pattern {
		return true
	}

	// Check if pattern is a directory (ends with /)
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(filePath, pattern)
	}

	// Check glob pattern match
	if matched, _ := filepath.Match(pattern, filePath); matched {
		return true
	}

	// Check if filename matches
	if filepath.Base(filePath) == pattern {
		return true
	}

	return false
}

// SetDefaults sets default validation rules
func (v *ValidationRules) SetDefaults() {
	// Set default max repo size to 1GB
	if v.MaxRepoSize == 0 {
		v.MaxRepoSize = 1024 * 1024 * 1024 // 1GB
	}

	// Set default language detection confidence
	if v.LanguageDetection.MinConfidence == 0 {
		v.LanguageDetection.MinConfidence = 0.7
	}
}