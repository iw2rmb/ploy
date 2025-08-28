package analyzer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PylintAnalyzer provides Pylint-specific static analysis functionality
type PylintAnalyzer struct {
	workDir       string
	pylintPath    string
	timeout       time.Duration
	maxMemory     string
	sandboxUser   string
}

// AnalyzeRequest represents an analysis request
type AnalyzeRequest struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Archive   []byte    `json:"-"` // Binary data not included in JSON
}

// AnalyzeResponse represents an analysis response
type AnalyzeResponse struct {
	ID        string       `json:"id"`
	Status    string       `json:"status"`
	Timestamp string       `json:"timestamp"`
	Result    AnalysisResult `json:"result"`
	Error     string       `json:"error,omitempty"`
	Duration  string       `json:"duration"`
}

// AnalysisResult contains the analysis findings
type AnalysisResult struct {
	Issues  []Issue       `json:"issues"`
	Metrics AnalysisMetrics `json:"metrics"`
	Summary AnalysisSummary `json:"summary"`
}

// Issue represents a single analysis issue
type Issue struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Category string `json:"category,omitempty"`
}

// AnalysisMetrics contains quantitative analysis results
type AnalysisMetrics struct {
	TotalFiles    int            `json:"total_files"`
	AnalyzedFiles int            `json:"analyzed_files"`
	TotalLines    int            `json:"total_lines"`
	IssueCount    int            `json:"issue_count"`
	Severity      SeverityCounts `json:"severity"`
	Categories    CategoryCounts `json:"categories"`
	Score         float64        `json:"score,omitempty"`
}

// SeverityCounts tracks issues by severity level
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
}

// CategoryCounts tracks issues by category
type CategoryCounts struct {
	Error       int `json:"error"`
	Warning     int `json:"warning"`
	Convention  int `json:"convention"`
	Refactor    int `json:"refactor"`
	Fatal       int `json:"fatal"`
}

// AnalysisSummary provides high-level analysis summary
type AnalysisSummary struct {
	Status       string  `json:"status"`
	TotalIssues  int     `json:"total_issues"`
	HighSeverity int     `json:"high_severity_issues"`
	Score        float64 `json:"score,omitempty"`
	Recommendation string `json:"recommendation"`
}

// NewPylintAnalyzer creates a new Pylint analyzer instance
func NewPylintAnalyzer(workDir string, config PylintConfig) *PylintAnalyzer {
	return &PylintAnalyzer{
		workDir:     workDir,
		pylintPath:  config.Executable,
		timeout:     config.Timeout,
		maxMemory:   config.MaxMemory,
		sandboxUser: config.SandboxUser,
	}
}

// PylintConfig contains Pylint-specific configuration
type PylintConfig struct {
	Executable   string        `yaml:"executable"`
	Timeout      time.Duration `yaml:"timeout"`
	MaxMemory    string        `yaml:"max_memory"`
	SandboxUser  string        `yaml:"sandbox_user"`
	OutputFormat string        `yaml:"output_format"`
}

// AnalyzeArchive analyzes a gzipped tar archive of Python files
func (p *PylintAnalyzer) AnalyzeArchive(ctx context.Context, archiveData []byte) (*AnalyzeResponse, error) {
	startTime := time.Now()
	
	// Generate unique analysis ID
	analysisID := fmt.Sprintf("pylint-%d", time.Now().UnixNano())
	
	response := &AnalyzeResponse{
		ID:        analysisID,
		Status:    "processing",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	
	// Create temporary analysis directory
	analysisDir := filepath.Join(p.workDir, analysisID)
	if err := os.MkdirAll(analysisDir, 0755); err != nil {
		response.Status = "error"
		response.Error = fmt.Sprintf("failed to create analysis directory: %v", err)
		return response, err
	}
	defer os.RemoveAll(analysisDir) // Cleanup
	
	// Extract archive
	if err := p.extractArchive(archiveData, analysisDir); err != nil {
		response.Status = "error"
		response.Error = fmt.Sprintf("failed to extract archive: %v", err)
		return response, err
	}
	
	// Run Pylint analysis
	result, err := p.runPylintAnalysis(ctx, analysisDir)
	if err != nil {
		response.Status = "error"
		response.Error = fmt.Sprintf("analysis failed: %v", err)
		return response, err
	}
	
	// Populate successful response
	response.Status = "success"
	response.Result = *result
	response.Duration = time.Since(startTime).String()
	
	return response, nil
}

// extractArchive extracts gzipped tar archive to target directory
func (p *PylintAnalyzer) extractArchive(archiveData []byte, targetDir string) error {
	// Create gzip reader
	gzReader, err := gzip.NewReader(strings.NewReader(string(archiveData)))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()
	
	// Create tar reader
	tarReader := tar.NewReader(gzReader)
	
	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}
		
		// Skip directories
		if header.Typeflag != tar.TypeReg {
			continue
		}
		
		// Validate file is Python file
		if !strings.HasSuffix(header.Name, ".py") && !strings.HasSuffix(header.Name, ".pyw") {
			continue
		}
		
		// Create target file path
		targetPath := filepath.Join(targetDir, header.Name)
		
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		
		// Create and write file
		file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", targetPath, err)
		}
		
		_, err = io.Copy(file, tarReader)
		file.Close()
		
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}
	}
	
	return nil
}

// runPylintAnalysis executes Pylint on the extracted code
func (p *PylintAnalyzer) runPylintAnalysis(ctx context.Context, codeDir string) (*AnalysisResult, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	
	// Prepare Pylint command
	args := []string{
		"--output-format=json",
		"--reports=no",
		"--score=no",
		codeDir,
	}
	
	cmd := exec.CommandContext(ctx, p.pylintPath, args...)
	cmd.Dir = codeDir
	
	// Set environment variables
	cmd.Env = append(os.Environ(),
		"PYTHONPATH="+codeDir,
		"PYTHONDONTWRITEBYTECODE=1",
	)
	
	// Execute command
	output, err := cmd.CombinedOutput()
	
	// Pylint returns non-zero exit code when issues are found, this is normal
	var pylintOutput []PylintIssue
	if len(output) > 0 {
		if err := json.Unmarshal(output, &pylintOutput); err != nil {
			return nil, fmt.Errorf("failed to parse Pylint output: %w", err)
		}
	}
	
	// Convert Pylint output to analysis result
	return p.convertPylintOutput(pylintOutput, codeDir)
}

// PylintIssue represents a single issue from Pylint JSON output
type PylintIssue struct {
	Type     string `json:"type"`
	Module   string `json:"module"`
	Obj      string `json:"obj"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Path     string `json:"path"`
	Symbol   string `json:"symbol"`
	Message  string `json:"message"`
	MessageID string `json:"message-id"`
}

// convertPylintOutput converts Pylint JSON output to analysis result
func (p *PylintAnalyzer) convertPylintOutput(pylintIssues []PylintIssue, codeDir string) (*AnalysisResult, error) {
	result := &AnalysisResult{
		Issues:  make([]Issue, 0, len(pylintIssues)),
		Metrics: AnalysisMetrics{},
		Summary: AnalysisSummary{},
	}
	
	// Convert issues
	for _, pylintIssue := range pylintIssues {
		issue := Issue{
			File:     p.sanitizePath(pylintIssue.Path, codeDir),
			Line:     pylintIssue.Line,
			Column:   pylintIssue.Column,
			Severity: p.mapSeverity(pylintIssue.Type),
			Rule:     pylintIssue.Symbol,
			Message:  pylintIssue.Message,
			Category: p.mapCategory(pylintIssue.Type),
		}
		result.Issues = append(result.Issues, issue)
	}
	
	// Calculate metrics
	result.Metrics = p.calculateMetrics(result.Issues, codeDir)
	
	// Generate summary
	result.Summary = p.generateSummary(result.Issues, result.Metrics)
	
	return result, nil
}

// sanitizePath removes the temporary directory prefix from file paths
func (p *PylintAnalyzer) sanitizePath(path, baseDir string) string {
	if strings.HasPrefix(path, baseDir) {
		return strings.TrimPrefix(path, baseDir+"/")
	}
	return path
}

// mapSeverity maps Pylint issue types to severity levels
func (p *PylintAnalyzer) mapSeverity(pylintType string) string {
	switch pylintType {
	case "fatal", "error":
		return "high"
	case "warning":
		return "medium"
	case "convention", "refactor":
		return "low"
	case "info":
		return "info"
	default:
		return "info"
	}
}

// mapCategory maps Pylint issue types to categories
func (p *PylintAnalyzer) mapCategory(pylintType string) string {
	switch pylintType {
	case "fatal", "error":
		return "error"
	case "warning":
		return "warning"
	case "convention":
		return "convention"
	case "refactor":
		return "refactor"
	default:
		return "info"
	}
}

// calculateMetrics computes analysis metrics from issues
func (p *PylintAnalyzer) calculateMetrics(issues []Issue, codeDir string) AnalysisMetrics {
	metrics := AnalysisMetrics{
		TotalFiles: p.countFiles(codeDir),
		IssueCount: len(issues),
	}
	
	// Count issues by severity and category
	for _, issue := range issues {
		switch issue.Severity {
		case "critical":
			metrics.Severity.Critical++
		case "high":
			metrics.Severity.High++
		case "medium":
			metrics.Severity.Medium++
		case "low":
			metrics.Severity.Low++
		case "info":
			metrics.Severity.Info++
		}
		
		switch issue.Category {
		case "error":
			metrics.Categories.Error++
		case "warning":
			metrics.Categories.Warning++
		case "convention":
			metrics.Categories.Convention++
		case "refactor":
			metrics.Categories.Refactor++
		case "fatal":
			metrics.Categories.Fatal++
		}
	}
	
	metrics.AnalyzedFiles = metrics.TotalFiles
	
	return metrics
}

// countFiles counts Python files in directory
func (p *PylintAnalyzer) countFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".pyw")) {
			count++
		}
		return nil
	})
	return count
}

// generateSummary creates a high-level analysis summary
func (p *PylintAnalyzer) generateSummary(issues []Issue, metrics AnalysisMetrics) AnalysisSummary {
	highSeverityCount := metrics.Severity.Critical + metrics.Severity.High
	
	status := "clean"
	recommendation := "Code quality looks good!"
	
	if highSeverityCount > 0 {
		status = "issues_found"
		recommendation = fmt.Sprintf("Found %d high-priority issues that should be addressed", highSeverityCount)
	} else if metrics.IssueCount > 0 {
		status = "minor_issues"
		recommendation = "Minor style and convention issues found - consider addressing for better maintainability"
	}
	
	return AnalysisSummary{
		Status:         status,
		TotalIssues:    metrics.IssueCount,
		HighSeverity:   highSeverityCount,
		Recommendation: recommendation,
	}
}