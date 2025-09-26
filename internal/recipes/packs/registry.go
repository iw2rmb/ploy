package packs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var (
	ErrPackListNotFound = errors.New("pack list not found")
	ErrInvalidSpec      = errors.New("invalid pack list spec")
)

type Pack struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Optional bool   `toml:"optional"`
}

type Spec struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Languages   []string `toml:"languages"`
	Default     bool     `toml:"default"`
	Packs       []Pack   `toml:"packs"`
}

type Registry struct {
	lists       map[string]Spec
	defaultName string
}

func LoadDirectory(dir string) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read pack directory: %w", err)
	}

	lists := make(map[string]Spec)
	var defaultName string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read pack file %s: %w", entry.Name(), err)
		}
		var spec Spec
		if err := toml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("decode pack list %s: %w", entry.Name(), err)
		}
		spec = normaliseSpec(spec)
		if err := validateSpec(spec); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
		}
		if _, exists := lists[spec.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate pack list %q", ErrInvalidSpec, spec.Name)
		}
		if spec.Default {
			if defaultName != "" && defaultName != spec.Name {
				return nil, fmt.Errorf("%w: multiple default pack lists (%s, %s)", ErrInvalidSpec, defaultName, spec.Name)
			}
			defaultName = spec.Name
		}
		lists[spec.Name] = spec
	}

	return &Registry{
		lists:       lists,
		defaultName: defaultName,
	}, nil
}

func (r *Registry) List() []Spec {
	if r == nil {
		return nil
	}
	result := make([]Spec, 0, len(r.lists))
	for _, spec := range r.lists {
		result = append(result, spec)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (r *Registry) Get(name string) (Spec, bool) {
	if r == nil {
		return Spec{}, false
	}
	spec, ok := r.lists[strings.TrimSpace(name)]
	return spec, ok
}

func (r *Registry) FindByLanguage(language string) []Spec {
	if r == nil {
		return nil
	}
	trimmed := strings.ToLower(strings.TrimSpace(language))
	if trimmed == "" {
		return nil
	}
	var result []Spec
	for _, spec := range r.lists {
		if contains(spec.Languages, trimmed) {
			result = append(result, spec)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (r *Registry) Default() (Spec, error) {
	if r == nil || r.defaultName == "" {
		return Spec{}, ErrPackListNotFound
	}
	spec, ok := r.lists[r.defaultName]
	if !ok {
		return Spec{}, ErrPackListNotFound
	}
	return spec, nil
}

func validateSpec(spec Spec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("name is required")
	}
	if len(spec.Languages) == 0 {
		return errors.New("at least one language is required")
	}
	if len(spec.Packs) == 0 {
		return errors.New("at least one pack is required")
	}
	for i, pack := range spec.Packs {
		if strings.TrimSpace(pack.ID) == "" {
			return fmt.Errorf("packs[%d].id is required", i)
		}
		if strings.TrimSpace(pack.Version) == "" {
			return fmt.Errorf("packs[%d].version is required", i)
		}
	}
	return nil
}

func normaliseSpec(spec Spec) Spec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Description = strings.TrimSpace(spec.Description)

	langSet := make(map[string]struct{})
	langs := make([]string, 0, len(spec.Languages))
	for _, lang := range spec.Languages {
		trimmed := strings.ToLower(strings.TrimSpace(lang))
		if trimmed == "" {
			continue
		}
		if _, exists := langSet[trimmed]; exists {
			continue
		}
		langSet[trimmed] = struct{}{}
		langs = append(langs, trimmed)
	}
	sort.Strings(langs)
	spec.Languages = langs

	packs := make([]Pack, 0, len(spec.Packs))
	for _, pack := range spec.Packs {
		packs = append(packs, Pack{
			ID:       strings.TrimSpace(pack.ID),
			Version:  strings.TrimSpace(pack.Version),
			Optional: pack.Optional,
		})
	}
	spec.Packs = packs
	return spec
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
