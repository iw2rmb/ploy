package manifests

import (
	"errors"
	"fmt"
	"strings"
)

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
