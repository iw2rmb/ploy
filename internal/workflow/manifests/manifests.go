package manifests

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var (
	errManifestNotFound    = errors.New("manifest not found")
	errInvalidManifest     = errors.New("invalid manifest configuration")
	errRegistryUnavailable = errors.New("manifest registry unavailable")
)

type CompileOptions struct {
	Name    string
	Version string
}

type Registry struct {
	entries map[string]rawEntry
}

type rawEntry struct {
	manifest rawManifest
	path     string
}

type rawManifest struct {
	Name     string      `toml:"name"`
	Version  string      `toml:"version"`
	Summary  string      `toml:"summary"`
	Topology rawTopology `toml:"topology"`
	Fixtures rawFixtures `toml:"fixtures"`
	Lanes    rawLanes    `toml:"lanes"`
	Aster    rawAster    `toml:"aster"`
}

type rawTopology struct {
	Description string    `toml:"description"`
	Allow       []rawFlow `toml:"allow"`
	Deny        []rawFlow `toml:"deny"`
}

type rawFlow struct {
	From   string `toml:"from"`
	To     string `toml:"to"`
	Reason string `toml:"reason"`
}

type rawFixtures struct {
	Required []rawFixture `toml:"required"`
	Optional []rawFixture `toml:"optional"`
}

type rawFixture struct {
	Name      string `toml:"name"`
	Reference string `toml:"reference"`
	Reason    string `toml:"reason"`
}

type rawLanes struct {
	Required []rawLane `toml:"required"`
	Allowed  []rawLane `toml:"allowed"`
}

type rawLane struct {
	Name   string `toml:"name"`
	Reason string `toml:"reason"`
}

type rawAster struct {
	Required []string `toml:"required"`
	Optional []string `toml:"optional"`
}

type Compilation struct {
	Manifest Metadata   `json:"manifest"`
	Topology Topology   `json:"topology"`
	Fixtures FixtureSet `json:"fixtures"`
	Lanes    LaneSet    `json:"lanes"`
	Aster    AsterSet   `json:"aster"`
}

type Metadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Summary string `json:"summary_markdown"`
}

type Topology struct {
	Description string `json:"description"`
	Allow       []Flow `json:"allow"`
	Deny        []Flow `json:"deny"`
}

type Flow struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

type FixtureSet struct {
	Required []Fixture `json:"required"`
	Optional []Fixture `json:"optional"`
}

type Fixture struct {
	Name      string `json:"name"`
	Reference string `json:"reference"`
	Reason    string `json:"reason,omitempty"`
}

type LaneSet struct {
	Required []Lane `json:"required"`
	Allowed  []Lane `json:"allowed"`
}

type Lane struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

type AsterSet struct {
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

func LoadDirectory(dir string) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read manifest directory: %w", err)
	}

	registry := &Registry{entries: make(map[string]rawEntry)}

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
			return nil, fmt.Errorf("read manifest %s: %w", entry.Name(), err)
		}

		var manifest rawManifest
		if err := toml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("decode manifest %s: %w", entry.Name(), err)
		}

		if err := validateRawManifest(manifest); err != nil {
			return nil, fmt.Errorf("%w (%s): %v", errInvalidManifest, entry.Name(), err)
		}

		key := strings.TrimSpace(manifest.Name)
		if _, exists := registry.entries[key]; exists {
			return nil, fmt.Errorf("%w: duplicate manifest %q", errInvalidManifest, key)
		}
		registry.entries[key] = rawEntry{manifest: manifest, path: path}
	}

	if len(registry.entries) == 0 {
		return nil, fmt.Errorf("%w: no manifest definitions found in %s", errRegistryUnavailable, dir)
	}

	return registry, nil
}

func validateRawManifest(m rawManifest) error {
	name := strings.TrimSpace(m.Name)
	if name == "" {
		return errors.New("name is required")
	}
	version := strings.TrimSpace(m.Version)
	if version == "" {
		return errors.New("version is required")
	}
	if strings.TrimSpace(m.Summary) == "" {
		return errors.New("summary is required")
	}

	if len(m.Topology.Allow) == 0 {
		return errors.New("topology.allow requires at least one flow")
	}
	for i, flow := range m.Topology.Allow {
		if strings.TrimSpace(flow.From) == "" {
			return fmt.Errorf("topology.allow[%d].from is required", i)
		}
		if strings.TrimSpace(flow.To) == "" {
			return fmt.Errorf("topology.allow[%d].to is required", i)
		}
	}
	for i, flow := range m.Topology.Deny {
		if strings.TrimSpace(flow.From) == "" {
			return fmt.Errorf("topology.deny[%d].from is required", i)
		}
		if strings.TrimSpace(flow.To) == "" {
			return fmt.Errorf("topology.deny[%d].to is required", i)
		}
		if strings.TrimSpace(flow.Reason) == "" {
			return fmt.Errorf("topology.deny[%d].reason is required", i)
		}
	}

	if len(m.Fixtures.Required) == 0 {
		return errors.New("fixtures.required requires at least one entry")
	}
	for i, fixture := range m.Fixtures.Required {
		if strings.TrimSpace(fixture.Name) == "" {
			return fmt.Errorf("fixtures.required[%d].name is required", i)
		}
		if strings.TrimSpace(fixture.Reference) == "" {
			return fmt.Errorf("fixtures.required[%d].reference is required", i)
		}
	}
	for i, fixture := range m.Fixtures.Optional {
		if strings.TrimSpace(fixture.Name) == "" {
			return fmt.Errorf("fixtures.optional[%d].name is required", i)
		}
		if strings.TrimSpace(fixture.Reference) == "" {
			return fmt.Errorf("fixtures.optional[%d].reference is required", i)
		}
	}

	if len(m.Lanes.Required) == 0 {
		return errors.New("lanes.required requires at least one entry")
	}
	for i, lane := range m.Lanes.Required {
		if strings.TrimSpace(lane.Name) == "" {
			return fmt.Errorf("lanes.required[%d].name is required", i)
		}
		if strings.TrimSpace(lane.Reason) == "" {
			return fmt.Errorf("lanes.required[%d].reason is required", i)
		}
	}
	for i, lane := range m.Lanes.Allowed {
		if strings.TrimSpace(lane.Name) == "" {
			return fmt.Errorf("lanes.allowed[%d].name is required", i)
		}
	}

	return nil
}

func (r *Registry) Compile(opts CompileOptions) (Compilation, error) {
	if r == nil {
		return Compilation{}, fmt.Errorf("%w: registry missing", errRegistryUnavailable)
	}
	key := strings.TrimSpace(opts.Name)
	if key == "" {
		return Compilation{}, fmt.Errorf("%w: name is required", errManifestNotFound)
	}

	entry, ok := r.entries[key]
	if !ok {
		return Compilation{}, fmt.Errorf("%w: %s", errManifestNotFound, key)
	}

	requestedVersion := strings.TrimSpace(opts.Version)
	actualVersion := strings.TrimSpace(entry.manifest.Version)
	if requestedVersion != "" && requestedVersion != actualVersion {
		return Compilation{}, fmt.Errorf("version mismatch: requested %s, manifest %s", requestedVersion, actualVersion)
	}

	return compile(entry.manifest), nil
}

func compile(raw rawManifest) Compilation {
	summary := strings.TrimSpace(raw.Summary)
	topology := Topology{
		Description: strings.TrimSpace(raw.Topology.Description),
		Allow:       make([]Flow, len(raw.Topology.Allow)),
		Deny:        make([]Flow, len(raw.Topology.Deny)),
	}
	for i, flow := range raw.Topology.Allow {
		topology.Allow[i] = Flow{
			From: strings.TrimSpace(flow.From),
			To:   strings.TrimSpace(flow.To),
		}
	}
	for i, flow := range raw.Topology.Deny {
		topology.Deny[i] = Flow{
			From:   strings.TrimSpace(flow.From),
			To:     strings.TrimSpace(flow.To),
			Reason: strings.TrimSpace(flow.Reason),
		}
	}

	fixtures := FixtureSet{
		Required: make([]Fixture, len(raw.Fixtures.Required)),
		Optional: make([]Fixture, len(raw.Fixtures.Optional)),
	}
	for i, fx := range raw.Fixtures.Required {
		fixtures.Required[i] = Fixture{
			Name:      strings.TrimSpace(fx.Name),
			Reference: strings.TrimSpace(fx.Reference),
			Reason:    strings.TrimSpace(fx.Reason),
		}
	}
	for i, fx := range raw.Fixtures.Optional {
		fixtures.Optional[i] = Fixture{
			Name:      strings.TrimSpace(fx.Name),
			Reference: strings.TrimSpace(fx.Reference),
			Reason:    strings.TrimSpace(fx.Reason),
		}
	}

	lanes := LaneSet{
		Required: make([]Lane, len(raw.Lanes.Required)),
		Allowed:  make([]Lane, len(raw.Lanes.Allowed)),
	}
	for i, lane := range raw.Lanes.Required {
		lanes.Required[i] = Lane{
			Name:   strings.TrimSpace(lane.Name),
			Reason: strings.TrimSpace(lane.Reason),
		}
	}
	for i, lane := range raw.Lanes.Allowed {
		lanes.Allowed[i] = Lane{
			Name:   strings.TrimSpace(lane.Name),
			Reason: strings.TrimSpace(lane.Reason),
		}
	}

	aster := AsterSet{
		Required: normalizeToggles(raw.Aster.Required),
		Optional: normalizeToggles(raw.Aster.Optional),
	}

	return Compilation{
		Manifest: Metadata{
			Name:    strings.TrimSpace(raw.Name),
			Version: strings.TrimSpace(raw.Version),
			Summary: summary,
		},
		Topology: topology,
		Fixtures: fixtures,
		Lanes:    lanes,
		Aster:    aster,
	}
}

func normalizeToggles(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (c Compilation) JSON() ([]byte, error) {
	return json.Marshal(c)
}
