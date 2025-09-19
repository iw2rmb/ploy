package python

import (
	analysis "github.com/iw2rmb/ploy/api/analysis"
	"github.com/sirupsen/logrus"
)

// PylintAnalyzer implements the LanguageAnalyzer interface for Python using Pylint.
type PylintAnalyzer struct {
	config *PylintConfig
	logger *logrus.Logger
}

// PylintConfig contains configuration for Pylint analyzer.
type PylintConfig struct {
	Enabled         bool              `json:"enabled" yaml:"enabled"`
	PylintPath      string            `json:"pylint_path" yaml:"pylint_path"`
	RCFile          string            `json:"rcfile" yaml:"rcfile"`
	DisableRules    []string          `json:"disable_rules" yaml:"disable_rules"`
	EnableRules     []string          `json:"enable_rules" yaml:"enable_rules"`
	MinScore        float64           `json:"min_score" yaml:"min_score"`
	MaxLineLength   int               `json:"max_line_length" yaml:"max_line_length"`
	Jobs            int               `json:"jobs" yaml:"jobs"`
	OutputFormat    string            `json:"output_format" yaml:"output_format"`
	SeverityMapping map[string]string `json:"severity_mapping" yaml:"severity_mapping"`
}

// PylintMessage represents a single message from Pylint JSON output.
type PylintMessage struct {
	Type      string `json:"type"`
	Module    string `json:"module"`
	Object    string `json:"obj"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Path      string `json:"path"`
	Symbol    string `json:"symbol"`
	Message   string `json:"message"`
	MessageID string `json:"message-id"`
}

// DefaultPylintConfig returns the default Pylint configuration.
func DefaultPylintConfig() *PylintConfig {
	return &PylintConfig{
		Enabled:       true,
		PylintPath:    "pylint",
		OutputFormat:  "json",
		MinScore:      7.0,
		MaxLineLength: 120,
		Jobs:          4,
		DisableRules: []string{
			"C0111",
			"R0903",
			"W0511",
		},
		SeverityMapping: map[string]string{
			"fatal":      "critical",
			"error":      "high",
			"warning":    "medium",
			"convention": "low",
			"refactor":   "low",
			"info":       "info",
		},
	}
}

// NewPylintAnalyzer creates a new Pylint analyzer.
func NewPylintAnalyzer(logger *logrus.Logger) *PylintAnalyzer {
	return &PylintAnalyzer{
		config: DefaultPylintConfig(),
		logger: logger,
	}
}

// GetSupportedFileTypes returns supported file extensions.
func (a *PylintAnalyzer) GetSupportedFileTypes() []string {
	return []string{".py", ".pyw"}
}

// GetAnalyzerInfo returns analyzer information.
func (a *PylintAnalyzer) GetAnalyzerInfo() analysis.AnalyzerInfo {
	return analysis.AnalyzerInfo{
		Name:        "pylint",
		Version:     "3.0.0",
		Language:    "python",
		Description: "Pylint static analysis for Python code quality and error detection",
		Capabilities: []string{
			"syntax-checking",
			"error-detection",
			"code-standards",
			"refactoring-help",
			"duplicate-detection",
			"unused-detection",
			"complexity-analysis",
			"convention-checking",
			"security-scanning",
		},
	}
}
