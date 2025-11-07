package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// ploy-buildgate executes the existing Build Gate against a workspace and prints
// contracts.BuildGateStageMetadata as JSON to stdout. Exit code is 0 regardless
// of pass/fail; consumers should inspect the JSON (StaticChecks[0].Passed).
func main() {
	var (
		workspace string
		profile   string
		timeout   time.Duration
	)
	flag.StringVar(&workspace, "workspace", "", "path to workspace directory (host path) to validate")
	flag.StringVar(&profile, "profile", "", "gate profile: auto|java|java-maven|java-gradle (default: auto)")
	flag.DurationVar(&timeout, "timeout", 10*time.Minute, "overall timeout")
	flag.Parse()

	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		fmt.Fprintln(os.Stderr, "workspace is required")
		os.Exit(2)
	}
	if fi, err := os.Stat(workspace); err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "workspace not a directory: %s\n", workspace)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create Docker runtime (pull images by default) and gate executor.
	rt, err := step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{PullImage: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "docker runtime unavailable: %v\n", err)
		os.Exit(1)
	}
	gate := step.NewDockerGateExecutor(rt)

	p := strings.TrimSpace(profile)
	if p == "auto" {
		p = ""
	}
	spec := &contracts.StepGateSpec{Enabled: true, Profile: p, Env: map[string]string{}}

	meta, err := gate.Execute(ctx, spec, workspace)
	if err != nil {
		// On execution error, emit a minimal JSON with an error field for consumers.
		out := map[string]any{"error": err.Error()}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		os.Exit(1)
	}
	if meta == nil {
		meta = &contracts.BuildGateStageMetadata{}
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(meta)
}
