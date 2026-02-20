package manifests

import (
	"encoding/json"
	"sort"
	"strings"

	dtypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Compilation represents the normalized manifest payload consumed by the runner/control plane.
type Compilation struct {
	Manifest        Metadata   `json:"manifest"`
	ManifestVersion string     `json:"manifest_version"`
	Topology        Topology   `json:"topology"`
	Services        []Service  `json:"services"`
	Edges           []Edge     `json:"edges"`
	Exposures       []Exposure `json:"exposures"`
	Fixtures        FixtureSet `json:"fixtures"`
	Lanes           LaneSet    `json:"lanes"`
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

// Service describes a workload participating in the manifest topology.
type Service struct {
	Name     string               `json:"name"`
	Kind     string               `json:"kind"`
	Optional bool                 `json:"optional,omitempty"`
	Identity ServiceIdentity      `json:"identity"`
	Ports    []ServicePort        `json:"ports"`
	Requires []ServiceRequirement `json:"requires,omitempty"`
}

// ServiceIdentity contains per-service addressing metadata.
type ServiceIdentity struct {
	DNS string `json:"dns"`
}

// ServicePort captures an exposed network port for a service.
type ServicePort struct {
	Name     string          `json:"name"`
	Port     int             `json:"port"`
	Protocol dtypes.Protocol `json:"protocol"`
}

// ServiceRequirement documents dependencies required by the service.
type ServiceRequirement struct {
	Target string `json:"target"`
	Edge   string `json:"edge"`
}

// Edge codifies connectivity expectations between services.
type Edge struct {
	Source    string            `json:"source"`
	Target    string            `json:"target"`
	Ports     []string          `json:"ports"`
	Protocols []dtypes.Protocol `json:"protocols"`
}

// Exposure captures public or internal exposure intents for a service port.
type Exposure struct {
	Service string `json:"service"`
	Port    string `json:"port"`
	Mode    string `json:"mode"`
}

// compile constructs the normalized compilation payload for the manifest.
func compile(raw rawManifest) Compilation {
	manifestVersion := strings.TrimSpace(raw.ManifestVersion)
	if manifestVersion == "" {
		manifestVersion = "v2"
	}

	return Compilation{
		Manifest: Metadata{
			Name:    strings.TrimSpace(raw.Name),
			Version: strings.TrimSpace(raw.Version),
			Summary: strings.TrimSpace(raw.Summary),
		},
		ManifestVersion: manifestVersion,
		Topology: Topology{
			Description: strings.TrimSpace(raw.Topology.Description),
			Allow:       convertFlows(raw.Topology.Allow),
			Deny:        convertFlows(raw.Topology.Deny),
		},
		Services:  normalizeServices(raw.Services),
		Edges:     normalizeEdges(raw.Edges),
		Exposures: normalizeExposures(raw.Exposures),
		Fixtures: FixtureSet{
			Required: convertFixtures(raw.Fixtures.Required),
			Optional: convertFixtures(raw.Fixtures.Optional),
		},
		Lanes: LaneSet{
			Required: convertLanes(raw.Lanes.Required),
			Allowed:  convertLanes(raw.Lanes.Allowed),
		},
	}
}

func convertFlows(raw []rawFlow) []Flow {
	result := make([]Flow, len(raw))
	for i, f := range raw {
		result[i] = Flow{From: strings.TrimSpace(f.From), To: strings.TrimSpace(f.To), Reason: strings.TrimSpace(f.Reason)}
	}
	return result
}

func convertFixtures(raw []rawFixture) []Fixture {
	result := make([]Fixture, len(raw))
	for i, f := range raw {
		result[i] = Fixture{Name: strings.TrimSpace(f.Name), Reference: strings.TrimSpace(f.Reference), Reason: strings.TrimSpace(f.Reason)}
	}
	return result
}

func convertLanes(raw []rawLane) []Lane {
	result := make([]Lane, len(raw))
	for i, l := range raw {
		result[i] = Lane{Name: strings.TrimSpace(l.Name), Reason: strings.TrimSpace(l.Reason)}
	}
	return result
}

// JSON marshals the compilation payload for downstream consumers.
func (c Compilation) JSON() ([]byte, error) {
	return json.Marshal(c)
}

// normalizeServices trims, sorts, and deduplicates service metadata.
func normalizeServices(rawServices []rawService) []Service {
	if len(rawServices) == 0 {
		return nil
	}
	services := make([]Service, 0, len(rawServices))
	for _, svc := range rawServices {
		ports := make([]ServicePort, 0, len(svc.Ports))
		for _, port := range svc.Ports {
			// normalize protocol string to canonical lowercase enum
			proto := dtypes.Protocol(strings.ToLower(strings.TrimSpace(port.Protocol)))
			ports = append(ports, ServicePort{
				Name:     strings.TrimSpace(port.Name),
				Port:     port.Port,
				Protocol: proto,
			})
		}
		sort.Slice(ports, func(i, j int) bool {
			return ports[i].Name < ports[j].Name
		})
		requires := make([]ServiceRequirement, 0, len(svc.Requires))
		for _, req := range svc.Requires {
			requires = append(requires, ServiceRequirement{
				Target: strings.TrimSpace(req.Target),
				Edge:   strings.TrimSpace(req.Edge),
			})
		}
		if len(requires) > 0 {
			sort.Slice(requires, func(i, j int) bool {
				return requires[i].Target < requires[j].Target
			})
		} else {
			requires = nil
		}
		services = append(services, Service{
			Name:     strings.TrimSpace(svc.Name),
			Kind:     strings.TrimSpace(svc.Kind),
			Optional: svc.Optional,
			Identity: ServiceIdentity{DNS: strings.TrimSpace(svc.Identity.DNS)},
			Ports:    ports,
			Requires: requires,
		})
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services
}

// normalizeEdges sorts edges and their fields for deterministic payloads.
func normalizeEdges(rawEdges []rawEdge) []Edge {
	if len(rawEdges) == 0 {
		return nil
	}
	edges := make([]Edge, 0, len(rawEdges))
	for _, edge := range rawEdges {
		// normalize protocols into canonical enum slice
		protos := normalizeProtocols(edge.Protocols)
		edges = append(edges, Edge{
			Source:    strings.TrimSpace(edge.Source),
			Target:    strings.TrimSpace(edge.Target),
			Ports:     normalizeStringSet(edge.Ports),
			Protocols: protos,
		})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source == edges[j].Source {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Source < edges[j].Source
	})
	return edges
}

// normalizeExposures orders exposures to keep manifest parity stable.
func normalizeExposures(rawExposures []rawExposure) []Exposure {
	if len(rawExposures) == 0 {
		return nil
	}
	exposures := make([]Exposure, 0, len(rawExposures))
	for _, exposure := range rawExposures {
		exposures = append(exposures, Exposure{
			Service: strings.TrimSpace(exposure.Service),
			Port:    strings.TrimSpace(exposure.Port),
			Mode:    strings.TrimSpace(exposure.Mode),
		})
	}
	sort.Slice(exposures, func(i, j int) bool {
		if exposures[i].Service == exposures[j].Service {
			if exposures[i].Mode == exposures[j].Mode {
				return exposures[i].Port < exposures[j].Port
			}
			return exposures[i].Mode < exposures[j].Mode
		}
		return exposures[i].Service < exposures[j].Service
	})
	return exposures
}

// normalizeStringSet trims, deduplicates, and sorts a slice of strings.
func normalizeStringSet(values []string) []string {
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

// normalizeProtocols trims, deduplicates, sorts, and canonicalizes protocol values.
func normalizeProtocols(values []string) []dtypes.Protocol {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	tmp := make([]string, 0, len(set))
	for value := range set {
		tmp = append(tmp, value)
	}
	sort.Strings(tmp)
	result := make([]dtypes.Protocol, 0, len(tmp))
	for _, s := range tmp {
		result = append(result, dtypes.Protocol(s))
	}
	return result
}
