package manifests

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// LoadFile parses, validates, and compiles a single manifest file.
func LoadFile(path string) (Compilation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Compilation{}, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var manifest rawManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return Compilation{}, fmt.Errorf("decode manifest %s: %w", path, err)
	}

	if err := validateRawManifest(manifest); err != nil {
		return Compilation{}, fmt.Errorf("%w (%s): %v", errInvalidManifest, path, err)
	}

	return compile(manifest), nil
}

// EncodeCompilationToTOML renders the compilation back into the canonical v2 manifest TOML.
func EncodeCompilationToTOML(comp Compilation) ([]byte, error) {
	manifest := tomlManifest{
		ManifestVersion: comp.ManifestVersion,
		Name:            comp.Manifest.Name,
		Version:         comp.Manifest.Version,
		Summary:         comp.Manifest.Summary,
		Topology: tomlTopology{
			Description: comp.Topology.Description,
		},
		Fixtures: tomlFixtures{
			Required: toTomlFixtures(comp.Fixtures.Required),
			Optional: toTomlFixtures(comp.Fixtures.Optional),
		},
		Lanes: tomlLanes{
			Required: toTomlLanes(comp.Lanes.Required),
			Allowed:  toTomlLanes(comp.Lanes.Allowed),
		},
		Aster: tomlAster{
			Required: comp.Aster.Required,
			Optional: comp.Aster.Optional,
		},
		Services:  toTomlServices(comp.Services),
		Edges:     toTomlEdges(comp.Edges),
		Exposures: toTomlExposures(comp.Exposures),
	}

	if len(comp.Topology.Allow) > 0 {
		manifest.Topology.Allow = toTomlFlows(comp.Topology.Allow)
	}
	if len(comp.Topology.Deny) > 0 {
		manifest.Topology.Deny = toTomlFlows(comp.Topology.Deny)
	}

	return toml.Marshal(manifest)
}

type tomlManifest struct {
	ManifestVersion string         `toml:"manifest_version"`
	Name            string         `toml:"name"`
	Version         string         `toml:"version"`
	Summary         string         `toml:"summary"`
	Topology        tomlTopology   `toml:"topology"`
	Fixtures        tomlFixtures   `toml:"fixtures"`
	Lanes           tomlLanes      `toml:"lanes"`
	Aster           tomlAster      `toml:"aster"`
	Services        []tomlService  `toml:"services"`
	Edges           []tomlEdge     `toml:"edges"`
	Exposures       []tomlExposure `toml:"exposures,omitempty"`
}

type tomlTopology struct {
	Description string     `toml:"description"`
	Allow       []tomlFlow `toml:"allow,omitempty"`
	Deny        []tomlFlow `toml:"deny,omitempty"`
}

type tomlFlow struct {
	From   string `toml:"from"`
	To     string `toml:"to"`
	Reason string `toml:"reason,omitempty"`
}

type tomlFixtures struct {
	Required []tomlFixture `toml:"required"`
	Optional []tomlFixture `toml:"optional,omitempty"`
}

type tomlFixture struct {
	Name      string `toml:"name"`
	Reference string `toml:"reference"`
	Reason    string `toml:"reason,omitempty"`
}

type tomlLanes struct {
	Required []tomlLane `toml:"required"`
	Allowed  []tomlLane `toml:"allowed,omitempty"`
}

type tomlLane struct {
	Name   string `toml:"name"`
	Reason string `toml:"reason,omitempty"`
}

type tomlAster struct {
	Required []string `toml:"required,omitempty"`
	Optional []string `toml:"optional,omitempty"`
}

type tomlService struct {
	Name     string               `toml:"name"`
	Kind     string               `toml:"kind"`
	Optional bool                 `toml:"optional,omitempty"`
	Identity tomlServiceIdentity  `toml:"identity"`
	Ports    []tomlServicePort    `toml:"ports"`
	Requires []tomlServiceRequire `toml:"requires,omitempty"`
}

type tomlServiceIdentity struct {
	DNS string `toml:"dns"`
}

type tomlServicePort struct {
	Name     string `toml:"name"`
	Port     int    `toml:"port"`
	Protocol string `toml:"protocol"`
}

type tomlServiceRequire struct {
	Target string `toml:"target"`
	Edge   string `toml:"edge"`
}

type tomlEdge struct {
	Source    string   `toml:"source"`
	Target    string   `toml:"target"`
	Ports     []string `toml:"ports"`
	Protocols []string `toml:"protocols"`
}

type tomlExposure struct {
	Service string `toml:"service"`
	Port    string `toml:"port"`
	Mode    string `toml:"mode"`
}

func toTomlFlows(flows []Flow) []tomlFlow {
	if len(flows) == 0 {
		return nil
	}
	result := make([]tomlFlow, len(flows))
	for i, flow := range flows {
		result[i] = tomlFlow(flow)
	}
	return result
}

func toTomlFixtures(fixtures []Fixture) []tomlFixture {
	if len(fixtures) == 0 {
		return nil
	}
	result := make([]tomlFixture, len(fixtures))
	for i, fixture := range fixtures {
		result[i] = tomlFixture(fixture)
	}
	return result
}

func toTomlLanes(lanes []Lane) []tomlLane {
	if len(lanes) == 0 {
		return nil
	}
	result := make([]tomlLane, len(lanes))
	for i, lane := range lanes {
		result[i] = tomlLane(lane)
	}
	return result
}

func toTomlServices(services []Service) []tomlService {
	if len(services) == 0 {
		return nil
	}
	result := make([]tomlService, len(services))
	for i, svc := range services {
		result[i] = tomlService{
			Name:     svc.Name,
			Kind:     svc.Kind,
			Optional: svc.Optional,
			Identity: tomlServiceIdentity{DNS: svc.Identity.DNS},
			Ports:    toTomlServicePorts(svc.Ports),
			Requires: toTomlServiceRequires(svc.Requires),
		}
	}
	return result
}

func toTomlServicePorts(ports []ServicePort) []tomlServicePort {
	if len(ports) == 0 {
		return nil
	}
	result := make([]tomlServicePort, len(ports))
	for i, port := range ports {
		result[i] = tomlServicePort(port)
	}
	return result
}

func toTomlServiceRequires(requires []ServiceRequirement) []tomlServiceRequire {
	if len(requires) == 0 {
		return nil
	}
	result := make([]tomlServiceRequire, len(requires))
	for i, req := range requires {
		result[i] = tomlServiceRequire(req)
	}
	return result
}

func toTomlEdges(edges []Edge) []tomlEdge {
	if len(edges) == 0 {
		return nil
	}
	result := make([]tomlEdge, len(edges))
	for i, edge := range edges {
		result[i] = tomlEdge(edge)
	}
	return result
}

func toTomlExposures(exposures []Exposure) []tomlExposure {
	if len(exposures) == 0 {
		return nil
	}
	result := make([]tomlExposure, len(exposures))
	for i, exposure := range exposures {
		result[i] = tomlExposure(exposure)
	}
	return result
}
