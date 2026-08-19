package sbom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Package is one normalized package/version pair parsed from an SBOM document.
type Package struct {
	Name    string
	Version string
}

// ExtractPackagesFromJSON parses one supported SBOM JSON document and returns
// normalized package/version pairs without attaching producer provenance.
func ExtractPackagesFromJSON(raw []byte) ([]Package, error) {
	pkgs, parsed, err := parseSBOMJSON(raw)
	if err != nil {
		return nil, err
	}
	if !parsed || len(pkgs) == 0 {
		return nil, nil
	}

	seen := map[string]struct{}{}
	packages := make([]Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		lib := normalizeLib(pkg.Name)
		ver := normalizeVer(pkg.Version)
		if lib == "" || ver == "" {
			continue
		}
		key := lib + "\x00" + ver
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		packages = append(packages, Package{
			Name:    lib,
			Version: ver,
		})
	}

	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Name == packages[j].Name {
			return packages[i].Version < packages[j].Version
		}
		return packages[i].Name < packages[j].Name
	})
	return packages, nil
}

// rawPackage preserves source values until format-independent normalization and deduplication.
type rawPackage struct {
	Name    string
	Version string
}

func parseSBOMJSON(raw []byte) (pkgs []rawPackage, parsed bool, err error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}
	var sniff struct {
		SPDXVersion string          `json:"spdxVersion"`
		BOMFormat   string          `json:"bomFormat"`
		Packages    json.RawMessage `json:"packages"`
		Components  json.RawMessage `json:"components"`
	}
	if unmarshalErr := json.Unmarshal(raw, &sniff); unmarshalErr != nil {
		return nil, false, nil
	}

	if strings.TrimSpace(sniff.SPDXVersion) != "" || len(sniff.Packages) > 0 {
		spdxPkgs, parseErr := parseSPDXPackages(raw)
		return spdxPkgs, true, parseErr
	}
	if strings.EqualFold(strings.TrimSpace(sniff.BOMFormat), "CycloneDX") || len(sniff.Components) > 0 {
		cdxPkgs, parseErr := parseCycloneDXComponents(raw)
		return cdxPkgs, true, parseErr
	}
	return nil, false, nil
}

func parseSPDXPackages(raw []byte) ([]rawPackage, error) {
	var doc struct {
		Packages []struct {
			Name        string `json:"name"`
			VersionInfo string `json:"versionInfo"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse spdx json: %w", err)
	}
	out := make([]rawPackage, 0, len(doc.Packages))
	for _, pkg := range doc.Packages {
		out = append(out, rawPackage{Name: pkg.Name, Version: pkg.VersionInfo})
	}
	return out, nil
}

func parseCycloneDXComponents(raw []byte) ([]rawPackage, error) {
	type component struct {
		Name       string      `json:"name"`
		Version    string      `json:"version"`
		Components []component `json:"components"`
	}
	var doc struct {
		Components []component `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse cyclonedx json: %w", err)
	}
	out := make([]rawPackage, 0)
	var walk func(items []component)
	walk = func(items []component) {
		for _, item := range items {
			out = append(out, rawPackage{Name: item.Name, Version: item.Version})
			if len(item.Components) > 0 {
				walk(item.Components)
			}
		}
	}
	walk(doc.Components)
	return out, nil
}

func normalizeLib(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeVer(version string) string {
	return strings.TrimSpace(version)
}
