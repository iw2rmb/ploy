package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

var (
	namedSpecNameRE      = regexp.MustCompile(`^[0-9a-z._-]+$`)
	namedSpecSHARE       = regexp.MustCompile(`^[0-9a-f]{40}$`)
	namedSpecSHAPrefixRE = regexp.MustCompile(`^[0-9a-f]{8,40}$`)
)

func publishNamedSpecHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req domainapi.PublishNamedSpecRequest
		if err := decodeRequestJSON(w, r, &req, maxMigSpecSize); err != nil {
			return
		}

		storedDescription, sourceJSON, ok := validatePublishNamedSpecRequest(w, &req)
		if !ok {
			return
		}

		existing, err := st.GetNamedSpecByNameSourceSHA(r.Context(), store.GetNamedSpecByNameSourceSHAParams{
			Name:   req.Name,
			Domain: req.Source.Domain,
			Repo:   req.Source.Repo,
			Sha:    req.SHA,
		})
		if err == nil {
			writeJSON(w, http.StatusOK, namedSpecSummaryFromSpec(existing, true))
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			serverError(w, "publish named spec", "lookup named spec", err, "name", req.Name, "domain", req.Source.Domain, "repo", req.Source.Repo, "sha", req.SHA)
			return
		}

		createdBy, err := resolvedCreatedBy(r.Context(), st, stringPtrValue(req.CreatedBy))
		if err != nil {
			serverError(w, "publish named spec", "resolve created_by", err)
			return
		}

		created, err := st.CreateNamedSpec(r.Context(), store.CreateNamedSpecParams{
			ID:                domaintypes.NewSpecID(),
			Name:              req.Name,
			Description:       storedDescription,
			Source:            sourceJSON,
			Sha:               req.SHA,
			SourceCommittedAt: pgtype.Timestamptz{Time: req.SourceCommittedAt, Valid: true},
			Spec:              req.Spec,
			CreatedBy:         createdBy,
		})
		if err == nil {
			writeJSON(w, http.StatusCreated, namedSpecSummaryFromSpec(created, false))
			return
		}
		if !isUniqueViolation(err) {
			serverError(w, "publish named spec", "create named spec", err, "name", req.Name, "domain", req.Source.Domain, "repo", req.Source.Repo, "sha", req.SHA)
			return
		}

		existing, lookupErr := st.GetNamedSpecByNameSourceSHA(r.Context(), store.GetNamedSpecByNameSourceSHAParams{
			Name:   req.Name,
			Domain: req.Source.Domain,
			Repo:   req.Source.Repo,
			Sha:    req.SHA,
		})
		if lookupErr == nil {
			writeJSON(w, http.StatusOK, namedSpecSummaryFromSpec(existing, true))
			return
		}
		if !errors.Is(lookupErr, pgx.ErrNoRows) {
			serverError(w, "publish named spec", "reload named spec after conflict", lookupErr, "name", req.Name, "domain", req.Source.Domain, "repo", req.Source.Repo, "sha", req.SHA)
			return
		}
		writeHTTPError(w, http.StatusConflict, "named spec uniqueness conflict")
	}
}

func listNamedSpecsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		named := strings.TrimSpace(r.URL.Query().Get("named"))
		if named != "" && named != "true" {
			writeHTTPError(w, http.StatusBadRequest, "named must be true when provided")
			return
		}
		limit, offset, err := parsePagination(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		archived, err := parseBoolQueryDefault(r, "archived", false)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		rows, err := st.ListLatestNamedSpecs(r.Context(), store.ListLatestNamedSpecsParams{Limit: limit, Offset: offset, Archived: archived})
		if err != nil {
			serverError(w, "list named specs", "list named specs", err)
			return
		}
		summaries := make([]domainapi.NamedSpecSummary, 0, len(rows))
		for _, row := range rows {
			summary, err := namedSpecSummaryFromListRow(row, false)
			if err != nil {
				serverError(w, "list named specs", "decode named spec source", err, "spec_id", row.ID)
				return
			}
			summaries = append(summaries, summary)
		}
		writeJSON(w, http.StatusOK, domainapi.NamedSpecListResponse{Specs: summaries})
	}
}

func resolveNamedSpecHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		selector := strings.TrimSpace(r.URL.Query().Get("selector"))
		shaPrefix := strings.TrimSpace(r.URL.Query().Get("sha"))
		if strings.Contains(selector, "@") {
			var selectorSHA string
			var err error
			selector, selectorSHA, err = splitNamedSpecVersionSelector(selector)
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, "%s", err)
				return
			}
			if shaPrefix != "" && shaPrefix != selectorSHA {
				writeHTTPError(w, http.StatusBadRequest, "selector sha and sha query parameter conflict")
				return
			}
			shaPrefix = selectorSHA
		}
		parsed, err := parseNamedSpecSelector(selector)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		if shaPrefix != "" && !namedSpecSHAPrefixRE.MatchString(shaPrefix) {
			writeHTTPError(w, http.StatusBadRequest, "sha must be a lowercase 8-40 character hex prefix")
			return
		}
		archived, err := parseBoolQueryDefault(r, "archived", false)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		matches, err := resolveNamedSpecRows(r.Context(), st, parsed, shaPrefix, archived)
		if err != nil {
			serverError(w, "resolve named spec", "resolve named spec", err, "selector", selector)
			return
		}
		if len(matches) == 0 {
			writeHTTPError(w, http.StatusNotFound, "named spec not found")
			return
		}
		if len(matches) > 1 {
			writeHTTPError(w, http.StatusConflict, "named spec selector %s is ambiguous: %s", selector, formatNamedSpecChoices(matches))
			return
		}

		match := matches[0]
		summary, err := namedSpecSummaryFromResolveMatch(match, false)
		if err != nil {
			serverError(w, "resolve named spec", "decode named spec source", err, "spec_id", match.id)
			return
		}
		writeJSON(w, http.StatusOK, domainapi.NamedSpecResolveResponse{
			NamedSpecSummary: summary,
			Spec:             json.RawMessage(match.spec),
		})
	}
}

func updateNamedSpecHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		specID := strings.TrimSpace(r.PathValue("spec_id"))
		if specID == "" {
			writeHTTPError(w, http.StatusBadRequest, "spec_id is required")
			return
		}
		var req domainapi.UpdateNamedSpecRequest
		if err := decodeRequestJSON(w, r, &req, maxMigSpecSize); err != nil {
			return
		}
		updatedBy, err := resolvedCreatedBy(r.Context(), st, "")
		if err != nil {
			serverError(w, "update named spec", "resolve updated_by", err, "spec_id", specID)
			return
		}
		updated, err := st.UpdateNamedSpecArchiveState(r.Context(), store.UpdateNamedSpecArchiveStateParams{
			ID:        specID,
			Archived:  req.Archived,
			UpdatedBy: updatedBy,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeHTTPError(w, http.StatusNotFound, "named spec not found")
			return
		}
		if err != nil {
			serverError(w, "update named spec", "update archive state", err, "spec_id", specID)
			return
		}
		writeJSON(w, http.StatusOK, namedSpecSummaryFromSpec(updated, false))
	}
}

func validatePublishNamedSpecRequest(w http.ResponseWriter, req *domainapi.PublishNamedSpecRequest) (string, []byte, bool) {
	if !namedSpecNameRE.MatchString(req.Name) {
		writeHTTPError(w, http.StatusBadRequest, "name must match ^[0-9a-z._-]+$")
		return "", nil, false
	}
	req.Source.Domain = strings.TrimSpace(req.Source.Domain)
	req.Source.Repo = strings.Trim(strings.TrimSpace(req.Source.Repo), "/")
	if !validNamedSpecSource(req.Source) {
		writeHTTPError(w, http.StatusBadRequest, "source must include credential-free domain and repo")
		return "", nil, false
	}
	if !namedSpecSHARE.MatchString(req.SHA) {
		writeHTTPError(w, http.StatusBadRequest, "sha must be lowercase 40-hex")
		return "", nil, false
	}
	if req.SourceCommittedAt.IsZero() {
		writeHTTPError(w, http.StatusBadRequest, "source_committed_at is required")
		return "", nil, false
	}
	if len(req.Spec) == 0 {
		writeHTTPError(w, http.StatusBadRequest, "spec is required")
		return "", nil, false
	}
	parsed, err := contracts.ParseMigSpecJSON(req.Spec)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "spec: %v", err)
		return "", nil, false
	}
	if parsed.APIVersion != "ploy.mig/v1alpha1" {
		writeHTTPError(w, http.StatusBadRequest, "spec.apiVersion must be ploy.mig/v1alpha1")
		return "", nil, false
	}
	if parsed.Name != req.Name {
		writeHTTPError(w, http.StatusBadRequest, "spec.name must match request name")
		return "", nil, false
	}
	storedDescription := req.Description
	if parsed.Description != "" {
		if parsed.Description != req.Description {
			writeHTTPError(w, http.StatusBadRequest, "description must match spec.description")
			return "", nil, false
		}
		storedDescription = parsed.Description
	}
	sourceJSON, err := json.Marshal(req.Source)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "source is invalid: %v", err)
		return "", nil, false
	}
	return storedDescription, sourceJSON, true
}

func validNamedSpecSource(source domainapi.NamedSpecSource) bool {
	if source.Domain == "" || source.Repo == "" {
		return false
	}
	if strings.ContainsAny(source.Domain, "/:@") || strings.Contains(source.Domain, "..") {
		return false
	}
	if strings.ContainsAny(source.Repo, ":@") || strings.Contains(source.Repo, "//") || strings.HasSuffix(source.Repo, ".git") {
		return false
	}
	return true
}

type namedSpecSelector struct {
	name   string
	domain string
	repo   string
}

func parseNamedSpecSelector(selector string) (namedSpecSelector, error) {
	if selector == "" {
		return namedSpecSelector{}, fmt.Errorf("selector query parameter is required")
	}
	if strings.Count(selector, ":") > 1 {
		return namedSpecSelector{}, fmt.Errorf("invalid named spec selector: %s", selector)
	}
	if !strings.Contains(selector, ":") {
		if !namedSpecNameRE.MatchString(selector) {
			return namedSpecSelector{}, fmt.Errorf("invalid named spec selector: %s", selector)
		}
		return namedSpecSelector{name: selector}, nil
	}

	parts := strings.SplitN(selector, ":", 2)
	path, name := parts[0], parts[1]
	pathParts := strings.Split(path, "/")
	if path == "" || name == "" || !namedSpecNameRE.MatchString(name) || len(pathParts) < 2 {
		return namedSpecSelector{}, fmt.Errorf("invalid named spec selector: %s", selector)
	}
	for _, part := range pathParts {
		if part == "" {
			return namedSpecSelector{}, fmt.Errorf("invalid named spec selector: %s", selector)
		}
	}
	if len(pathParts) == 2 {
		return namedSpecSelector{name: name, repo: path}, nil
	}
	return namedSpecSelector{name: name, domain: pathParts[0], repo: strings.Join(pathParts[1:], "/")}, nil
}

func splitNamedSpecVersionSelector(selector string) (string, string, error) {
	idx := strings.LastIndex(selector, "@")
	if idx < 0 {
		return selector, "", nil
	}
	base := strings.TrimSpace(selector[:idx])
	shaPrefix := strings.TrimSpace(selector[idx+1:])
	if base == "" || shaPrefix == "" {
		return "", "", fmt.Errorf("invalid named spec selector: %s", selector)
	}
	if !namedSpecSHAPrefixRE.MatchString(shaPrefix) {
		return "", "", fmt.Errorf("sha must be a lowercase 8-40 character hex prefix")
	}
	return base, shaPrefix, nil
}

type namedSpecResolveMatch struct {
	id                string
	name              string
	description       string
	source            []byte
	sha               string
	sourceCommittedAt pgtype.Timestamptz
	spec              []byte
	createdBy         *string
	updatedBy         *string
	createdAt         pgtype.Timestamptz
	archivedAt        pgtype.Timestamptz
}

func resolveNamedSpecRows(ctx context.Context, st store.Store, selector namedSpecSelector, shaPrefix string, archived bool) ([]namedSpecResolveMatch, error) {
	if shaPrefix != "" {
		return resolveNamedSpecVersionRows(ctx, st, selector, shaPrefix, archived)
	}
	if selector.domain != "" {
		rows, err := st.ResolveLatestNamedSpecByDomainRepoName(ctx, store.ResolveLatestNamedSpecByDomainRepoNameParams{
			Name: selector.name, Domain: selector.domain, Repo: selector.repo, Archived: archived,
		})
		return mapNamedSpecResolveRows(rows, func(row store.ResolveLatestNamedSpecByDomainRepoNameRow) namedSpecResolveMatch {
			return namedSpecResolveMatch{id: row.ID, name: row.Name, description: row.Description, source: row.Source, sha: row.Sha, sourceCommittedAt: row.SourceCommittedAt, spec: row.Spec, createdBy: row.CreatedBy, updatedBy: row.UpdatedBy, createdAt: row.CreatedAt, archivedAt: row.ArchivedAt}
		}), err
	}
	if selector.repo != "" {
		rows, err := st.ResolveLatestNamedSpecByRepoName(ctx, store.ResolveLatestNamedSpecByRepoNameParams{Name: selector.name, Repo: selector.repo, Archived: archived})
		return mapNamedSpecResolveRows(rows, func(row store.ResolveLatestNamedSpecByRepoNameRow) namedSpecResolveMatch {
			return namedSpecResolveMatch{id: row.ID, name: row.Name, description: row.Description, source: row.Source, sha: row.Sha, sourceCommittedAt: row.SourceCommittedAt, spec: row.Spec, createdBy: row.CreatedBy, updatedBy: row.UpdatedBy, createdAt: row.CreatedAt, archivedAt: row.ArchivedAt}
		}), err
	}
	rows, err := st.ResolveLatestNamedSpecByName(ctx, store.ResolveLatestNamedSpecByNameParams{Name: selector.name, Archived: archived})
	return mapNamedSpecResolveRows(rows, func(row store.ResolveLatestNamedSpecByNameRow) namedSpecResolveMatch {
		return namedSpecResolveMatch{id: row.ID, name: row.Name, description: row.Description, source: row.Source, sha: row.Sha, sourceCommittedAt: row.SourceCommittedAt, spec: row.Spec, createdBy: row.CreatedBy, updatedBy: row.UpdatedBy, createdAt: row.CreatedAt, archivedAt: row.ArchivedAt}
	}), err
}

func resolveNamedSpecVersionRows(ctx context.Context, st store.Store, selector namedSpecSelector, shaPrefix string, archived bool) ([]namedSpecResolveMatch, error) {
	if selector.domain != "" {
		rows, err := st.ResolveNamedSpecVersionByDomainRepoName(ctx, store.ResolveNamedSpecVersionByDomainRepoNameParams{
			Name: selector.name, Domain: selector.domain, Repo: selector.repo, ShaPrefix: shaPrefix, Archived: archived,
		})
		return mapNamedSpecResolveRows(rows, specToNamedSpecResolveMatch), err
	}
	if selector.repo != "" {
		rows, err := st.ResolveNamedSpecVersionByRepoName(ctx, store.ResolveNamedSpecVersionByRepoNameParams{
			Name: selector.name, Repo: selector.repo, ShaPrefix: shaPrefix, Archived: archived,
		})
		return mapNamedSpecResolveRows(rows, specToNamedSpecResolveMatch), err
	}
	rows, err := st.ResolveNamedSpecVersionByName(ctx, store.ResolveNamedSpecVersionByNameParams{
		Name: selector.name, ShaPrefix: shaPrefix, Archived: archived,
	})
	return mapNamedSpecResolveRows(rows, specToNamedSpecResolveMatch), err
}

func mapNamedSpecResolveRows[T any](rows []T, mapper func(T) namedSpecResolveMatch) []namedSpecResolveMatch {
	matches := make([]namedSpecResolveMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, mapper(row))
	}
	return matches
}

func namedSpecSummaryFromSpec(row store.Spec, skipped bool) domainapi.NamedSpecSummary {
	source, _ := decodeNamedSpecSource(row.Source)
	summary := domainapi.NamedSpecSummary{
		ID:                row.ID.String(),
		Name:              row.Name,
		Description:       row.Description,
		Source:            source,
		SHA:               row.Sha,
		SourceCommittedAt: row.SourceCommittedAt.Time,
		CreatedBy:         row.CreatedBy,
		UpdatedBy:         row.UpdatedBy,
		CreatedAt:         row.CreatedAt.Time,
		Skipped:           skipped,
	}
	if row.ArchivedAt.Valid {
		summary.ArchivedAt = &row.ArchivedAt.Time
	}
	return summary
}

func namedSpecSummaryFromListRow(row store.ListLatestNamedSpecsRow, skipped bool) (domainapi.NamedSpecSummary, error) {
	source, err := decodeNamedSpecSource(row.Source)
	if err != nil {
		return domainapi.NamedSpecSummary{}, err
	}
	summary := domainapi.NamedSpecSummary{ID: row.ID, Name: row.Name, Description: row.Description, Source: source, SHA: row.Sha, SourceCommittedAt: row.SourceCommittedAt.Time, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt.Time, Skipped: skipped}
	if row.ArchivedAt.Valid {
		summary.ArchivedAt = &row.ArchivedAt.Time
	}
	return summary, nil
}

func namedSpecSummaryFromResolveMatch(row namedSpecResolveMatch, skipped bool) (domainapi.NamedSpecSummary, error) {
	source, err := decodeNamedSpecSource(row.source)
	if err != nil {
		return domainapi.NamedSpecSummary{}, err
	}
	summary := domainapi.NamedSpecSummary{ID: row.id, Name: row.name, Description: row.description, Source: source, SHA: row.sha, SourceCommittedAt: row.sourceCommittedAt.Time, CreatedBy: row.createdBy, UpdatedBy: row.updatedBy, CreatedAt: row.createdAt.Time, Skipped: skipped}
	if row.archivedAt.Valid {
		summary.ArchivedAt = &row.archivedAt.Time
	}
	return summary, nil
}

func specToNamedSpecResolveMatch(row store.Spec) namedSpecResolveMatch {
	return namedSpecResolveMatch{
		id:                row.ID.String(),
		name:              row.Name,
		description:       row.Description,
		source:            row.Source,
		sha:               row.Sha,
		sourceCommittedAt: row.SourceCommittedAt,
		spec:              row.Spec,
		createdBy:         row.CreatedBy,
		updatedBy:         row.UpdatedBy,
		createdAt:         row.CreatedAt,
		archivedAt:        row.ArchivedAt,
	}
}

func decodeNamedSpecSource(raw []byte) (domainapi.NamedSpecSource, error) {
	var source domainapi.NamedSpecSource
	if len(raw) == 0 {
		return source, nil
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return domainapi.NamedSpecSource{}, err
	}
	return source, nil
}

func formatNamedSpecChoices(matches []namedSpecResolveMatch) string {
	choices := make([]string, 0, len(matches))
	for _, match := range matches {
		source, err := decodeNamedSpecSource(match.source)
		if err != nil {
			continue
		}
		choices = append(choices, source.Domain+"/"+source.Repo+":"+match.name+"@"+shortNamedSpecSHA(match.sha))
	}
	sort.Strings(choices)
	return strings.Join(choices, ", ")
}

func parseBoolQueryDefault(r *http.Request, name string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	switch raw {
	case "":
		return defaultValue, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", name)
	}
}

func stringPtrValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func shortNamedSpecSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}
