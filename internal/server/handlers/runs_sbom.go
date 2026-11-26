package handlers

import (
	"cmp"
	"errors"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func getRunSBOMHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		view := strings.TrimSpace(r.PathValue("view"))
		if view != "pre" && view != "post" && view != "diff" {
			writeHTTPError(w, http.StatusBadRequest, "invalid sbom view")
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "get run sbom")
		if !ok {
			return
		}
		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "spec not found")
				return
			}
			serverError(w, "get run sbom", "get spec", err, "run_id", runID.String(), "spec_id", run.SpecID.String())
			return
		}
		parsed, err := contracts.ParseMigSpecJSON(spec.Spec)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to parse run spec: %v", err)
			return
		}
		if parsed.BuildGate != nil && parsed.BuildGate.Disabled {
			writeHTTPError(w, http.StatusBadRequest, "build gate disabled for run")
			return
		}

		switch view {
		case "pre":
			packages, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePreGate, false)
			if err != nil {
				serverError(w, "get run sbom", "list pre sbom rows", err, "run_id", runID.String())
				return
			}
			writeJSON(w, http.StatusOK, migsapi.RunSBOMPackagesResponse{RunID: runID, View: view, Packages: packages})
		case "post":
			packages, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePostGate, false)
			if err != nil {
				serverError(w, "get run sbom", "list post sbom rows", err, "run_id", runID.String())
				return
			}
			writeJSON(w, http.StatusOK, migsapi.RunSBOMPackagesResponse{RunID: runID, View: view, Packages: packages})
		case "diff":
			pre, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePreGate, true)
			if err != nil {
				serverError(w, "get run sbom", "list pre sbom rows", err, "run_id", runID.String())
				return
			}
			post, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePostGate, true)
			if err != nil {
				serverError(w, "get run sbom", "list post sbom rows", err, "run_id", runID.String())
				return
			}
			writeJSON(w, http.StatusOK, migsapi.RunSBOMDiffResponse{RunID: runID, View: view, Packages: diffSBOMPackages(pre, post)})
		}
	}
}

func listRunSBOMPackages(r *http.Request, st store.Store, runID domaintypes.RunID, jobType domaintypes.JobType, byModule bool) ([]migsapi.RunSBOMPackage, error) {
	rows, err := st.ListRunSBOMRowsByJobType(r.Context(), store.ListRunSBOMRowsByJobTypeParams{RunID: runID, JobType: jobType, ByModule: byModule})
	if err != nil {
		return nil, err
	}
	packages := make([]migsapi.RunSBOMPackage, 0, len(rows))
	for _, row := range rows {
		packages = append(packages, migsapi.RunSBOMPackage{
			ModuleID: row.ModuleID,
			Package:  row.Lib,
			Version:  row.Ver,
		})
	}
	return packages, nil
}

type sbomPackageID struct {
	moduleID string
	name     string
}

func diffSBOMPackages(pre, post []migsapi.RunSBOMPackage) []migsapi.RunSBOMDiffPackage {
	// A legacy gate supplies only aggregate rows. Comparing module keys against
	// those rows would report a module change for every shared package.
	unknownModule := func(p migsapi.RunSBOMPackage) bool { return p.ModuleID == "" }
	byModule := !slices.ContainsFunc(pre, unknownModule) && !slices.ContainsFunc(post, unknownModule)
	preByPackage := sbomVersionsByPackage(pre, byModule)
	postByPackage := sbomVersionsByPackage(post, byModule)

	packageSet := make(map[sbomPackageID]struct{}, len(preByPackage)+len(postByPackage))
	for pkg := range preByPackage {
		packageSet[pkg] = struct{}{}
	}
	for pkg := range postByPackage {
		packageSet[pkg] = struct{}{}
	}

	packages := slices.SortedFunc(maps.Keys(packageSet), func(a, b sbomPackageID) int {
		return cmp.Or(cmp.Compare(a.moduleID, b.moduleID), cmp.Compare(a.name, b.name))
	})

	out := make([]migsapi.RunSBOMDiffPackage, 0)
	for _, pkg := range packages {
		preVersions := sortedSetValues(preByPackage[pkg])
		postVersions := sortedSetValues(postByPackage[pkg])
		preOnly := versionsAbsentFrom(preVersions, postByPackage[pkg])
		postOnly := versionsAbsentFrom(postVersions, preByPackage[pkg])

		if len(preOnly) > 0 && len(postOnly) > 0 {
			pairedPre := preOnly[len(preOnly)-1]
			pairedPost := postOnly[len(postOnly)-1]
			for _, version := range preOnly[:len(preOnly)-1] {
				out = append(out, migsapi.RunSBOMDiffPackage{
					ModuleID:   pkg.moduleID,
					Package:    pkg.name,
					VersionPre: version,
					Change:     "removed",
				})
			}
			out = append(out, migsapi.RunSBOMDiffPackage{
				ModuleID:    pkg.moduleID,
				Package:     pkg.name,
				VersionPre:  pairedPre,
				VersionPost: pairedPost,
				Change:      "changed",
			})
			for _, version := range postOnly[:len(postOnly)-1] {
				out = append(out, migsapi.RunSBOMDiffPackage{
					ModuleID:    pkg.moduleID,
					Package:     pkg.name,
					VersionPost: version,
					Change:      "added",
				})
			}
			continue
		}
		for _, version := range preOnly {
			out = append(out, migsapi.RunSBOMDiffPackage{
				ModuleID:   pkg.moduleID,
				Package:    pkg.name,
				VersionPre: version,
				Change:     "removed",
			})
		}
		for _, version := range postOnly {
			out = append(out, migsapi.RunSBOMDiffPackage{
				ModuleID:    pkg.moduleID,
				Package:     pkg.name,
				VersionPost: version,
				Change:      "added",
			})
		}
	}
	return out
}

func versionsAbsentFrom(sortedVersions []string, other map[string]struct{}) []string {
	out := make([]string, 0, len(sortedVersions))
	for _, version := range sortedVersions {
		if _, ok := other[version]; ok {
			continue
		}
		out = append(out, version)
	}
	return out
}

func sbomVersionsByPackage(packages []migsapi.RunSBOMPackage, byModule bool) map[sbomPackageID]map[string]struct{} {
	out := make(map[sbomPackageID]map[string]struct{})
	for _, pkg := range packages {
		name := strings.TrimSpace(pkg.Package)
		version := strings.TrimSpace(pkg.Version)
		if name == "" || version == "" {
			continue
		}
		key := sbomPackageID{name: name}
		if byModule {
			key.moduleID = pkg.ModuleID
		}
		if out[key] == nil {
			out[key] = map[string]struct{}{}
		}
		out[key][version] = struct{}{}
	}
	return out
}

func sortedSetValues(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
