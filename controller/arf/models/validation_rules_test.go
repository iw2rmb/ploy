package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationRules_Validate(t *testing.T) {
	tests := []struct {
		name          string
		rules         ValidationRules
		expectedError string
	}{
		// Valid validation rules
		{
			name: "valid complete rules",
			rules: ValidationRules{
				RequiredFiles:  []string{"package.json", "src/main.js"},
				ForbiddenFiles: []string{"node_modules/", ".env"},
				FilePatterns:   []string{"*.js", "*.ts", "*.json"},
				MinFileCount:   5,
				MaxRepoSize:    1024 * 1024, // 1MB
				LanguageDetection: LanguageDetection{
					Primary:       "javascript",
					Secondary:     []string{"typescript"},
					MinConfidence: 0.8,
					Required:      true,
				},
				CustomRules: []CustomRule{
					{
						Name:        "has_dockerfile",
						Description: "Project must have Dockerfile",
						Type:        "file_exists",
						Value:       "Dockerfile",
						Required:    true,
					},
				},
			},
		},
		{
			name: "minimal valid rules",
			rules: ValidationRules{
				RequiredFiles: []string{"README.md"},
			},
		},
		{
			name:  "empty rules (valid)",
			rules: ValidationRules{},
		},

		// Invalid required files
		{
			name: "empty required file path",
			rules: ValidationRules{
				RequiredFiles: []string{"valid.txt", ""},
			},
			expectedError: "required file path cannot be empty",
		},
		{
			name: "absolute path in required files",
			rules: ValidationRules{
				RequiredFiles: []string{"/etc/passwd"},
			},
			expectedError: "invalid required file path: /etc/passwd",
		},
		{
			name: "parent directory reference in required files",
			rules: ValidationRules{
				RequiredFiles: []string{"../config.json"},
			},
			expectedError: "invalid required file path: ../config.json",
		},

		// Invalid forbidden files
		{
			name: "empty forbidden file path",
			rules: ValidationRules{
				ForbiddenFiles: []string{"", "node_modules/"},
			},
			expectedError: "forbidden file path cannot be empty",
		},
		{
			name: "absolute path in forbidden files",
			rules: ValidationRules{
				ForbiddenFiles: []string{"/tmp/secret"},
			},
			expectedError: "invalid forbidden file path: /tmp/secret",
		},
		{
			name: "parent directory reference in forbidden files",
			rules: ValidationRules{
				ForbiddenFiles: []string{"../../secrets"},
			},
			expectedError: "invalid forbidden file path: ../../secrets",
		},

		// Invalid file patterns
		{
			name: "empty file pattern",
			rules: ValidationRules{
				FilePatterns: []string{"*.js", ""},
			},
			expectedError: "file pattern cannot be empty",
		},
		{
			name: "invalid glob pattern",
			rules: ValidationRules{
				FilePatterns: []string{"["},
			},
			expectedError: "invalid file pattern: [",
		},

		// Invalid counts and sizes
		{
			name: "negative min file count",
			rules: ValidationRules{
				MinFileCount: -1,
			},
			expectedError: "min_file_count cannot be negative",
		},
		{
			name: "negative max repo size",
			rules: ValidationRules{
				MaxRepoSize: -1,
			},
			expectedError: "max_repo_size cannot be negative",
		},

		// Invalid language detection
		{
			name: "invalid primary language",
			rules: ValidationRules{
				LanguageDetection: LanguageDetection{
					Primary: "invalid-language",
				},
			},
			expectedError: "language detection validation failed: invalid primary language: invalid-language",
		},
		{
			name: "invalid secondary language",
			rules: ValidationRules{
				LanguageDetection: LanguageDetection{
					Secondary: []string{"javascript", "invalid-language"},
				},
			},
			expectedError: "language detection validation failed: invalid secondary language: invalid-language",
		},
		{
			name: "invalid confidence below range",
			rules: ValidationRules{
				LanguageDetection: LanguageDetection{
					MinConfidence: -0.1,
				},
			},
			expectedError: "language detection validation failed: min_confidence must be between 0 and 1",
		},
		{
			name: "invalid confidence above range",
			rules: ValidationRules{
				LanguageDetection: LanguageDetection{
					MinConfidence: 1.1,
				},
			},
			expectedError: "language detection validation failed: min_confidence must be between 0 and 1",
		},

		// Invalid custom rules
		{
			name: "custom rule missing name",
			rules: ValidationRules{
				CustomRules: []CustomRule{
					{
						Type:  "file_exists",
						Value: "Dockerfile",
					},
				},
			},
			expectedError: "custom rule 1 validation failed: custom rule name is required",
		},
		{
			name: "custom rule missing type",
			rules: ValidationRules{
				CustomRules: []CustomRule{
					{
						Name:  "has_dockerfile",
						Value: "Dockerfile",
					},
				},
			},
			expectedError: "custom rule 1 validation failed: custom rule type is required",
		},
		{
			name: "custom rule invalid type",
			rules: ValidationRules{
				CustomRules: []CustomRule{
					{
						Name:  "has_dockerfile",
						Type:  "invalid_type",
						Value: "Dockerfile",
					},
				},
			},
			expectedError: "custom rule 1 validation failed: invalid custom rule type: invalid_type",
		},
		{
			name: "custom rule missing value",
			rules: ValidationRules{
				CustomRules: []CustomRule{
					{
						Name: "has_dockerfile",
						Type: "file_exists",
					},
				},
			},
			expectedError: "custom rule 1 validation failed: custom rule value is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rules.Validate()
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestLanguageDetection_Validate(t *testing.T) {
	tests := []struct {
		name          string
		detection     LanguageDetection
		expectedError string
	}{
		// Valid language detection settings
		{
			name: "valid complete detection",
			detection: LanguageDetection{
				Primary:       "javascript",
				Secondary:     []string{"typescript", "python"},
				MinConfidence: 0.8,
				Required:      true,
			},
		},
		{
			name: "valid minimal detection",
			detection: LanguageDetection{
				Primary: "go",
			},
		},
		{
			name: "valid with zero confidence",
			detection: LanguageDetection{
				Primary:       "python",
				MinConfidence: 0.0,
			},
		},
		{
			name: "valid with max confidence",
			detection: LanguageDetection{
				Primary:       "rust",
				MinConfidence: 1.0,
			},
		},
		{
			name:      "empty detection (valid)",
			detection: LanguageDetection{},
		},

		// Invalid language detection
		{
			name: "invalid primary language",
			detection: LanguageDetection{
				Primary: "nonexistent-language",
			},
			expectedError: "invalid primary language: nonexistent-language",
		},
		{
			name: "invalid secondary language",
			detection: LanguageDetection{
				Secondary: []string{"javascript", "invalid-lang"},
			},
			expectedError: "invalid secondary language: invalid-lang",
		},
		{
			name: "confidence too low",
			detection: LanguageDetection{
				MinConfidence: -0.5,
			},
			expectedError: "min_confidence must be between 0 and 1",
		},
		{
			name: "confidence too high",
			detection: LanguageDetection{
				MinConfidence: 1.5,
			},
			expectedError: "min_confidence must be between 0 and 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.detection.Validate()
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestCustomRule_Validate(t *testing.T) {
	tests := []struct {
		name          string
		rule          CustomRule
		expectedError string
	}{
		// Valid custom rules for each type
		{
			name: "valid file_exists rule",
			rule: CustomRule{
				Name:        "has_dockerfile",
				Description: "Must have Dockerfile",
				Type:        "file_exists",
				Value:       "Dockerfile",
				Required:    true,
			},
		},
		{
			name: "valid file_content rule",
			rule: CustomRule{
				Name:  "package_json_has_scripts",
				Type:  "file_content",
				Value: `{"path": "package.json", "contains": "scripts"}`,
			},
		},
		{
			name: "valid directory_exists rule",
			rule: CustomRule{
				Name:  "has_src_directory",
				Type:  "directory_exists",
				Value: "src/",
			},
		},
		{
			name: "valid command_output rule",
			rule: CustomRule{
				Name:  "go_version_check",
				Type:  "command_output",
				Value: `{"command": "go version", "contains": "go1"}`,
			},
		},
		{
			name: "valid env_var rule",
			rule: CustomRule{
				Name:  "has_go_path",
				Type:  "env_var",
				Value: "GOPATH",
			},
		},
		{
			name: "valid regex_match rule",
			rule: CustomRule{
				Name:  "version_format",
				Type:  "regex_match",
				Value: `^v\d+\.\d+\.\d+$`,
			},
		},
		{
			name: "valid json_path rule",
			rule: CustomRule{
				Name:  "package_json_name",
				Type:  "json_path",
				Value: `{"file": "package.json", "path": "$.name"}`,
			},
		},
		{
			name: "valid xml_path rule",
			rule: CustomRule{
				Name:  "pom_artifact_id",
				Type:  "xml_path",
				Value: `{"file": "pom.xml", "path": "/project/artifactId"}`,
			},
		},

		// Invalid custom rules
		{
			name: "missing name",
			rule: CustomRule{
				Type:  "file_exists",
				Value: "Dockerfile",
			},
			expectedError: "custom rule name is required",
		},
		{
			name: "missing type",
			rule: CustomRule{
				Name:  "test_rule",
				Value: "test_value",
			},
			expectedError: "custom rule type is required",
		},
		{
			name: "invalid type",
			rule: CustomRule{
				Name:  "test_rule",
				Type:  "invalid_type",
				Value: "test_value",
			},
			expectedError: "invalid custom rule type: invalid_type",
		},
		{
			name: "missing value",
			rule: CustomRule{
				Name: "test_rule",
				Type: "file_exists",
			},
			expectedError: "custom rule value is required",
		},
		{
			name: "empty name",
			rule: CustomRule{
				Name:  "",
				Type:  "file_exists",
				Value: "Dockerfile",
			},
			expectedError: "custom rule name is required",
		},
		{
			name: "empty type",
			rule: CustomRule{
				Name:  "test_rule",
				Type:  "",
				Value: "test_value",
			},
			expectedError: "custom rule type is required",
		},
		{
			name: "empty value",
			rule: CustomRule{
				Name:  "test_rule",
				Type:  "file_exists",
				Value: "",
			},
			expectedError: "custom rule value is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestValidationRules_CheckCompatibility(t *testing.T) {
	tests := []struct {
		name          string
		rules         ValidationRules
		codebaseInfo  CodebaseInfo
		expectedError string
	}{
		// Compatible codebases
		{
			name: "meets all requirements",
			rules: ValidationRules{
				RequiredFiles:  []string{"package.json", "src/index.js"},
				ForbiddenFiles: []string{".env"},
				FilePatterns:   []string{"*.js"},
				MinFileCount:   3,
				MaxRepoSize:    1000,
				LanguageDetection: LanguageDetection{
					Primary:       "javascript",
					MinConfidence: 0.7,
					Required:      true,
				},
			},
			codebaseInfo: CodebaseInfo{
				Files:     []string{"package.json", "src/index.js", "README.md"},
				TotalSize: 500,
				Languages: map[string]float64{"javascript": 0.8},
			},
		},
		{
			name: "minimal requirements met",
			rules: ValidationRules{
				RequiredFiles: []string{"README.md"},
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"README.md", "main.go"},
			},
		},
		{
			name:  "no requirements - always passes",
			rules: ValidationRules{},
			codebaseInfo: CodebaseInfo{
				Files: []string{"anything.txt"},
			},
		},

		// Missing required files
		{
			name: "missing required file",
			rules: ValidationRules{
				RequiredFiles: []string{"package.json"},
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"index.js", "README.md"},
			},
			expectedError: "required file not found: package.json",
		},
		{
			name: "missing multiple required files",
			rules: ValidationRules{
				RequiredFiles: []string{"package.json", "Dockerfile"},
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"index.js"},
			},
			expectedError: "required file not found: package.json",
		},

		// Forbidden files present
		{
			name: "forbidden file present",
			rules: ValidationRules{
				ForbiddenFiles: []string{".env"},
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"index.js", ".env", "README.md"},
			},
			expectedError: "forbidden file found: .env",
		},
		{
			name: "forbidden directory present",
			rules: ValidationRules{
				ForbiddenFiles: []string{"node_modules/"},
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"index.js", "node_modules/express/package.json"},
			},
			expectedError: "forbidden file found: node_modules/",
		},

		// File pattern requirements
		{
			name: "no matching file patterns",
			rules: ValidationRules{
				FilePatterns: []string{"*.py"},
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"main.js", "package.json"},
			},
			expectedError: "no files matching required patterns found",
		},
		{
			name: "file pattern matches basename",
			rules: ValidationRules{
				FilePatterns: []string{"*.js"},
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"src/components/App.js"},
			},
		},

		// File count requirements
		{
			name: "insufficient file count",
			rules: ValidationRules{
				MinFileCount: 5,
			},
			codebaseInfo: CodebaseInfo{
				Files: []string{"main.go", "README.md"},
			},
			expectedError: "insufficient files: found 2, required 5",
		},

		// Repository size limits
		{
			name: "repository too large",
			rules: ValidationRules{
				MaxRepoSize: 1000,
			},
			codebaseInfo: CodebaseInfo{
				Files:     []string{"large_file.bin"},
				TotalSize: 2000,
			},
			expectedError: "repository too large: 2000 bytes exceeds limit of 1000 bytes",
		},

		// Language detection requirements
		{
			name: "missing required primary language",
			rules: ValidationRules{
				LanguageDetection: LanguageDetection{
					Primary:  "python",
					Required: true,
				},
			},
			codebaseInfo: CodebaseInfo{
				Languages: map[string]float64{"javascript": 0.8},
			},
			expectedError: "required primary language not detected: python",
		},
		{
			name: "primary language confidence too low",
			rules: ValidationRules{
				LanguageDetection: LanguageDetection{
					Primary:       "python",
					MinConfidence: 0.8,
					Required:      true,
				},
			},
			codebaseInfo: CodebaseInfo{
				Languages: map[string]float64{"python": 0.6},
			},
			expectedError: "primary language confidence too low: python (0.60 < 0.80)",
		},
		{
			name: "missing required secondary language",
			rules: ValidationRules{
				LanguageDetection: LanguageDetection{
					Secondary: []string{"typescript"},
					Required:  true,
				},
			},
			codebaseInfo: CodebaseInfo{
				Languages: map[string]float64{"javascript": 0.8},
			},
			expectedError: "required secondary language not detected: typescript",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rules.CheckCompatibility(&tt.codebaseInfo)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestValidationRules_SetDefaults(t *testing.T) {
	t.Run("sets default max repo size", func(t *testing.T) {
		rules := ValidationRules{}
		rules.SetDefaults()

		assert.Equal(t, int64(1024*1024*1024), rules.MaxRepoSize) // 1GB
	})

	t.Run("sets default language detection confidence", func(t *testing.T) {
		rules := ValidationRules{}
		rules.SetDefaults()

		assert.Equal(t, 0.7, rules.LanguageDetection.MinConfidence)
	})

	t.Run("does not override existing values", func(t *testing.T) {
		rules := ValidationRules{
			MaxRepoSize: 500 * 1024 * 1024, // 500MB
			LanguageDetection: LanguageDetection{
				MinConfidence: 0.9,
			},
		}

		rules.SetDefaults()

		assert.Equal(t, int64(500*1024*1024), rules.MaxRepoSize)
		assert.Equal(t, 0.9, rules.LanguageDetection.MinConfidence)
	})

	t.Run("sets defaults for empty fields only", func(t *testing.T) {
		rules := ValidationRules{
			MaxRepoSize: 100 * 1024 * 1024, // 100MB - should not change
			// MinConfidence not set - should get default
		}

		rules.SetDefaults()

		assert.Equal(t, int64(100*1024*1024), rules.MaxRepoSize)
		assert.Equal(t, 0.7, rules.LanguageDetection.MinConfidence)
	})
}

// Helper function tests
func TestIsValidFilePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Valid paths
		{
			name:     "simple filename",
			path:     "main.go",
			expected: true,
		},
		{
			name:     "relative path",
			path:     "src/main.go",
			expected: true,
		},
		{
			name:     "nested relative path",
			path:     "src/components/App.js",
			expected: true,
		},
		{
			name:     "path with dots",
			path:     "config/.env.example",
			expected: true,
		},

		// Invalid paths
		{
			name:     "absolute path",
			path:     "/etc/passwd",
			expected: false,
		},
		{
			name:     "parent directory reference",
			path:     "../config.json",
			expected: false,
		},
		{
			name:     "nested parent directory reference",
			path:     "../../secrets/key.pem",
			expected: false,
		},
		{
			name:     "parent directory in middle",
			path:     "src/../config/app.json",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidFilePath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesPath(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		pattern  string
		expected bool
	}{
		// Exact matches
		{
			name:     "exact match",
			filePath: "package.json",
			pattern:  "package.json",
			expected: true,
		},
		{
			name:     "exact path match",
			filePath: "src/main.js",
			pattern:  "src/main.js",
			expected: true,
		},

		// Directory patterns
		{
			name:     "directory pattern match",
			filePath: "node_modules/express/package.json",
			pattern:  "node_modules/",
			expected: true,
		},
		{
			name:     "directory pattern no match",
			filePath: "src/main.js",
			pattern:  "node_modules/",
			expected: false,
		},

		// Glob patterns
		{
			name:     "glob pattern match",
			filePath: "main.js",
			pattern:  "*.js",
			expected: true,
		},
		{
			name:     "glob pattern no match",
			filePath: "main.py",
			pattern:  "*.js",
			expected: false,
		},

		// Basename matches
		{
			name:     "basename match",
			filePath: "src/components/package.json",
			pattern:  "package.json",
			expected: true,
		},
		{
			name:     "basename no match",
			filePath: "src/main.js",
			pattern:  "package.json",
			expected: false,
		},

		// No matches
		{
			name:     "no match",
			filePath: "README.md",
			pattern:  "Dockerfile",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPath(tt.filePath, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmarks
func BenchmarkValidationRules_Validate(b *testing.B) {
	rules := ValidationRules{
		RequiredFiles:  []string{"package.json", "src/main.js"},
		ForbiddenFiles: []string{".env", "node_modules/"},
		FilePatterns:   []string{"*.js", "*.ts"},
		MinFileCount:   5,
		MaxRepoSize:    1024 * 1024,
		LanguageDetection: LanguageDetection{
			Primary:       "javascript",
			Secondary:     []string{"typescript"},
			MinConfidence: 0.8,
		},
		CustomRules: []CustomRule{
			{
				Name:  "has_dockerfile",
				Type:  "file_exists",
				Value: "Dockerfile",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rules.Validate()
	}
}

func BenchmarkValidationRules_CheckCompatibility(b *testing.B) {
	rules := ValidationRules{
		RequiredFiles:  []string{"package.json"},
		ForbiddenFiles: []string{".env"},
		FilePatterns:   []string{"*.js"},
		MinFileCount:   3,
		MaxRepoSize:    1000,
	}

	info := CodebaseInfo{
		Files:     []string{"package.json", "src/index.js", "README.md"},
		TotalSize: 500,
		Languages: map[string]float64{"javascript": 0.8},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rules.CheckCompatibility(&info)
	}
}

// Property-based testing
func TestValidationRules_Properties(t *testing.T) {
	t.Run("empty rules always validate", func(t *testing.T) {
		rules := ValidationRules{}
		err := rules.Validate()
		assert.NoError(t, err)
	})

	t.Run("empty codebase info with empty rules passes compatibility", func(t *testing.T) {
		rules := ValidationRules{}
		info := CodebaseInfo{}
		err := rules.CheckCompatibility(&info)
		assert.NoError(t, err)
	})

	t.Run("set defaults is idempotent", func(t *testing.T) {
		rules := ValidationRules{}
		
		rules.SetDefaults()
		maxRepoSize1 := rules.MaxRepoSize
		minConfidence1 := rules.LanguageDetection.MinConfidence
		
		rules.SetDefaults()
		maxRepoSize2 := rules.MaxRepoSize
		minConfidence2 := rules.LanguageDetection.MinConfidence
		
		assert.Equal(t, maxRepoSize1, maxRepoSize2)
		assert.Equal(t, minConfidence1, minConfidence2)
	})
}

// Test custom rule type validation completeness
func TestCustomRule_AllValidTypes(t *testing.T) {
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

	for _, ruleType := range validTypes {
		t.Run("valid_type_"+ruleType, func(t *testing.T) {
			rule := CustomRule{
				Name:  "test_rule",
				Type:  ruleType,
				Value: "test_value",
			}
			
			err := rule.Validate()
			assert.NoError(t, err, "type %s should be valid", ruleType)
		})
	}
}

func TestLanguageDetection_EdgeCases(t *testing.T) {
	t.Run("boundary confidence values", func(t *testing.T) {
		testCases := []struct {
			confidence    float64
			shouldBeValid bool
		}{
			{-0.0001, false}, // Just below 0
			{0.0, true},      // Exactly 0
			{0.5, true},      // Middle
			{1.0, true},      // Exactly 1
			{1.0001, false},  // Just above 1
		}

		for _, tc := range testCases {
			detection := LanguageDetection{
				MinConfidence: tc.confidence,
			}
			
			err := detection.Validate()
			if tc.shouldBeValid {
				assert.NoError(t, err, "confidence %f should be valid", tc.confidence)
			} else {
				assert.Error(t, err, "confidence %f should be invalid", tc.confidence)
			}
		}
	})

	t.Run("case sensitivity for languages", func(t *testing.T) {
		testCases := []struct {
			language      string
			shouldBeValid bool
		}{
			{"JavaScript", true},  // Mixed case
			{"PYTHON", true},      // Uppercase
			{"go", true},          // Lowercase
			{"Go", true},          // Capitalized
			{"typescript", true},  // Lowercase
			{"TypeScript", true},  // Mixed case
		}

		for _, tc := range testCases {
			detection := LanguageDetection{
				Primary: tc.language,
			}
			
			err := detection.Validate()
			if tc.shouldBeValid {
				assert.NoError(t, err, "language %s should be valid", tc.language)
			} else {
				assert.Error(t, err, "language %s should be invalid", tc.language)
			}
		}
	})
}