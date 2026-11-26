package stackdetect

import (
	"bytes"
	"os"

	"github.com/Masterminds/semver/v3"
	"github.com/tailscale/hujson"
)

func detectDotnet(s scanResult) (*Observation, error) {
	evidence := s.evidence
	unknown := func(message string) (*Observation, error) {
		return nil, &DetectionError{Reason: "unknown", Message: message, Evidence: evidence}
	}

	// Build Gate starts in the checkout root. SDK settings outside that root
	// describe the host environment and cannot determine the repository's release.
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return unknown("failed to open workspace: " + err.Error())
	}
	defer root.Close()
	data := readDotnetBuildFile(root, "global.json")
	if data == nil {
		return unknown("global.json is missing or unreadable")
	}

	config, err := hujson.Parse(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if err != nil {
		return unknown("invalid global.json: " + err.Error())
	}
	value := config.Find("/sdk/version")
	if value == nil || value.Value.Kind() != '"' {
		return unknown("global.json must declare sdk.version as a string")
	}
	release := value.Value.(hujson.Literal).String()
	evidence = append(evidence, EvidenceItem{Path: "global.json", Key: "sdk.version", Value: release})
	if _, err := semver.StrictNewVersion(release); err != nil {
		return unknown("invalid SDK version in global.json: " + err.Error())
	}
	return &Observation{Language: "dotnet", Tool: "dotnet", Release: &release, Evidence: evidence}, nil
}
