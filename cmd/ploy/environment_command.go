package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
)

// handleEnvironment routes environment subcommands.
func handleEnvironment(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printEnvironmentUsage(stderr)
		return errors.New("environment subcommand required")
	}

	switch args[0] {
	case "materialize":
		return handleEnvironmentMaterialize(args[1:], stderr)
	default:
		printEnvironmentUsage(stderr)
		return fmt.Errorf("unknown environment subcommand %q", args[0])
	}
}

// printEnvironmentUsage prints the environment command overview.
func printEnvironmentUsage(w io.Writer) {
	printCommandUsage(w, "environment")
}

// handleEnvironmentMaterialize materialises an environment plan or execution request.
func handleEnvironmentMaterialize(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("environment materialize", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	app := fs.String("app", "", "application identifier")
    // tenant removed
	dryRun := fs.Bool("dry-run", false, "plan resources without hydrating caches")
	manifestOverride := fs.String("manifest", "", "override manifest in the form name@version")
	aster := fs.String("aster", "", "comma-separated optional Aster toggles to include")

	commitArg := ""
	parseArgs := args
	if len(parseArgs) > 0 && !strings.HasPrefix(strings.TrimSpace(parseArgs[0]), "-") {
		commitArg = parseArgs[0]
		parseArgs = parseArgs[1:]
	}

	if err := fs.Parse(parseArgs); err != nil {
		printEnvironmentMaterializeUsage(stderr)
		return err
	}

	remaining := fs.Args()
	if commitArg == "" {
		if len(remaining) == 0 {
			printEnvironmentMaterializeUsage(stderr)
			return errors.New("commit SHA required")
		}
		commitArg = remaining[0]
		remaining = remaining[1:]
	}
	if len(remaining) > 0 {
		printEnvironmentMaterializeUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(remaining, " "))
	}

	commit := strings.TrimSpace(commitArg)
	if commit == "" {
		printEnvironmentMaterializeUsage(stderr)
		return errors.New("commit SHA required")
	}

	trimmedApp := strings.TrimSpace(*app)
	if trimmedApp == "" {
		printEnvironmentMaterializeUsage(stderr)
		return errors.New("app is required")
	}

    // tenant removed

    manifestName, manifestVersion, err := parseManifestOverride(*manifestOverride, trimmedApp)
	if err != nil {
		printEnvironmentMaterializeUsage(stderr)
		return err
	}

    compiler, err := manifestRegistryLoader(manifestConfigDir)
    if err != nil {
        return fmt.Errorf("load manifests: %w", err)
    }

	compiled, err := compiler.Compile(context.Background(), contracts.ManifestReference{Name: manifestName, Version: manifestVersion})
	if err != nil {
		return err
	}

    service, err := environmentServiceFactory()
    if err != nil {
        return err
    }

	asterActive := asterEnabled()
	asterToggles := []string(nil)
	if asterActive {
		asterToggles = splitToggles(*aster)
	}

    result, err := service.Materialize(context.Background(), environments.Request{
        CommitSHA:    commit,
        App:          trimmedApp,
        DryRun:       *dryRun,
        Manifest:     compiled,
        ManifestRef:  contracts.ManifestReference{Name: compiled.Manifest.Name, Version: compiled.Manifest.Version},
        AsterEnabled: asterActive,
        AsterToggles: asterToggles,
    })
	if err != nil {
		return err
	}

	printEnvironmentMaterialize(stderr, result)
	return nil
}

// printEnvironmentMaterializeUsage shows the materialize flags and arguments.
func printEnvironmentMaterializeUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy environment materialize <commit-sha> --app <app> [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]")
}

// printEnvironmentMaterialize renders the environment result summary.
func printEnvironmentMaterialize(w io.Writer, result environments.Result) {
	_, _ = fmt.Fprintf(w, "Environment: %s@%s\n", result.App, result.CommitSHA)
	mode := "execute"
	if result.DryRun {
		mode = "dry-run"
	}
	_, _ = fmt.Fprintf(w, "Mode: %s\n", mode)
	_, _ = fmt.Fprintf(w, "Manifest: %s@%s\n", result.ManifestRef.Name, result.ManifestRef.Version)
	if len(result.AsterToggles) > 0 {
		_, _ = fmt.Fprintf(w, "Aster Toggles: %s\n", strings.Join(result.AsterToggles, ", "))
	}

    // Snapshots removed from environment materialization.

	if len(result.Caches) == 0 {
		_, _ = fmt.Fprintln(w, "Caches: none")
	} else {
		_, _ = fmt.Fprintln(w, "Caches:")
		for _, cache := range result.Caches {
			status := "pending"
			if cache.Hydrated {
				status = "hydrated"
			}
			_, _ = fmt.Fprintf(w, "  - %s -> %s (%s)\n", cache.Lane, cache.CacheKey, status)
		}
	}
}

// parseManifestOverride resolves the manifest override flag into name and version.
func parseManifestOverride(candidate, fallback string) (string, string, error) {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return fallback, "", nil
	}
	parts := strings.Split(trimmed, "@")
	if len(parts) > 2 {
		return "", "", errors.New("manifest override must be <name>@<version>")
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return "", "", errors.New("manifest override requires a name")
	}
	version := ""
	if len(parts) == 2 {
		version = strings.TrimSpace(parts[1])
	}
	return name, version, nil
}
