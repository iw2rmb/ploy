package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

func (d *AnalysisDispatcher) submitToNomad(job *AnalysisJob) error {
	tmpl, ok := d.jobTemplates[job.Analyzer]
	if !ok {
		return fmt.Errorf("no template for analyzer: %s", job.Analyzer)
	}

	consulAddr := os.Getenv("CONSUL_HTTP_ADDR")
	if consulAddr == "" {
		consulAddr = "http://localhost:8500"
	}

	configJSON := "{}"
	if job.Config != nil {
		if bytes, err := json.Marshal(job.Config); err == nil {
			configJSON = string(bytes)
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"JobID":      job.ID,
		"InputURL":   job.InputURL,
		"OutputURL":  job.OutputURL,
		"ConsulAddr": consulAddr,
		"ConfigJSON": configJSON,
	}); err != nil {
		return fmt.Errorf("failed to generate job HCL: %w", err)
	}

	tmp, err := os.CreateTemp("", "analysis-*.hcl")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		return err
	}
	if cerr := tmp.Close(); cerr != nil {
		return cerr
	}

	return orchestration.Submit(tmp.Name())
}
