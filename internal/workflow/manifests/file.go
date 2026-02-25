package manifests

import (
	"fmt"
	"os"

	dtypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/pelletier/go-toml/v2"
)

// LoadFile parses, validates, and compiles a single manifest file.
func LoadFile(path string) (Compilation, error) {
	manifest, err := decodeAndValidateManifest(path, path)
	if err != nil {
		return Compilation{}, err
	}

	return compile(manifest), nil
}

// decodeAndValidateManifest reads, decodes, and validates a manifest using label in error text.
func decodeAndValidateManifest(path string, label string) (rawManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rawManifest{}, fmt.Errorf("read manifest %s: %w", label, err)
	}

	var manifest rawManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return rawManifest{}, fmt.Errorf("decode manifest %s: %w", label, err)
	}

	if err := validateRawManifest(manifest); err != nil {
		return rawManifest{}, fmt.Errorf("%w (%s): %v", errInvalidManifest, label, err)
	}

	return manifest, nil
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
	return mapSlice(flows, func(flow Flow) rawFlow {
		return rawFlow(flow)
	})
}

func compilationFixturesToRaw(fixtures []Fixture) []rawFixture {
	return mapSlice(fixtures, func(fixture Fixture) rawFixture {
		return rawFixture(fixture)
	})
}

func compilationLanesToRaw(lanes []Lane) []rawLane {
	return mapSlice(lanes, func(lane Lane) rawLane {
		return rawLane(lane)
	})
}

func compilationServicesToRaw(services []Service) []rawService {
	return mapSlice(services, func(svc Service) rawService {
		return rawService{
			Name:     svc.Name,
			Kind:     svc.Kind,
			Optional: svc.Optional,
			Identity: rawServiceIdentity{DNS: svc.Identity.DNS},
			Ports:    compilationServicePortsToRaw(svc.Ports),
			Requires: compilationServiceRequiresToRaw(svc.Requires),
		}
	})
}

func compilationServicePortsToRaw(ports []ServicePort) []rawServicePort {
	return mapSlice(ports, func(port ServicePort) rawServicePort {
		return rawServicePort{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: port.Protocol.String(),
		}
	})
}

func compilationServiceRequiresToRaw(requires []ServiceRequirement) []rawServiceRequirement {
	return mapSlice(requires, func(req ServiceRequirement) rawServiceRequirement {
		return rawServiceRequirement(req)
	})
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
	return mapSlice(exposures, func(exposure Exposure) rawExposure {
		return rawExposure(exposure)
	})
}

func protocolsToStrings(in []dtypes.Protocol) []string {
	return mapSlice(in, func(p dtypes.Protocol) string {
		return p.String()
	})
}
