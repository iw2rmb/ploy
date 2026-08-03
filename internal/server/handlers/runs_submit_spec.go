package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/speccatalog"
	"github.com/iw2rmb/ploy/internal/speccompiler"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type runSubmitSpecServices struct {
	catalog specCatalogResolver
	bundles speccompiler.BundleStore
}

type specCatalogResolver interface {
	WithResolvedSource(context.Context, string, func(speccatalog.Entry, string) error) error
}

type runSubmissionSpec struct {
	canonical   json.RawMessage
	name        string
	description string
	source      gitSpecSnapshotSource
	sha         string
	committedAt time.Time
}

type gitSpecSnapshotSource struct {
	Domain string `json:"domain"`
	Repo   string `json:"repo"`
	URL    string `json:"url"`
	Path   string `json:"path"`
}

func (s runSubmissionSpec) named() bool {
	return s.name != ""
}

type namedSpecCompilationError struct {
	entry speccatalog.Entry
	err   error
}

func (e *namedSpecCompilationError) Error() string {
	return fmt.Sprintf("compile named spec %s:%s at %s: %v", e.entry.Source, e.entry.Path, e.entry.SHA, e.err)
}

func (e *namedSpecCompilationError) Unwrap() error { return e.err }

func writeRunSubmissionSpecError(w http.ResponseWriter, err error) {
	var invalid *speccatalog.InvalidSelectorError
	var ambiguous *speccatalog.AmbiguousError
	var compilation *namedSpecCompilationError
	switch {
	case errors.As(err, &invalid):
		writeHTTPError(w, http.StatusBadRequest, "%s", err)
	case errors.Is(err, speccatalog.ErrNotFound):
		writeHTTPError(w, http.StatusNotFound, "%s", err)
	case errors.As(err, &ambiguous):
		writeHTTPError(w, http.StatusConflict, "%s", err)
	case errors.As(err, &compilation):
		writeHTTPError(w, http.StatusBadRequest, "%s", err)
	default:
		writeHTTPError(w, http.StatusServiceUnavailable, "%s", err)
	}
}

func validateRunSubmissionSpecRequest(req domainapi.RunSubmitRequest) error {
	hasSpec := len(bytes.TrimSpace(req.Spec)) > 0 && !bytes.Equal(bytes.TrimSpace(req.Spec), []byte("null"))
	selector := strings.TrimSpace(req.SpecSelector)
	hasSelector := selector != ""
	if hasSpec == hasSelector {
		return errors.New("exactly one of spec or spec_selector is required")
	}
	if hasSpec {
		if req.SpecOverrides != nil {
			return errors.New("spec_overrides requires spec_selector")
		}
		if _, err := contracts.ParseMigSpecJSON(req.Spec); err != nil {
			return fmt.Errorf("spec: %w", err)
		}
	}
	return nil
}

func resolveRunSubmissionSpec(ctx context.Context, req domainapi.RunSubmitRequest, services runSubmitSpecServices) (runSubmissionSpec, error) {
	if err := validateRunSubmissionSpecRequest(req); err != nil {
		return runSubmissionSpec{}, err
	}
	if len(bytes.TrimSpace(req.Spec)) > 0 && !bytes.Equal(bytes.TrimSpace(req.Spec), []byte("null")) {
		return runSubmissionSpec{canonical: req.Spec}, nil
	}
	selector := strings.TrimSpace(req.SpecSelector)
	if services.catalog == nil {
		return runSubmissionSpec{}, errors.New("named spec catalog is unavailable")
	}

	var result runSubmissionSpec
	err := services.catalog.WithResolvedSource(ctx, selector, func(entry speccatalog.Entry, repositoryRoot string) error {
		source, err := speccompiler.NewRepositorySource(repositoryRoot)
		if err != nil {
			return &namedSpecCompilationError{entry: entry, err: err}
		}
		defer func() { _ = source.Close() }()
		compiler, err := speccompiler.New(speccompiler.Options{Source: source, Bundles: services.bundles})
		if err != nil {
			return &namedSpecCompilationError{entry: entry, err: err}
		}
		canonical, err := compiler.Build(ctx, filepath.FromSlash(entry.Path), speccompiler.Overrides{})
		if err != nil {
			return &namedSpecCompilationError{entry: entry, err: err}
		}
		canonical, err = applyNamedRunSpecOverrides(canonical, req.SpecOverrides)
		if err != nil {
			return &namedSpecCompilationError{entry: entry, err: err}
		}
		domain, repo, err := namedSpecSourceParts(entry.Source)
		if err != nil {
			return &namedSpecCompilationError{entry: entry, err: err}
		}
		result = runSubmissionSpec{
			canonical:   canonical,
			name:        entry.Name,
			description: entry.Description,
			source: gitSpecSnapshotSource{
				Domain: domain,
				Repo:   repo,
				URL:    entry.Source,
				Path:   entry.Path,
			},
			sha:         entry.SHA,
			committedAt: entry.CommittedAt,
		}
		return nil
	})
	return result, err
}

func applyNamedRunSpecOverrides(canonical json.RawMessage, overrides *domainapi.RunSpecOverrides) (json.RawMessage, error) {
	if overrides == nil {
		return canonical, nil
	}
	var err error
	if len(overrides.StepEnvs) > 0 {
		for step, assignments := range overrides.StepEnvs {
			if assignments == nil {
				return nil, fmt.Errorf("spec_overrides.step_envs.%s must be an array", step)
			}
		}
		canonical, err = speccompiler.ApplyStepEnvOverrides(canonical, overrides.StepEnvs)
		if err != nil {
			return nil, err
		}
	}
	if overrides.BuildGateForced != nil {
		forced, err := buildGateForcedCompilerOverrides(*overrides.BuildGateForced)
		if err != nil {
			return nil, err
		}
		if forced.HasAny() {
			canonical, err = speccompiler.ApplyBuildGateForcedOverrides(canonical, forced)
			if err != nil {
				return nil, err
			}
		}
	}
	return canonical, nil
}

func buildGateForcedCompilerOverrides(raw domainapi.RunBuildGateForcedOverrides) (speccompiler.BuildGateForcedOverrides, error) {
	convert := func(phase string, stack *domainapi.RunBuildGateForcedStack) (*contracts.BuildGateStackConfig, error) {
		if stack == nil {
			return nil, nil
		}
		language := strings.TrimSpace(stack.Language)
		release := strings.TrimSpace(stack.Release)
		tool := strings.TrimSpace(stack.Tool)
		if language == "" || release == "" {
			return nil, fmt.Errorf("spec_overrides.build_gate_forced.%s requires language and release", phase)
		}
		return &contracts.BuildGateStackConfig{
			Mode:     contracts.BuildGateStackModeForced,
			Language: language,
			Release:  release,
			Tool:     tool,
		}, nil
	}
	pre, err := convert("pre", raw.Pre)
	if err != nil {
		return speccompiler.BuildGateForcedOverrides{}, err
	}
	post, err := convert("post", raw.Post)
	if err != nil {
		return speccompiler.BuildGateForcedOverrides{}, err
	}
	return speccompiler.BuildGateForcedOverrides{Pre: pre, Post: post}, nil
}

func namedSpecSourceParts(source string) (string, string, error) {
	parts := strings.Split(strings.Trim(domaintypes.NormalizeRepoURLSchemless(source), "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid named spec source %q", source)
	}
	return parts[0], strings.Join(parts[1:], "/"), nil
}

func persistRunSubmissionSpec(ctx context.Context, st store.Store, spec runSubmissionSpec, createdBy *string) (domaintypes.SpecID, error) {
	if !spec.named() {
		created, err := st.CreateSpec(ctx, store.CreateSpecParams{
			ID:        domaintypes.NewSpecID(),
			Name:      "",
			Spec:      spec.canonical,
			CreatedBy: createdBy,
		})
		return created.ID, err
	}

	source, err := json.Marshal(spec.source)
	if err != nil {
		return "", fmt.Errorf("marshal named spec source: %w", err)
	}
	lookup := store.GetGitSpecSnapshotParams{
		Name: spec.name, Domain: spec.source.Domain, Repo: spec.source.Repo, Path: spec.source.Path, Sha: spec.sha, Spec: spec.canonical,
	}
	if existing, err := st.GetGitSpecSnapshot(ctx, lookup); err == nil {
		return existing.ID, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup Git spec snapshot: %w", err)
	}

	created, err := st.CreateGitSpecSnapshot(ctx, store.CreateGitSpecSnapshotParams{
		ID:                domaintypes.NewSpecID(),
		Name:              spec.name,
		Description:       spec.description,
		Source:            source,
		Sha:               spec.sha,
		SourceCommittedAt: pgtype.Timestamptz{Time: spec.committedAt, Valid: true},
		Spec:              spec.canonical,
		CreatedBy:         createdBy,
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}
