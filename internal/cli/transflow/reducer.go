package transflow

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// executeReducerMode renders and optionally submits reducer job
func executeReducerMode(runner *TransflowRunner, preserve bool) error {
	assets, err := runner.RenderReducerAssets()
	if err != nil {
		return fmt.Errorf("failed to render reducer assets: %w", err)
	}

	// Substitute placeholders
	hclBytes, err := os.ReadFile(assets.HCLPath)
	if err != nil {
		return fmt.Errorf("failed to read reducer HCL: %w", err)
	}

	model := os.Getenv("TRANSFLOW_MODEL")
	if model == "" {
		model = "gpt-4o-mini@2024-08-06"
	}
	toolsJSON := os.Getenv("TRANSFLOW_TOOLS")
	if toolsJSON == "" {
		toolsJSON = `{"file":{"allow":["src/**","pom.xml"]}}`
	}
	limitsJSON := os.Getenv("TRANSFLOW_LIMITS")
	if limitsJSON == "" {
		limitsJSON = `{"max_steps":4,"max_tool_calls":8,"timeout":"15m"}`
	}

	runID := fmt.Sprintf("%s-%d", runner.config.ID, time.Now().Unix())
	rendered := strings.NewReplacer(
		"${MODEL}", model,
		"${TOOLS_JSON}", toolsJSON,
		"${LIMITS_JSON}", limitsJSON,
		"${RUN_ID}", runID,
	).Replace(string(hclBytes))

	renderedPath := filepath.Join(filepath.Dir(assets.HCLPath), "reducer.rendered.hcl")
	if err := os.WriteFile(renderedPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("failed to write rendered HCL: %w", err)
	}

	fmt.Printf("Reducer HCL rendered: %s\n", renderedPath)
	if preserve {
		fmt.Printf("Workspace preserved at: %s\n", runner.workspaceDir)
	}

	if os.Getenv("TRANSFLOW_SUBMIT") != "1" {
		fmt.Println("Skipping reducer submission (unset TRANSFLOW_SUBMIT).")
		return nil
	}

    timeout := ResolveDefaultsFromEnv().ReducerTimeout
    if err := orchestration.SubmitAndWaitTerminal(renderedPath, timeout); err != nil {
        return fmt.Errorf("reducer job failed: %w", err)
    }

	// Fetch next.json via URL or local path
	if url := os.Getenv("TRANSFLOW_NEXT_URL"); url != "" {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			printNextSummary(b)
		} else if err != nil {
			fmt.Printf("Failed to fetch next URL: %v\n", err)
		}
	}

	np := os.Getenv("TRANSFLOW_NEXT_PATH")
	if np == "" {
		np = filepath.Join(filepath.Dir(renderedPath), "out", "next.json")
	}
	if b, err := os.ReadFile(np); err == nil {
		printNextSummary(b)
	} else {
		fmt.Println("Reducer job completed. Could not read next.json; set TRANSFLOW_NEXT_PATH or TRANSFLOW_NEXT_URL.")
	}

	return nil
}
