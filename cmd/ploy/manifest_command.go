package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

// handleManifest routes manifest subcommands.
func handleManifest(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printManifestUsage(stderr)
		return errors.New("manifest subcommand required")
	}

	switch args[0] {
	case "schema":
		return handleManifestSchema(args[1:], stderr)
	case "validate":
		return handleManifestValidate(args[1:], stderr)
	default:
		printManifestUsage(stderr)
		return fmt.Errorf("unknown manifest subcommand %q", args[0])
	}
}

// printManifestUsage prints the manifest command usage information.
func printManifestUsage(w io.Writer) {
	printCommandUsage(w, "manifest")
}

// handleManifestSchema writes the manifest schema file to the provided writer.
func handleManifestSchema(args []string, stderr io.Writer) error {
	if len(args) > 0 {
		printManifestSchemaUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
	}

	data, err := os.ReadFile(manifestSchemaPath)
	if err != nil {
		return fmt.Errorf("read manifest schema: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "Ploy integration manifest schema (%s):\n", manifestSchemaPath)
	if _, err := stderr.Write(data); err != nil {
		return fmt.Errorf("write manifest schema: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		_, _ = fmt.Fprintln(stderr)
	}
	return nil
}

// printManifestSchemaUsage displays the schema command usage.
func printManifestSchemaUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy manifest schema")
}

// handleManifestValidate validates manifests and optionally rewrites them in place.
func handleManifestValidate(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printManifestValidateUsage(stderr)
		return errors.New("manifest path required")
	}

	rewrite := false
	targets := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg == "--rewrite=v2":
			rewrite = true
		case strings.HasPrefix(arg, "--"):
			printManifestValidateUsage(stderr)
			return fmt.Errorf("unknown flag %q", arg)
		default:
			targets = append(targets, arg)
		}
	}

	if len(targets) == 0 {
		printManifestValidateUsage(stderr)
		return errors.New("manifest path required")
	}

	files, err := collectManifestFiles(targets)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no manifest files found")
	}

	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })

	for _, file := range files {
		comp, err := manifests.LoadFile(file.path)
		if err != nil {
			return err
		}
		if rewrite {
			data, err := manifests.EncodeCompilationToTOML(comp)
			if err != nil {
				return fmt.Errorf("encode manifest %s: %w", file.path, err)
			}
			mode := file.mode
			if mode == 0 {
				mode = 0o644
			}
			if err := os.WriteFile(file.path, data, mode); err != nil {
				return fmt.Errorf("rewrite manifest %s: %w", file.path, err)
			}
			_, _ = fmt.Fprintf(stderr, "Rewrote manifest %s to v2 (%s@%s)\n", file.path, comp.Manifest.Name, comp.Manifest.Version)
			continue
		}
		_, _ = fmt.Fprintf(stderr, "Validated manifest %s (%s@%s)\n", file.path, comp.Manifest.Name, comp.Manifest.Version)
	}
	return nil
}

type manifestFile struct {
	path string
	mode os.FileMode
}

func collectManifestFiles(targets []string) ([]manifestFile, error) {
	var files []manifestFile
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return nil, fmt.Errorf("stat manifest target %s: %w", target, err)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(target)
			if err != nil {
				return nil, fmt.Errorf("read manifest directory %s: %w", target, err)
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
					continue
				}
				fullPath := filepath.Join(target, entry.Name())
				mode := os.FileMode(0)
				if info, err := entry.Info(); err == nil {
					mode = info.Mode().Perm()
				}
				files = append(files, manifestFile{path: fullPath, mode: mode})
			}
			continue
		}
		files = append(files, manifestFile{path: target, mode: info.Mode().Perm()})
	}
	return files, nil
}

// printManifestValidateUsage displays usage guidance for the validate command.
func printManifestValidateUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy manifest validate [--rewrite=v2] <path> [<path>...]")
}
