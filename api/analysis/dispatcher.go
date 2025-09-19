package analysis

import (
	"os"
	"text/template"
	"time"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	istorage "github.com/iw2rmb/ploy/internal/storage"
)

// AnalysisJob represents a code analysis job.
type AnalysisJob struct {
	ID          string                  `json:"id"`
	Analyzer    string                  `json:"analyzer"`
	Language    string                  `json:"language"`
	InputURL    string                  `json:"input_url"`
	OutputURL   string                  `json:"output_url"`
	Config      map[string]interface{}  `json:"config,omitempty"`
	Status      string                  `json:"status"`
	CreatedAt   time.Time               `json:"created_at"`
	StartedAt   *time.Time              `json:"started_at"`
	CompletedAt *time.Time              `json:"completed_at"`
	Result      *LanguageAnalysisResult `json:"result,omitempty"`
	Error       string                  `json:"error,omitempty"`
}

// AnalysisDispatcher handles job dispatch and monitoring for code analysis.
type AnalysisDispatcher struct {
	kv             orchestration.KV
	storage        istorage.Storage
	jobTemplates   map[string]*template.Template
	storageBaseURL string
	submitFn       func(*AnalysisJob) error
	deregisterFn   func(string, bool) error
}

// NewAnalysisDispatcherOrchestration creates a new dispatcher backed by the orchestration facade.
func NewAnalysisDispatcherOrchestration(storage istorage.Storage) (*AnalysisDispatcher, error) {
	storageBaseURL := "http://seaweedfs-filer.storage.ploy.local:8888"
	if url := os.Getenv("SEAWEEDFS_URL"); url != "" {
		storageBaseURL = url
	}

	dispatcher := &AnalysisDispatcher{
		kv:             orchestration.NewKV(),
		storage:        storage,
		jobTemplates:   make(map[string]*template.Template),
		storageBaseURL: storageBaseURL,
	}

	dispatcher.submitFn = dispatcher.submitToNomad
	dispatcher.deregisterFn = orchestration.DeregisterJob

	if err := dispatcher.loadJobTemplates(); err != nil {
		return nil, err
	}

	return dispatcher, nil
}
