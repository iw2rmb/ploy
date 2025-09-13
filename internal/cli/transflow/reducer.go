package transflow

import (
    "context"
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
    ctx := context.Background()
    assets, err := runner.RenderReducerAssets()
    if err != nil {
        return fmt.Errorf("failed to render reducer assets: %w", err)
    }

	// Substitute placeholders
	hclBytes, err := os.ReadFile(assets.HCLPath)
	if err != nil {
		return fmt.Errorf("failed to read reducer HCL: %w", err)
	}

    llm := ResolveLLMDefaultsFromEnv()
    model := llm.Model
    toolsJSON := llm.ToolsJSON
    // For reducer, slightly tighter default limits; override only if TRANSFLOW_LIMITS provided
    limitsJSON := os.Getenv("TRANSFLOW_LIMITS")
    if limitsJSON == "" {
        limitsJSON = `{"max_steps":4,"max_tool_calls":8,"timeout":"15m"}`
    }

	runID := ReducerRunID(runner.config.ID)
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

    runner.emit(ctx, "reducer", "render", "info", fmt.Sprintf("Reducer HCL rendered: %s", renderedPath))
    if preserve {
        runner.emit(ctx, "reducer", "preserve", "info", fmt.Sprintf("Workspace preserved at: %s", runner.workspaceDir))
    }

    if os.Getenv("TRANSFLOW_SUBMIT") != "1" {
        runner.emit(ctx, "reducer", "submit", "info", "Skipping reducer submission (unset TRANSFLOW_SUBMIT)")
        return nil
    }

	timeout := ResolveDefaultsFromEnv().ReducerTimeout
    if err := orchestration.SubmitAndWaitTerminal(renderedPath, timeout); err != nil {
        runner.emit(ctx, "reducer", "submit", "error", fmt.Sprintf("reducer job failed: %v", err))
        return fmt.Errorf("reducer job failed: %w", err)
    }

	// Fetch next.json via URL or local path
    if url := os.Getenv("TRANSFLOW_NEXT_URL"); url != "" {
        client := &http.Client{Timeout: 15 * time.Second}
        resp, err := client.Get(url)
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
