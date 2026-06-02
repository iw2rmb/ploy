package handlers

import (
	"errors"
	"net/http"
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
			packages, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePreGate)
			if err != nil {
				serverError(w, "get run sbom", "list pre sbom rows", err, "run_id", runID.String())
				return
			}
			writeJSON(w, http.StatusOK, migsapi.RunSBOMPackagesResponse{RunID: runID, View: view, Packages: packages})
		case "post":
			packages, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePostGate)
			if err != nil {
				serverError(w, "get run sbom", "list post sbom rows", err, "run_id", runID.String())
				return
			}
			writeJSON(w, http.StatusOK, migsapi.RunSBOMPackagesResponse{RunID: runID, View: view, Packages: packages})
		case "diff":
			pre, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePreGate)
			if err != nil {
				serverError(w, "get run sbom", "list pre sbom rows", err, "run_id", runID.String())
				return
			}
			post, err := listRunSBOMPackages(r, st, run.ID, domaintypes.JobTypePostGate)
			if err != nil {
				serverError(w, "get run sbom", "list post sbom rows", err, "run_id", runID.String())
				return
			}
			writeJSON(w, http.StatusOK, migsapi.RunSBOMDiffResponse{RunID: runID, View: view, Packages: diffSBOMPackages(pre, post)})
		}
	}
}

func listRunSBOMPackages(r *http.Request, st store.Store, runID domaintypes.RunID, jobType domaintypes.JobType) ([]migsapi.RunSBOMPackage, error) {
	rows, err := listRunSBOMRows(r, st, runID, jobType)
	if err != nil {
		return nil, err
	}
	packages := make([]migsapi.RunSBOMPackage, 0, len(rows))
	for _, row := range rows {
		packages = append(packages, migsapi.RunSBOMPackage{
			Package: row.Lib,
			Version: row.Ver,
		})
	}
	return packages, nil
}

func listRunSBOMRows(r *http.Request, st store.Store, runID domaintypes.RunID, jobType domaintypes.JobType) ([]store.ListRunSBOMRowsByJobTypeRow, error) {
	return st.ListRunSBOMRowsByJobType(r.Context(), store.ListRunSBOMRowsByJobTypeParams{
		RunID:   runID,
		JobType: jobType,
	})
}

func diffSBOMPackages(pre, post []migsapi.RunSBOMPackage) []migsapi.RunSBOMDiffPackage {
	preByPackage := sbomVersionsByPackage(pre)
	postByPackage := sbomVersionsByPackage(post)

	packageSet := make(map[string]struct{}, len(preByPackage)+len(postByPackage))
	for pkg := range preByPackage {
		packageSet[pkg] = struct{}{}
	}
	for pkg := range postByPackage {
		packageSet[pkg] = struct{}{}
	}

	packages := make([]string, 0, len(packageSet))
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	out := make([]migsapi.RunSBOMDiffPackage, 0)
	for _, pkg := range packages {
		preVersions := sortedSetValues(preByPackage[pkg])
		postVersions := sortedSetValues(postByPackage[pkg])
		if len(preVersions) == 1 && len(postVersions) == 1 && preVersions[0] != postVersions[0] {
			out = append(out, migsapi.RunSBOMDiffPackage{
				Package:     pkg,
				VersionPre:  preVersions[0],
				VersionPost: postVersions[0],
				Change:      "changed",
			})
			continue
		}
		for _, version := range preVersions {
			if _, ok := postByPackage[pkg][version]; ok {
				continue
			}
			out = append(out, migsapi.RunSBOMDiffPackage{
				Package:    pkg,
				VersionPre: version,
				Change:     "removed",
			})
		}
		for _, version := range postVersions {
			if _, ok := preByPackage[pkg][version]; ok {
				continue
			}
			out = append(out, migsapi.RunSBOMDiffPackage{
				Package:     pkg,
				VersionPost: version,
				Change:      "added",
			})
		}
	}
	return out
}

func sbomVersionsByPackage(packages []migsapi.RunSBOMPackage) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{})
	for _, pkg := range packages {
		name := strings.TrimSpace(pkg.Package)
		version := strings.TrimSpace(pkg.Version)
		if name == "" || version == "" {
			continue
		}
		if out[name] == nil {
			out[name] = map[string]struct{}{}
		}
		out[name][version] = struct{}{}
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
