package manifests

import (
	"fmt"
	"os"

	dtypes "github.com/iw2rmb/ploy/internal/domain/types"
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
	manifest := rawManifest{
		ManifestVersion: comp.ManifestVersion,
		Name:            comp.Manifest.Name,
		Version:         comp.Manifest.Version,
		Summary:         comp.Manifest.Summary,
		Topology: rawTopology{
			Description: comp.Topology.Description,
		},
		Fixtures: rawFixtures{
			Required: compilationFixturesToRaw(comp.Fixtures.Required),
			Optional: compilationFixturesToRaw(comp.Fixtures.Optional),
		},
		Lanes: rawLanes{
			Required: compilationLanesToRaw(comp.Lanes.Required),
			Allowed:  compilationLanesToRaw(comp.Lanes.Allowed),
		},
		Services:  compilationServicesToRaw(comp.Services),
		Edges:     compilationEdgesToRaw(comp.Edges),
		Exposures: compilationExposuresToRaw(comp.Exposures),
	}

	if len(comp.Topology.Allow) > 0 {
		manifest.Topology.Allow = compilationFlowsToRaw(comp.Topology.Allow)
	}
	if len(comp.Topology.Deny) > 0 {
		manifest.Topology.Deny = compilationFlowsToRaw(comp.Topology.Deny)
	}

	return toml.Marshal(manifest)
}

func compilationFlowsToRaw(flows []Flow) []rawFlow {
	if len(flows) == 0 {
		return nil
	}
	result := make([]rawFlow, len(flows))
	for i, flow := range flows {
		result[i] = rawFlow(flow)
	}
	return result
}

func compilationFixturesToRaw(fixtures []Fixture) []rawFixture {
	if len(fixtures) == 0 {
		return nil
	}
	result := make([]rawFixture, len(fixtures))
	for i, fixture := range fixtures {
		result[i] = rawFixture(fixture)
	}
	return result
}

func compilationLanesToRaw(lanes []Lane) []rawLane {
	if len(lanes) == 0 {
		return nil
	}
	result := make([]rawLane, len(lanes))
	for i, lane := range lanes {
		result[i] = rawLane(lane)
	}
	return result
}

func compilationServicesToRaw(services []Service) []rawService {
	if len(services) == 0 {
		return nil
	}
	result := make([]rawService, len(services))
	for i, svc := range services {
		result[i] = rawService{
			Name:     svc.Name,
			Kind:     svc.Kind,
			Optional: svc.Optional,
			Identity: rawServiceIdentity{DNS: svc.Identity.DNS},
			Ports:    compilationServicePortsToRaw(svc.Ports),
			Requires: compilationServiceRequiresToRaw(svc.Requires),
		}
	}
	return result
}

func compilationServicePortsToRaw(ports []ServicePort) []rawServicePort {
	if len(ports) == 0 {
		return nil
	}
	result := make([]rawServicePort, len(ports))
	for i, port := range ports {
		result[i] = rawServicePort{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: port.Protocol.String(),
		}
	}
	return result
}

func compilationServiceRequiresToRaw(requires []ServiceRequirement) []rawServiceRequirement {
	if len(requires) == 0 {
		return nil
	}
	result := make([]rawServiceRequirement, len(requires))
	for i, req := range requires {
		result[i] = rawServiceRequirement(req)
	}
	return result
}

func compilationEdgesToRaw(edges []Edge) []rawEdge {
	if len(edges) == 0 {
		return nil
	}
	result := make([]rawEdge, len(edges))
	for i, edge := range edges {
		result[i] = rawEdge{
			Source:    edge.Source,
			Target:    edge.Target,
			Ports:     edge.Ports,
			Protocols: protocolsToStrings(edge.Protocols),
		}
	}
	return result
}

func compilationExposuresToRaw(exposures []Exposure) []rawExposure {
	if len(exposures) == 0 {
		return nil
	}
	result := make([]rawExposure, len(exposures))
	for i, exposure := range exposures {
		result[i] = rawExposure(exposure)
	}
	return result
}

func protocolsToStrings(in []dtypes.Protocol) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = p.String()
	}
	return out
}
