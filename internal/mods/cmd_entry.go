package mods

import (
	"fmt"
	"os"
)

// ModCmd provides the CLI entrypoint to run mods.
func ModCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		printModHelp()
		return
	}

	switch args[0] {
	case "help":
		printModHelp()
		return
	case "run":
		if err := runMod(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "watch":
		if err := watchMod(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "render":
		if err := modsRenderCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "plan":
		if err := modsPlanCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "reduce":
		if err := modsReduceCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "apply":
		if err := modsApplyCmd(args[1:], controllerURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		printModHelp()
	}
}

func printModHelp() {
	fmt.Println("Usage: ploy mod <subcommand> [options]")
	fmt.Println("Subcommands:")
	fmt.Println("  run      - Execute full workflow remotely (default mode)")
	fmt.Println("  watch    - Attach to a running execution by ID")
	fmt.Println("  render   - Render planner inputs and HCL locally (no submission)")
	fmt.Println("  plan     - Render planner and optionally submit (use --submit)")
	fmt.Println("  reduce   - Render reducer and optionally submit (use --submit)")
	fmt.Println("  apply    - Apply a diff locally and run build gate (use --diff-path/--diff-url)")
	fmt.Println("  help     - Show this help message")
}
