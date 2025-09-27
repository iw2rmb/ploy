package main

import (
	"bytes"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"strings"
	"testing"
)

func TestHandleLanesDescribePrintsDetails(t *testing.T) {
	buf := &bytes.Buffer{}
	prevLoader := laneRegistryLoader
	prevDir := laneConfigDir
	defer func() {
		laneRegistryLoader = prevLoader
		laneConfigDir = prevDir
	}()

	desc := lanes.Description{
		Lane: lanes.Spec{
			Name:           "node-wasm",
			Description:    "Node lane",
			RuntimeFamily:  "wasm-node",
			CacheNamespace: "node",
			Commands: lanes.Commands{
				Build: []string{"npm", "ci"},
				Test:  []string{"npm", "test"},
			},
			Job: lanes.JobSpec{
				Image:   "registry.dev/node:20",
				Command: []string{"npm", "test"},
				Env:     map[string]string{"NODE_ENV": "test"},
				Resources: lanes.JobResources{
					CPU:    "2000m",
					Memory: "4Gi",
				},
				Priority: "standard",
			},
		},
		CacheKey: "node/node-wasm@commit=abc@...",
	}

	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		return &fakeLaneRegistry{description: desc}, nil
	}
	laneConfigDir = "ignored"

	err := handleLanes([]string{"describe", "--lane", "node-wasm", "--commit", "abc"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	for _, fragment := range []string{"node-wasm", "wasm-node", "node", "registry.dev/node:20", "Job Env", "Job Resources", "node/node-wasm@commit=abc"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleLanesRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleLanes(nil, buf)
	if err == nil {
		t.Fatal("expected error when lanes subcommand missing")
	}
	if !strings.Contains(buf.String(), "Usage: ploy lanes") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}
