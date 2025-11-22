package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// main regenerates shell completion files from the Cobra command tree.
// This replaces the previous manual clitree-based completion maintenance.
// Run this tool whenever the command tree changes to keep completions in sync.
//
// Usage:
//
//	go run tools/gencomp/main.go
//
// The tool invokes the ploy binary's built-in completion commands (provided by Cobra)
// to generate bash, zsh, and fish completion scripts, then writes them to
// cmd/ploy/autocomplete/ for version control and distribution.
//
// Prerequisites:
// - The ploy binary must be built at ./dist/ploy before running this tool.
// - Run `make build` or `go build -o dist/ploy ./cmd/ploy` first.
func main() {
	// Path to the ploy binary.
	// We use the built binary instead of importing cmd/ploy to avoid import cycles.
	// The binary must exist before running this tool.
	ployBinary := filepath.Join("dist", "ploy")

	// Verify that the ploy binary exists.
	if _, err := os.Stat(ployBinary); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: ploy binary not found at %s\n", ployBinary)
		fmt.Fprintf(os.Stderr, "Run 'make build' or 'go build -o dist/ploy ./cmd/ploy' first.\n")
		os.Exit(1)
	}

	// Output directory for completion files.
	outDir := filepath.Join("cmd", "ploy", "autocomplete")

	// Generate completion for each supported shell.
	// Cobra provides a built-in "completion" command with subcommands for each shell.
	shells := []string{"bash", "zsh", "fish"}

	for _, shell := range shells {
		outFile := filepath.Join(outDir, "ploy."+shell)

		// Invoke: ploy completion <shell> > cmd/ploy/autocomplete/ploy.<shell>
		// Cobra's completion command writes to stdout by default.
		cmd := exec.Command(ployBinary, "completion", shell)

		// Capture both stdout and stderr.
		// The ploy binary currently writes completion output to stderr (due to root.SetOut(stderr)).
		// This is a quirk of the current root command configuration.
		// Capture both streams and use whichever one has content.
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Run the command.
		err := cmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s completion: %v\n", shell, err)
			fmt.Fprintf(os.Stderr, "stderr: %s\n", stderr.String())
			fmt.Fprintf(os.Stderr, "stdout: %s\n", stdout.String())
			os.Exit(1)
		}

		// Check which stream has the completion output.
		// Cobra's completion commands write to the command's Out writer,
		// which we've set to stderr in root.go (root.SetOut(stderr)).
		// So we prefer stderr if it has content, otherwise fall back to stdout.
		output := stderr.Bytes()
		if len(output) == 0 {
			output = stdout.Bytes()
		}

		// Write the generated completion script to the output file.
		// Use 0644 permissions (readable by all, writable by owner).
		if err := os.WriteFile(outFile, output, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outFile, err)
			os.Exit(1)
		}

		fmt.Printf("wrote %s (%d bytes)\n", outFile, len(output))
	}
}
