package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/server/sbom"
	"github.com/iw2rmb/ploy/internal/store"
)

type sbomCompatSelector struct {
	Name  string
	Floor string
}

type sbomCompatSelectorInput struct {
	Raw        string
	Candidates []sbomCompatSelector
}

func sbomCompatHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))
		release := strings.TrimSpace(r.URL.Query().Get("release"))
		tool := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tool")))
		libsRaw := strings.TrimSpace(r.URL.Query().Get("libs"))

		if lang == "" || release == "" || tool == "" || libsRaw == "" {
			httpErr(w, http.StatusBadRequest, "lang, release, tool, and libs are required")
			return
		}

		selectorInputs, libs, parseErr := parseSBOMCompatSelectors(libsRaw)
		if parseErr != nil {
			httpErr(w, http.StatusBadRequest, "invalid libs selector: %v", parseErr)
			return
		}

		hasEvidence, err := st.HasSBOMEvidenceForStack(r.Context(), store.HasSBOMEvidenceForStackParams{
			Lang:    lang,
			Release: release,
			Tool:    tool,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to query sbom evidence: %v", err)
			return
		}
		if !hasEvidence {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("null"))
			return
		}

		rows, err := st.ListSBOMCompatRows(r.Context(), store.ListSBOMCompatRowsParams{
			Lang:    lang,
			Release: release,
			Tool:    tool,
			Libs:    libs,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to query sbom compatibility rows: %v", err)
			return
		}

		versionsByLib := make(map[string][]string, len(libs))
		for _, row := range rows {
			lib := strings.ToLower(strings.TrimSpace(row.Lib))
			ver := strings.TrimSpace(row.Ver)
			if lib == "" || ver == "" {
				continue
			}
			versionsByLib[lib] = append(versionsByLib[lib], ver)
		}

		selectors := make([]sbomCompatSelector, 0, len(selectorInputs))
		seen := map[string]string{}
		for _, input := range selectorInputs {
			selected, ok := resolveSBOMCompatSelector(input, versionsByLib, lang, tool)
			if !ok {
				continue
			}
			if prevFloor, exists := seen[selected.Name]; exists {
				if prevFloor != selected.Floor {
					httpErr(w, http.StatusBadRequest, "invalid libs selector: %v", errCompatSelector(input.Raw))
					return
				}
				continue
			}
			seen[selected.Name] = selected.Floor
			selectors = append(selectors, selected)
		}

		result := make(map[string]string, len(selectors))
		for _, selector := range selectors {
			best := bestSBOMCompatVersion(selector, versionsByLib, lang, tool)
			if best != "" {
				result[selector.Name] = best
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func parseSBOMCompatSelectors(raw string) ([]sbomCompatSelectorInput, []string, error) {
	parts := strings.Split(raw, ",")
	out := make([]sbomCompatSelectorInput, 0, len(parts))
	seenFloors := map[string]string{}
	libsSet := map[string]struct{}{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		candidates, ok := parseSBOMCompatSelector(item)
		if !ok {
			return nil, nil, errCompatSelector(item)
		}
		for _, candidate := range candidates {
			if candidate.Name == "" {
				return nil, nil, errCompatSelector(item)
			}
			libsSet[candidate.Name] = struct{}{}
			if candidate.Floor == "" {
				continue
			}
			if prev, exists := seenFloors[candidate.Name]; exists && prev != candidate.Floor {
				return nil, nil, errCompatSelector(item)
			}
			seenFloors[candidate.Name] = candidate.Floor
		}
		out = append(out, sbomCompatSelectorInput{
			Raw:        item,
			Candidates: candidates,
		})
	}
	if len(out) == 0 {
		return nil, nil, errCompatSelector(raw)
	}
	libs := make([]string, 0, len(libsSet))
	for lib := range libsSet {
		libs = append(libs, lib)
	}
	sort.Strings(libs)
	return out, libs, nil
}

func parseSBOMCompatSelector(item string) (candidates []sbomCompatSelector, ok bool) {
	switch strings.Count(item, ":") {
	case 0:
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "" {
			return nil, false
		}
		return []sbomCompatSelector{{Name: name}}, true
	case 1:
		nv := strings.SplitN(item, ":", 2)
		left := strings.TrimSpace(nv[0])
		right := strings.TrimSpace(nv[1])
		if left == "" || right == "" {
			return nil, false
		}
		return []sbomCompatSelector{
			{Name: strings.ToLower(strings.TrimSpace(item))},
			{Name: strings.ToLower(left), Floor: right},
		}, true
	default:
		idx := strings.LastIndex(item, ":")
		left := strings.TrimSpace(item[:idx])
		right := strings.TrimSpace(item[idx+1:])
		if left == "" || right == "" {
			return nil, false
		}
		return []sbomCompatSelector{{Name: strings.ToLower(left), Floor: right}}, true
	}
}

func resolveSBOMCompatSelector(
	input sbomCompatSelectorInput,
	versionsByLib map[string][]string,
	lang string,
	tool string,
) (selector sbomCompatSelector, ok bool) {
	for _, candidate := range input.Candidates {
		if bestSBOMCompatVersion(candidate, versionsByLib, lang, tool) != "" {
			return candidate, true
		}
	}
	return sbomCompatSelector{}, false
}

func bestSBOMCompatVersion(
	selector sbomCompatSelector,
	versionsByLib map[string][]string,
	lang string,
	tool string,
) string {
	versions := versionsByLib[selector.Name]
	if len(versions) == 0 {
		return ""
	}
	best := ""
	for _, v := range versions {
		if selector.Floor != "" && sbom.CompareVersions(lang, tool, v, selector.Floor) < 0 {
			continue
		}
		if best == "" || sbom.CompareVersions(lang, tool, v, best) < 0 {
			best = v
		}
	}
	return best
}

func errCompatSelector(item string) error {
	return fmt.Errorf("%q", item)
}
