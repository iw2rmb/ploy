package sbom

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// Row is one normalized SBOM package row bound to producer identity.
type Row struct {
	JobID  types.JobID
	RepoID types.RepoID
	Lib    string
	Ver    string
}

// ExtractRowsFromBundle parses supported SBOM files from a gzipped tar bundle
// and returns normalized rows keyed to the provided job/repo provenance.
func ExtractRowsFromBundle(bundle []byte, jobID types.JobID, repoID types.RepoID) ([]Row, error) {
	if len(bundle) == 0 {
		return nil, nil
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return nil, fmt.Errorf("open bundle gzip: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tr := tar.NewReader(gzReader)
	seen := map[string]struct{}{}
	rows := make([]Row, 0)
	var firstParseErr error

	for {
		hdr, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("read tar entry: %w", nextErr)
		}
		if hdr == nil || hdr.Typeflag == tar.TypeDir {
			continue
		}

		name := normalizeEntryPath(hdr.Name)
		if name == "" || !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}

		raw, readErr := io.ReadAll(tr)
		if readErr != nil {
			return nil, fmt.Errorf("read tar payload %q: %w", name, readErr)
		}

		pkgs, parsed, parseErr := parseSBOMJSON(raw)
		if parseErr != nil && firstParseErr == nil {
			firstParseErr = fmt.Errorf("%s: %w", name, parseErr)
		}
		if !parsed || len(pkgs) == 0 {
			continue
		}

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
			rows = append(rows, Row{
				JobID:  jobID,
				RepoID: repoID,
				Lib:    lib,
				Ver:    ver,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Lib == rows[j].Lib {
			return rows[i].Ver < rows[j].Ver
		}
		return rows[i].Lib < rows[j].Lib
	})
	return rows, firstParseErr
}

type packageTuple struct {
	Name    string
	Version string
}

func parseSBOMJSON(raw []byte) (pkgs []packageTuple, parsed bool, err error) {
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

func parseSPDXPackages(raw []byte) ([]packageTuple, error) {
	var doc struct {
		Packages []struct {
			Name        string `json:"name"`
			VersionInfo string `json:"versionInfo"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse spdx json: %w", err)
	}
	out := make([]packageTuple, 0, len(doc.Packages))
	for _, pkg := range doc.Packages {
		out = append(out, packageTuple{Name: pkg.Name, Version: pkg.VersionInfo})
	}
	return out, nil
}

func parseCycloneDXComponents(raw []byte) ([]packageTuple, error) {
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
	out := make([]packageTuple, 0)
	var walk func(items []component)
	walk = func(items []component) {
		for _, item := range items {
			out = append(out, packageTuple{Name: item.Name, Version: item.Version})
			if len(item.Components) > 0 {
				walk(item.Components)
			}
		}
	}
	walk(doc.Components)
	return out, nil
}

func normalizeEntryPath(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(trimmed, "/"))
	if cleaned == "/" || strings.HasPrefix(cleaned, "/../") {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func normalizeLib(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeVer(version string) string {
	return strings.TrimSpace(version)
}
