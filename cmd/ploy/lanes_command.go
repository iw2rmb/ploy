package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/lanes"
)

// handleLanes routes lane subcommands to their handlers.
func handleLanes(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printLanesUsage(stderr)
		return errors.New("lanes subcommand required")
	}

	switch args[0] {
	case "describe":
		return handleLanesDescribe(args[1:], stderr)
	default:
		printLanesUsage(stderr)
		return fmt.Errorf("unknown lanes subcommand %q", args[0])
	}
}

// printLanesUsage outputs the lanes command usage string.
func printLanesUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy lanes <command>")
}

// handleLanesDescribe shows lane metadata and cache preview options.
func handleLanesDescribe(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("lanes describe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	laneName := fs.String("lane", "", "lane identifier to describe")
	commit := fs.String("commit", "", "commit SHA for cache preview")
	snapshot := fs.String("snapshot", "", "snapshot fingerprint for cache preview")
	manifest := fs.String("manifest", "", "integration manifest version for cache preview")
	aster := fs.String("aster", "", "comma-separated Aster toggles for cache preview")
	if err := fs.Parse(args); err != nil {
		printLanesDescribeUsage(stderr)
		return err
	}

	trimmedLane := strings.TrimSpace(*laneName)
	if trimmedLane == "" {
		printLanesDescribeUsage(stderr)
		return errors.New("lane is required")
	}

	reg, err := laneRegistryLoader(laneConfigDir)
	if err != nil {
		return err
	}

	desc, err := reg.Describe(trimmedLane, lanes.DescribeOptions{
		CommitSHA:           strings.TrimSpace(*commit),
		SnapshotFingerprint: strings.TrimSpace(*snapshot),
		ManifestVersion:     strings.TrimSpace(*manifest),
		AsterToggles:        splitToggles(*aster),
	})
	if err != nil {
		return err
	}

	printLaneDescription(stderr, desc)
	return nil
}

// printLanesDescribeUsage describes the flags accepted by lanes describe.
func printLanesDescribeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy lanes describe --lane <lane-name> [--commit <sha>] [--snapshot <fingerprint>] [--manifest <version>] [--aster <a,b,c>]")
}

// printLaneDescription renders lane details and cache parameters.
func printLaneDescription(w io.Writer, desc lanes.Description) {
	_, _ = fmt.Fprintf(w, "Lane: %s\n", desc.Lane.Name)
	if desc.Lane.Description != "" {
		_, _ = fmt.Fprintf(w, "Description: %s\n", desc.Lane.Description)
	}
	_, _ = fmt.Fprintf(w, "Runtime Family: %s\n", desc.Lane.RuntimeFamily)
	_, _ = fmt.Fprintf(w, "Cache Namespace: %s\n", desc.Lane.CacheNamespace)
	if len(desc.Lane.Commands.Setup) > 0 {
		_, _ = fmt.Fprintf(w, "Setup Command: %s\n", strings.Join(desc.Lane.Commands.Setup, " "))
	}
	_, _ = fmt.Fprintf(w, "Build Command: %s\n", strings.Join(desc.Lane.Commands.Build, " "))
	_, _ = fmt.Fprintf(w, "Test Command: %s\n", strings.Join(desc.Lane.Commands.Test, " "))
	_, _ = fmt.Fprintf(w, "Cache Key Preview: %s\n", desc.CacheKey)

	inputs := []string{}
	if desc.Parameters.CommitSHA != "" {
		inputs = append(inputs, fmt.Sprintf("commit=%s", desc.Parameters.CommitSHA))
	}
	if desc.Parameters.SnapshotFingerprint != "" {
		inputs = append(inputs, fmt.Sprintf("snapshot=%s", desc.Parameters.SnapshotFingerprint))
	}
	if desc.Parameters.ManifestVersion != "" {
		inputs = append(inputs, fmt.Sprintf("manifest=%s", desc.Parameters.ManifestVersion))
	}
	if len(desc.Parameters.AsterToggles) > 0 {
		inputs = append(inputs, fmt.Sprintf("aster=%s", strings.Join(desc.Parameters.AsterToggles, ",")))
	}
	if len(inputs) > 0 {
		_, _ = fmt.Fprintf(w, "Inputs: %s\n", strings.Join(inputs, "; "))
	}
}
