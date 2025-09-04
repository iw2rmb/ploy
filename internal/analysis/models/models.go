package models

import "time"

type Repository struct {
    Name string `json:"name"`
    URL  string `json:"url"`
}

type AnalysisConfig struct {
    Enabled        bool                   `json:"enabled"`
    FailOnCritical bool                   `json:"fail_on_critical"`
    ARFIntegration bool                   `json:"arf_integration"`
    Timeout        time.Duration          `json:"timeout"`
    Languages      map[string]interface{} `json:"languages,omitempty"`
}

type AnalysisRequest struct {
    Repository Repository    `json:"repository"`
    Config     AnalysisConfig `json:"config"`
    FixIssues  bool           `json:"fix_issues"`
    DryRun     bool           `json:"dry_run"`
}

type Issue struct {
    Severity string `json:"severity"`
    Category string `json:"category"`
    File     string `json:"file"`
    Line     int    `json:"line"`
    Message  string `json:"message"`
}

type Metrics struct {
    AnalysisTime     time.Duration    `json:"analysis_time"`
    TotalFiles       int              `json:"total_files"`
    AnalyzedFiles    int              `json:"analyzed_files"`
    TotalIssues      int              `json:"total_issues"`
    IssuesBySeverity map[string]int   `json:"issues_by_severity"`
}

type LanguageResult struct {
    Analyzer string  `json:"analyzer"`
    Issues   []Issue `json:"issues"`
    Success  bool    `json:"success"`
    Error    string  `json:"error,omitempty"`
}

type AnalysisResult struct {
    ID              string                      `json:"id"`
    Repository      Repository                  `json:"repository"`
    Timestamp       time.Time                   `json:"timestamp"`
    OverallScore    float64                     `json:"overall_score"`
    Metrics         Metrics                     `json:"metrics"`
    Issues          []Issue                     `json:"issues"`
    LanguageResults map[string]LanguageResult   `json:"language_results"`
    Success         bool                        `json:"success"`
    Error           string                      `json:"error,omitempty"`
    ARFTriggers     []string                    `json:"arf_triggers,omitempty"`
}
