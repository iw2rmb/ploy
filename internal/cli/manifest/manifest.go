package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

var (
	ErrManifestPathRequired = errors.New("manifest path required")
	ErrNoManifestsFound     = errors.New("no manifest files found")
)

type ValidateOptions struct {
	Targets []string
	Rewrite bool
}

type ValidateResult struct {
	Path      string
	Name      string
	Version   string
	Rewritten bool
}

type manifestFile struct {
	path string
	mode os.FileMode
}

func LoadSchema(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest schema: %w", err)
	}
	return data, nil
}

func Validate(opts ValidateOptions) ([]ValidateResult, error) {
	if len(opts.Targets) == 0 {
		return nil, ErrManifestPathRequired
	}
	files, err := collectManifestFiles(opts.Targets)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNoManifestsFound
	}

	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })

	results := make([]ValidateResult, 0, len(files))
	for _, file := range files {
		comp, err := manifests.LoadFile(file.path)
		if err != nil {
			return nil, err
		}
		if opts.Rewrite {
			data, err := manifests.EncodeCompilationToTOML(comp)
			if err != nil {
				return nil, fmt.Errorf("encode manifest %s: %w", file.path, err)
			}
			mode := file.mode
			if mode == 0 {
				mode = 0o644
			}
			if err := os.WriteFile(file.path, data, mode); err != nil {
				return nil, fmt.Errorf("rewrite manifest %s: %w", file.path, err)
			}
			results = append(results, ValidateResult{
				Path:      file.path,
				Name:      comp.Manifest.Name,
				Version:   comp.Manifest.Version,
				Rewritten: true,
			})
			continue
		}
		results = append(results, ValidateResult{
			Path:    file.path,
			Name:    comp.Manifest.Name,
			Version: comp.Manifest.Version,
		})
	}
	return results, nil
}

func ParseTargets(args []string) (rewrite bool, targets []string, err error) {
	if len(args) == 0 {
		return false, nil, ErrManifestPathRequired
	}
	rewrite = false
	targets = make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg == "--rewrite=v2":
			rewrite = true
		case strings.HasPrefix(arg, "--"):
			return false, nil, fmt.Errorf("unknown flag %q", arg)
		default:
			targets = append(targets, arg)
		}
	}
	if len(targets) == 0 {
		return false, nil, ErrManifestPathRequired
	}
	return rewrite, targets, nil
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
