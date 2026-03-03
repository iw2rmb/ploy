package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/iw2rmb/ploy/internal/store"
)

type sbomCompatSelector struct {
	Name  string
	Floor string
}

func sbomCompatHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := strings.TrimSpace(r.URL.Query().Get("lang"))
		release := strings.TrimSpace(r.URL.Query().Get("release"))
		tool := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tool")))
		libsRaw := strings.TrimSpace(r.URL.Query().Get("libs"))

		if lang == "" || release == "" || tool == "" || libsRaw == "" {
			httpErr(w, http.StatusBadRequest, "lang, release, tool, and libs are required")
			return
		}

		selectors, libs, parseErr := parseSBOMCompatSelectors(libsRaw)
		if parseErr != nil {
			httpErr(w, http.StatusBadRequest, "invalid libs selector: %v", parseErr)
			return
		}

		hasEvidence, err := st.HasSBOMEvidenceForStack(r.Context(), store.HasSBOMEvidenceForStackParams{
			Lang:    strings.ToLower(lang),
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
			Lang:    strings.ToLower(lang),
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

		result := make(map[string]string, len(selectors))
		for _, selector := range selectors {
			versions := versionsByLib[selector.Name]
			if len(versions) == 0 {
				continue
			}
			best := ""
			for _, v := range versions {
				if selector.Floor != "" && compareLooseVersions(v, selector.Floor) < 0 {
					continue
				}
				if best == "" || compareLooseVersions(v, best) < 0 {
					best = v
				}
			}
			if best != "" {
				result[selector.Name] = best
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func parseSBOMCompatSelectors(raw string) ([]sbomCompatSelector, []string, error) {
	parts := strings.Split(raw, ",")
	out := make([]sbomCompatSelector, 0, len(parts))
	seen := map[string]string{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		name := ""
		floor := ""
		if strings.Contains(item, ":") {
			nv := strings.SplitN(item, ":", 2)
			name = strings.ToLower(strings.TrimSpace(nv[0]))
			floor = strings.TrimSpace(nv[1])
			if floor == "" {
				return nil, nil, errCompatSelector(item)
			}
		} else {
			name = strings.ToLower(strings.TrimSpace(item))
		}
		if name == "" {
			return nil, nil, errCompatSelector(item)
		}
		if prev, ok := seen[name]; ok {
			if prev != floor {
				return nil, nil, errCompatSelector(item)
			}
			continue
		}
		seen[name] = floor
		out = append(out, sbomCompatSelector{Name: name, Floor: floor})
	}
	if len(out) == 0 {
		return nil, nil, errCompatSelector(raw)
	}
	libs := make([]string, 0, len(out))
	for _, selector := range out {
		libs = append(libs, selector.Name)
	}
	sort.Strings(libs)
	return out, libs, nil
}

func errCompatSelector(item string) error {
	return fmt.Errorf("%q", item)
}

func compareLooseVersions(a, b string) int {
	at := tokenizeVersion(a)
	bt := tokenizeVersion(b)
	maxLen := len(at)
	if len(bt) > maxLen {
		maxLen = len(bt)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(at) {
			return -1
		}
		if i >= len(bt) {
			return 1
		}
		av := at[i]
		bv := bt[i]
		aNum := isAllDigits(av)
		bNum := isAllDigits(bv)
		if aNum && bNum {
			av = trimLeadingZeros(av)
			bv = trimLeadingZeros(bv)
			if len(av) < len(bv) {
				return -1
			}
			if len(av) > len(bv) {
				return 1
			}
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func tokenizeVersion(v string) []string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return nil
	}

	tokens := make([]string, 0)
	var current strings.Builder
	tokenKind := 0 // 0 none, 1 digits, 2 letters
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
		tokenKind = 0
	}

	for _, r := range v {
		switch {
		case unicode.IsDigit(r):
			if tokenKind != 0 && tokenKind != 1 {
				flush()
			}
			tokenKind = 1
			current.WriteRune(r)
		case unicode.IsLetter(r):
			if tokenKind != 0 && tokenKind != 2 {
				flush()
			}
			tokenKind = 2
			current.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func trimLeadingZeros(s string) string {
	trimmed := strings.TrimLeft(s, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}
