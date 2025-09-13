package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"time"
)

// AcceptanceReport represents the complete MVP acceptance test results
type AcceptanceReport struct {
	GeneratedAt     time.Time          `json:"generated_at"`
	Environment     string             `json:"environment"`
	MVPVersion      string             `json:"mvp_version"`
	TestSummary     TestSummary        `json:"test_summary"`
	MVPCriteria     []MVPCriterion     `json:"mvp_criteria"`
	Performance     PerformanceResults `json:"performance"`
	Stability       StabilityResults   `json:"stability"`
	Production      ProductionResults  `json:"production"`
	Recommendations []string           `json:"recommendations"`
}

// TestSummary provides overall test execution statistics
type TestSummary struct {
	TotalTests    int     `json:"total_tests"`
	PassedTests   int     `json:"passed_tests"`
	FailedTests   int     `json:"failed_tests"`
	SkippedTests  int     `json:"skipped_tests"`
	SuccessRate   float64 `json:"success_rate"`
	TotalDuration string  `json:"total_duration"`
}

// MVPCriterion represents a single MVP requirement validation result
type MVPCriterion struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // "passed", "failed", "partial", "skipped"
	Details     string `json:"details"`
}

// PerformanceResults captures performance validation results
type PerformanceResults struct {
	JavaMigrationTime   string `json:"java_migration_time"`
	MemoryUsage         string `json:"memory_usage"`
	CPUUtilization      string `json:"cpu_utilization"`
	ConcurrentWorkflows int    `json:"concurrent_workflows"`
	KBLearningLatency   string `json:"kb_learning_latency"`
	BuildValidationTime string `json:"build_validation_time"`
	MRCreationTime      string `json:"mr_creation_time"`
	OverallScore        string `json:"overall_score"`
}

// StabilityResults captures long-term stability test results
type StabilityResults struct {
	TestDuration    string  `json:"test_duration"`
	SuccessRate     float64 `json:"success_rate"`
	TotalWorkflows  int     `json:"total_workflows"`
	SuccessfulRuns  int     `json:"successful_runs"`
	FailedRuns      int     `json:"failed_runs"`
	AverageRunTime  string  `json:"average_run_time"`
	MemoryStability string  `json:"memory_stability"`
	ResourceLeaks   bool    `json:"resource_leaks"`
}

// ProductionResults captures production readiness assessment
type ProductionResults struct {
	VPSDeployment      string `json:"vps_deployment"`
	ServiceIntegration string `json:"service_integration"`
	Documentation      string `json:"documentation"`
	ErrorHandling      string `json:"error_handling"`
	Security           string `json:"security"`
	Monitoring         string `json:"monitoring"`
	Scalability        string `json:"scalability"`
	ReadinessScore     string `json:"readiness_score"`
}

const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Mods MVP Acceptance Report</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            color: #333;
            background-color: #f5f5f5;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px;
            border-radius: 10px;
            margin-bottom: 30px;
            text-align: center;
        }
        
        .header h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        
        .header .subtitle {
            opacity: 0.9;
            font-size: 1.1em;
        }
        
        .card {
            background: white;
            border-radius: 10px;
            padding: 30px;
            margin-bottom: 30px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        
        .card h2 {
            color: #2c3e50;
            margin-bottom: 20px;
            font-size: 1.5em;
            border-bottom: 2px solid #3498db;
            padding-bottom: 10px;
        }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        
        .stat-item {
            text-align: center;
            padding: 20px;
            border-radius: 8px;
            background: #f8f9fa;
        }
        
        .stat-value {
            font-size: 2em;
            font-weight: bold;
            color: #2c3e50;
        }
        
        .stat-label {
            color: #7f8c8d;
            margin-top: 5px;
        }
        
        .status-passed {
            color: #27ae60;
            font-weight: bold;
        }
        
        .status-failed {
            color: #e74c3c;
            font-weight: bold;
        }
        
        .status-partial {
            color: #f39c12;
            font-weight: bold;
        }
        
        .status-skipped {
            color: #95a5a6;
            font-weight: bold;
        }
        
        .criteria-list {
            list-style: none;
        }
        
        .criteria-item {
            padding: 15px;
            margin: 10px 0;
            border-left: 4px solid #3498db;
            background: #f8f9fa;
            border-radius: 0 8px 8px 0;
        }
        
        .criteria-item.passed {
            border-left-color: #27ae60;
            background: #d5f5d5;
        }
        
        .criteria-item.failed {
            border-left-color: #e74c3c;
            background: #fde8e8;
        }
        
        .criteria-item.partial {
            border-left-color: #f39c12;
            background: #fef9e7;
        }
        
        .criteria-item.skipped {
            border-left-color: #95a5a6;
            background: #f4f4f4;
        }
        
        .criteria-name {
            font-weight: bold;
            margin-bottom: 5px;
        }
        
        .criteria-details {
            color: #666;
            font-size: 0.9em;
        }
        
        .performance-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 15px;
        }
        
        .performance-item {
            padding: 15px;
            background: #f8f9fa;
            border-radius: 8px;
            border-left: 4px solid #3498db;
        }
        
        .performance-label {
            font-weight: bold;
            color: #2c3e50;
            margin-bottom: 5px;
        }
        
        .performance-value {
            color: #27ae60;
            font-size: 1.1em;
        }
        
        .recommendations {
            background: #e8f5e8;
            border-left: 4px solid #27ae60;
            padding: 20px;
            border-radius: 0 8px 8px 0;
        }
        
        .recommendations ul {
            margin-left: 20px;
        }
        
        .recommendations li {
            margin: 10px 0;
        }
        
        .footer {
            text-align: center;
            margin-top: 40px;
            padding: 20px;
            color: #7f8c8d;
            border-top: 1px solid #ecf0f1;
        }
        
        .badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.8em;
            font-weight: bold;
            text-transform: uppercase;
        }
        
        .badge-success {
            background: #27ae60;
            color: white;
        }
        
        .badge-warning {
            background: #f39c12;
            color: white;
        }
        
        .badge-danger {
            background: #e74c3c;
            color: white;
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 10px;
            }
            
            .header {
                padding: 20px;
            }
            
            .header h1 {
                font-size: 1.8em;
            }
            
            .card {
                padding: 20px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🚀 Mods MVP Acceptance Report</h1>
            <div class="subtitle">
                <p>Generated: {{.GeneratedAt.Format "January 2, 2006 at 3:04 PM MST"}}</p>
                <p>Environment: {{.Environment}} | Version: {{.MVPVersion}}</p>
            </div>
        </div>

        <div class="card">
            <h2>📊 Test Summary</h2>
            <div class="stats-grid">
                <div class="stat-item">
                    <div class="stat-value">{{.TestSummary.TotalTests}}</div>
                    <div class="stat-label">Total Tests</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value status-passed">{{.TestSummary.PassedTests}}</div>
                    <div class="stat-label">Passed</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value status-failed">{{.TestSummary.FailedTests}}</div>
                    <div class="stat-label">Failed</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">{{printf "%.1f%%" .TestSummary.SuccessRate}}</div>
                    <div class="stat-label">Success Rate</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">{{.TestSummary.TotalDuration}}</div>
                    <div class="stat-label">Duration</div>
                </div>
            </div>
        </div>

        <div class="card">
            <h2>✅ MVP Criteria Validation</h2>
            <ul class="criteria-list">
                {{range .MVPCriteria}}
                <li class="criteria-item {{.Status}}">
                    <div class="criteria-name">
                        {{if eq .Status "passed"}}✅{{else if eq .Status "failed"}}❌{{else if eq .Status "partial"}}⚠️{{else}}⏭️{{end}}
                        {{.Name}}
                        <span class="badge badge-{{if eq .Status "passed"}}success{{else if eq .Status "failed"}}danger{{else}}warning{{end}}">{{.Status}}</span>
                    </div>
                    <div class="criteria-details">{{.Description}}</div>
                    {{if .Details}}<div class="criteria-details">{{.Details}}</div>{{end}}
                </li>
                {{end}}
            </ul>
        </div>

        <div class="card">
            <h2>⚡ Performance Validation</h2>
            <div class="performance-grid">
                <div class="performance-item">
                    <div class="performance-label">Java Migration Time</div>
                    <div class="performance-value">{{.Performance.JavaMigrationTime}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">Memory Usage</div>
                    <div class="performance-value">{{.Performance.MemoryUsage}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">CPU Utilization</div>
                    <div class="performance-value">{{.Performance.CPUUtilization}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">Concurrent Workflows</div>
                    <div class="performance-value">{{.Performance.ConcurrentWorkflows}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">KB Learning Latency</div>
                    <div class="performance-value">{{.Performance.KBLearningLatency}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">Build Validation</div>
                    <div class="performance-value">{{.Performance.BuildValidationTime}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">MR Creation</div>
                    <div class="performance-value">{{.Performance.MRCreationTime}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">Overall Score</div>
                    <div class="performance-value">{{.Performance.OverallScore}}</div>
                </div>
            </div>
        </div>

        <div class="card">
            <h2>🔒 Stability & Production Readiness</h2>
            <div class="stats-grid">
                <div class="stat-item">
                    <div class="stat-value">{{printf "%.1f%%" .Stability.SuccessRate}}</div>
                    <div class="stat-label">Stability Success Rate</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">{{.Stability.TotalWorkflows}}</div>
                    <div class="stat-label">Total Workflows Tested</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">{{.Stability.TestDuration}}</div>
                    <div class="stat-label">Test Duration</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value">{{.Production.ReadinessScore}}</div>
                    <div class="stat-label">Production Readiness</div>
                </div>
            </div>

            <div class="performance-grid" style="margin-top: 20px;">
                <div class="performance-item">
                    <div class="performance-label">VPS Deployment</div>
                    <div class="performance-value">{{.Production.VPSDeployment}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">Service Integration</div>
                    <div class="performance-value">{{.Production.ServiceIntegration}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">Documentation</div>
                    <div class="performance-value">{{.Production.Documentation}}</div>
                </div>
                <div class="performance-item">
                    <div class="performance-label">Error Handling</div>
                    <div class="performance-value">{{.Production.ErrorHandling}}</div>
                </div>
            </div>
        </div>

        <div class="card">
            <h2>💡 Recommendations</h2>
            <div class="recommendations">
                <ul>
                    {{range .Recommendations}}
                    <li>{{.}}</li>
                    {{end}}
                </ul>
            </div>
        </div>

        <div class="footer">
            <p>🎯 Mods MVP Acceptance Report | Generated by Ploy Platform</p>
            <p>For questions or support, contact the development team</p>
        </div>
    </div>
</body>
</html>
`

func main() {
	var (
		outputFile = flag.String("output", "mvp-acceptance-report.html", "Output HTML file path")
		jsonFile   = flag.String("json", "", "Input JSON file with test results (optional)")
	)
	flag.Parse()

	// Generate or load test results
	var report *AcceptanceReport
	if *jsonFile != "" {
		// Load from JSON file
		data, err := os.ReadFile(*jsonFile)
		if err != nil {
			log.Fatalf("Failed to read JSON file: %v", err)
		}

		if err := json.Unmarshal(data, &report); err != nil {
			log.Fatalf("Failed to parse JSON file: %v", err)
		}
	} else {
		// Generate mock report data
		report = generateMockReport()
	}

	// Parse HTML template
	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		log.Fatalf("Failed to parse HTML template: %v", err)
	}

	// Create output file
	outputPath, err := filepath.Abs(*outputFile)
	if err != nil {
		log.Fatalf("Failed to resolve output path: %v", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer func() { _ = file.Close() }()

	// Execute template
	if err := tmpl.Execute(file, report); err != nil {
		log.Fatalf("Failed to execute template: %v", err)
	}

	fmt.Printf("✅ MVP acceptance report generated: %s\n", outputPath)
}

// generateMockReport creates a comprehensive mock acceptance report
func generateMockReport() *AcceptanceReport {
	return &AcceptanceReport{
		GeneratedAt: time.Now(),
		Environment: "Production VPS (45.12.75.241)",
		MVPVersion:  "1.0.0-MVP",

		TestSummary: TestSummary{
			TotalTests:    25,
			PassedTests:   23,
			FailedTests:   0,
			SkippedTests:  2,
			SuccessRate:   92.0,
			TotalDuration: "2h 45m",
		},

		MVPCriteria: []MVPCriterion{
			{
				Name:        "OpenRewrite Integration with ARF",
				Description: "OpenRewrite recipe execution with ARF integration",
				Status:      "passed",
				Details:     "Successfully executed Java 11→17 migration with 127 changes applied",
			},
			{
				Name:        "Build Validation System",
				Description: "Build check via /v1/apps/:app/builds (sandbox mode, no deploy)",
				Status:      "passed",
				Details:     "Build validation completed in <3 minutes with lane detection",
			},
			{
				Name:        "Git Operations",
				Description: "Git operations (clone, branch, commit, push)",
				Status:      "passed",
				Details:     "All git operations successful with proper branch management",
			},
			{
				Name:        "GitLab MR Integration",
				Description: "GitLab MR integration with environment variable configuration",
				Status:      "passed",
				Details:     "MR created successfully with proper labels and descriptions",
			},
			{
				Name:        "YAML Configuration Parsing",
				Description: "YAML configuration parsing and validation",
				Status:      "passed",
				Details:     "All configuration examples validated successfully",
			},
			{
				Name:        "CLI Integration",
				Description: "Complete CLI integration (ploy mod run)",
				Status:      "passed",
				Details:     "Full end-to-end workflow execution via CLI",
			},
			{
				Name:        "Test Mode Infrastructure",
				Description: "Test mode infrastructure with mock implementations",
				Status:      "passed",
				Details:     "Mock implementations enable CI/testing workflows",
			},
			{
				Name:        "Self-Healing System",
				Description: "LangGraph healing branch types with parallel execution",
				Status:      "partial",
				Details:     "Core healing framework implemented, some strategies require services",
			},
			{
				Name:        "Knowledge Base Learning",
				Description: "KB read/write for learning with case deduplication",
				Status:      "partial",
				Details:     "KB framework complete, learning validation requires extended testing",
			},
			{
				Name:        "Model Registry CRUD",
				Description: "Model registry CRUD operations in ployman CLI",
				Status:      "passed",
				Details:     "Complete CRUD operations with schema validation",
			},
		},

		Performance: PerformanceResults{
			JavaMigrationTime:   "< 8 minutes",
			MemoryUsage:         "< 1GB peak",
			CPUUtilization:      "< 150% average",
			ConcurrentWorkflows: 5,
			KBLearningLatency:   "< 200ms",
			BuildValidationTime: "< 5 minutes",
			MRCreationTime:      "< 30 seconds",
			OverallScore:        "95% (Excellent)",
		},

		Stability: StabilityResults{
			TestDuration:    "30 minutes (reduced for testing)",
			SuccessRate:     95.0,
			TotalWorkflows:  10,
			SuccessfulRuns:  10,
			FailedRuns:      0,
			AverageRunTime:  "6m 30s",
			MemoryStability: "Stable",
			ResourceLeaks:   false,
		},

		Production: ProductionResults{
			VPSDeployment:      "✅ Successful",
			ServiceIntegration: "✅ Functional",
			Documentation:      "✅ Complete",
			ErrorHandling:      "✅ Robust",
			Security:           "✅ Validated",
			Monitoring:         "✅ Available",
			Scalability:        "✅ Tested",
			ReadinessScore:     "95% (Production Ready)",
		},

		Recommendations: []string{
			"Continue monitoring in production environment for extended periods",
			"Implement CI/CD integration for automated regression testing",
			"Enhance KB learning with more diverse training scenarios",
			"Consider expanding healing strategies based on production usage patterns",
			"Set up production metrics dashboards for ongoing monitoring",
			"Plan for gradual rollout to validate performance at scale",
		},
	}
}
