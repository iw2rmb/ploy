package manifests

import (
	"encoding/json"
	"sort"
	"strings"
)

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
