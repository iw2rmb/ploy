package manifests

import (
	"errors"
	"fmt"
	"strings"

	dtypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type rawManifest struct {
	ManifestVersion string        `toml:"manifest_version"`
	Name            string        `toml:"name"`
	Version         string        `toml:"version"`
	Summary         string        `toml:"summary"`
	Topology        rawTopology   `toml:"topology"`
	Fixtures        rawFixtures   `toml:"fixtures"`
	Lanes           rawLanes      `toml:"lanes"`
	Services        []rawService  `toml:"services"`
	Edges           []rawEdge     `toml:"edges"`
	Exposures       []rawExposure `toml:"exposures,omitempty"`
}

type rawTopology struct {
	Description string    `toml:"description"`
	Allow       []rawFlow `toml:"allow,omitempty"`
	Deny        []rawFlow `toml:"deny,omitempty"`
}

type rawFlow struct {
	From   string `toml:"from"`
	To     string `toml:"to"`
	Reason string `toml:"reason,omitempty"`
}

type rawFixtures struct {
	Required []rawFixture `toml:"required"`
	Optional []rawFixture `toml:"optional,omitempty"`
}

type rawFixture struct {
	Name      string `toml:"name"`
	Reference string `toml:"reference"`
	Reason    string `toml:"reason,omitempty"`
}

type rawLanes struct {
	Required []rawLane `toml:"required"`
	Allowed  []rawLane `toml:"allowed,omitempty"`
}

type rawLane struct {
	Name   string `toml:"name"`
	Reason string `toml:"reason,omitempty"`
}

type rawService struct {
	Name     string                  `toml:"name"`
	Kind     string                  `toml:"kind"`
	Optional bool                    `toml:"optional,omitempty"`
	Identity rawServiceIdentity      `toml:"identity"`
	Ports    []rawServicePort        `toml:"ports"`
	Requires []rawServiceRequirement `toml:"requires,omitempty"`
}

type rawServiceIdentity struct {
	DNS string `toml:"dns"`
}

type rawServicePort struct {
	Name     string `toml:"name"`
	Port     int    `toml:"port"`
	Protocol string `toml:"protocol"`
}

type rawServiceRequirement struct {
	Target string `toml:"target"`
	Edge   string `toml:"edge"`
}

type rawEdge struct {
	Source    string   `toml:"source"`
	Target    string   `toml:"target"`
	Ports     []string `toml:"ports"`
	Protocols []string `toml:"protocols"`
}

type rawExposure struct {
	Service string `toml:"service"`
	Port    string `toml:"port"`
	Mode    string `toml:"mode"`
}

// validateRawManifest ensures the manifest captures the required topology contracts.
func validateRawManifest(m rawManifest) error {
	manifestVersion := strings.TrimSpace(m.ManifestVersion)
	if manifestVersion == "" {
		return errors.New("manifest_version must be set to v2")
	}
	if manifestVersion != "v2" {
		return fmt.Errorf("manifest_version must be v2 (got %s)", manifestVersion)
	}

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

	if len(m.Services) == 0 {
		return errors.New("services requires at least one entry")
	}
	servicePorts := make(map[string]map[string]struct{}, len(m.Services))
	for i, service := range m.Services {
		serviceName := strings.TrimSpace(service.Name)
		if serviceName == "" {
			return fmt.Errorf("services[%d].name is required", i)
		}
		if _, exists := servicePorts[serviceName]; exists {
			return fmt.Errorf("services[%d].name duplicates an existing service", i)
		}
		if strings.TrimSpace(service.Kind) == "" {
			return fmt.Errorf("services[%d].kind is required", i)
		}
		if strings.TrimSpace(service.Identity.DNS) == "" {
			return fmt.Errorf("services[%d].identity.dns is required", i)
		}
		if len(service.Ports) == 0 {
			return fmt.Errorf("services[%d].ports requires at least one entry", i)
		}
		ports := make(map[string]struct{}, len(service.Ports))
		for j, port := range service.Ports {
			name := strings.TrimSpace(port.Name)
			if name == "" {
				return fmt.Errorf("services[%d].ports[%d].name is required", i, j)
			}
			if _, exists := ports[name]; exists {
				return fmt.Errorf("services[%d].ports[%d].name duplicates port %q", i, j, name)
			}
			if port.Port <= 0 {
				return fmt.Errorf("services[%d].ports[%d].port must be positive", i, j)
			}
			proto := strings.ToLower(strings.TrimSpace(port.Protocol))
			if proto == "" {
				return fmt.Errorf("services[%d].ports[%d].protocol is required", i, j)
			}
			if err := dtypes.Protocol(proto).Validate(); err != nil {
				return fmt.Errorf("services[%d].ports[%d].protocol %q is not supported", i, j, proto)
			}
			ports[name] = struct{}{}
		}
		servicePorts[serviceName] = ports
		for j, req := range service.Requires {
			if strings.TrimSpace(req.Target) == "" {
				return fmt.Errorf("services[%d].requires[%d].target is required", i, j)
			}
			if strings.TrimSpace(req.Edge) == "" {
				return fmt.Errorf("services[%d].requires[%d].edge is required", i, j)
			}
		}
	}

	if len(m.Edges) == 0 {
		return errors.New("edges requires at least one entry")
	}
	for i, edge := range m.Edges {
		source := strings.TrimSpace(edge.Source)
		if source == "" {
			return fmt.Errorf("edges[%d].source is required", i)
		}
		if _, ok := servicePorts[source]; !ok {
			return fmt.Errorf("edges[%d].source %q missing from services", i, source)
		}
		target := strings.TrimSpace(edge.Target)
		if target == "" {
			return fmt.Errorf("edges[%d].target is required", i)
		}
		ports, ok := servicePorts[target]
		if !ok {
			return fmt.Errorf("edges[%d].target %q missing from services", i, target)
		}
		if len(edge.Ports) == 0 {
			return fmt.Errorf("edges[%d].ports requires at least one entry", i)
		}
		for j, port := range edge.Ports {
			trimmed := strings.TrimSpace(port)
			if trimmed == "" {
				return fmt.Errorf("edges[%d].ports[%d] is required", i, j)
			}
			if _, exists := ports[trimmed]; !exists {
				return fmt.Errorf("edges[%d].ports[%d] references unknown target port %q", i, j, trimmed)
			}
		}
		if len(edge.Protocols) == 0 {
			return fmt.Errorf("edges[%d].protocols requires at least one entry", i)
		}
		for j, protocol := range edge.Protocols {
			proto := strings.ToLower(strings.TrimSpace(protocol))
			if proto == "" {
				return fmt.Errorf("edges[%d].protocols[%d] is required", i, j)
			}
			if err := dtypes.Protocol(proto).Validate(); err != nil {
				return fmt.Errorf("edges[%d].protocols[%d] %q is not supported", i, j, proto)
			}
		}
	}

	allowedExposureModes := map[string]struct{}{
		"public":  {},
		"cluster": {},
		"local":   {},
	}
	for i, exposure := range m.Exposures {
		service := strings.TrimSpace(exposure.Service)
		if service == "" {
			return fmt.Errorf("exposures[%d].service is required", i)
		}
		ports, ok := servicePorts[service]
		if !ok {
			return fmt.Errorf("exposures[%d].service %q missing from services", i, service)
		}
		port := strings.TrimSpace(exposure.Port)
		if port == "" {
			return fmt.Errorf("exposures[%d].port is required", i)
		}
		if _, exists := ports[port]; !exists {
			return fmt.Errorf("exposures[%d].port %q missing from service %q", i, port, service)
		}
		mode := strings.TrimSpace(exposure.Mode)
		if mode == "" {
			return fmt.Errorf("exposures[%d].mode is required", i)
		}
		if _, allowed := allowedExposureModes[mode]; !allowed {
			return fmt.Errorf("exposures[%d].mode %q is not supported", i, mode)
		}
	}

	return nil
}
