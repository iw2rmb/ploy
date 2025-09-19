package mods

import (
	"flag"
	"fmt"
	"os"
)

// modsRenderCmd: planner render (no submission)
func modsRenderCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod render", flag.ContinueOnError)
	file := fs.String("f", "", "mods YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <mods.yaml>")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "mods-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer func() { _ = os.RemoveAll(wd) }()
	}
	integrations := NewModIntegrationsWithTestMode(controllerURL, wd, true)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	_ = verbose // reserved; executePlannerMode emits events/logs already
	return executePlannerMode(runner, *preserve, *verbose)
}

// modsPlanCmd: render planner and optionally submit when --submit provided
func modsPlanCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod plan", flag.ContinueOnError)
	file := fs.String("f", "", "mods YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	submit := fs.Bool("submit", false, "submit planner job after rendering")
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <mods.yaml>")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "mods-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer func() { _ = os.RemoveAll(wd) }()
	}
	integrations := NewModIntegrationsWithTestMode(controllerURL, wd, false)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	if *submit {
		_ = os.Setenv("MODS_SUBMIT", "1")
	} else {
		_ = os.Unsetenv("MODS_SUBMIT")
	}
	return executePlannerMode(runner, *preserve, *verbose)
}

// modsReduceCmd: render reducer and optionally submit when --submit
func modsReduceCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod reduce", flag.ContinueOnError)
	file := fs.String("f", "", "mods YAML file")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	submit := fs.Bool("submit", false, "submit reducer job after rendering")
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <mods.yaml>")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "mods-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer func() { _ = os.RemoveAll(wd) }()
	}
	integrations := NewModIntegrationsWithTestMode(controllerURL, wd, false)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	if *submit {
		_ = os.Setenv("MODS_SUBMIT", "1")
	} else {
		_ = os.Unsetenv("MODS_SUBMIT")
	}
	_ = verbose
	return executeReducerMode(runner, *preserve)
}

// modsApplyCmd: apply a diff to repo and run build gate
func modsApplyCmd(args []string, controllerURL string) error {
	fs := flag.NewFlagSet("mod apply", flag.ContinueOnError)
	file := fs.String("f", "", "mods YAML file")
	diffPath := fs.String("diff-path", "", "local unified diff file path")
	diffURL := fs.String("diff-url", "", "URL to download unified diff")
	workDir := fs.String("work-dir", "", "working directory (default: temp dir)")
	preserve := fs.Bool("preserve-workspace", false, "do not delete the temporary workspace")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("missing -f <mods.yaml>")
	}
	if *diffPath == "" && *diffURL == "" {
		return fmt.Errorf("provide --diff-path or --diff-url")
	}
	cfg, err := LoadConfig(*file)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	wd := *workDir
	if wd == "" {
		if wd, err = os.MkdirTemp("", "mods-*"); err != nil {
			return err
		}
	}
	if !*preserve {
		defer func() { _ = os.RemoveAll(wd) }()
	}
	integrations := NewModIntegrationsWithTestMode(controllerURL, wd, false)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		return err
	}
	if *diffURL != "" {
		_ = os.Setenv("MODS_DIFF_URL", *diffURL)
	} else {
		_ = os.Unsetenv("MODS_DIFF_URL")
	}
	if *diffPath != "" {
		_ = os.Setenv("MODS_DIFF_PATH", *diffPath)
	} else {
		_ = os.Unsetenv("MODS_DIFF_PATH")
	}
	return executeApplyFirst(runner)
}
