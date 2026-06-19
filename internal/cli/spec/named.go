package spec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/specpayload"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"gopkg.in/yaml.v3"
)

type namedSpecProbe struct {
	APIVersion  string `yaml:"apiVersion"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type gitSpecSource struct {
	Worktree    string
	SHA         string
	CommittedAt time.Time
	Domain      string
	Repo        string
}

func handlePush(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printPushUsage(stderr)
		return nil
	}
	if len(args) > 1 {
		printPushUsage(stderr)
		return errors.New("spec push takes at most one git folder")
	}

	gitFolder := "."
	if len(args) == 1 {
		gitFolder = strings.TrimSpace(args[0])
	}
	if gitFolder == "" {
		return errors.New("git folder required")
	}

	ctx := context.Background()
	source, err := resolveGitSpecSource(ctx, gitFolder)
	if err != nil {
		return err
	}

	matches, err := discoverNamedSpecs(source.Worktree)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		_, _ = fmt.Fprintln(stdout, "No named specs found")
		return nil
	}

	base, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	results := make([]domainapi.NamedSpecSummary, 0, len(matches))
	for _, match := range matches {
		specJSON, err := specpayload.BuildSelected(ctx, base, client, match.path, "", nil, "", false, "")
		if err != nil {
			return fmt.Errorf("%s: load spec: %w", match.path, err)
		}
		if len(specJSON) == 0 {
			return fmt.Errorf("%s: spec is empty", match.path)
		}
		result, err := publishNamedSpec(ctx, base, client, domainapi.PublishNamedSpecRequest{
			Name:              match.probe.Name,
			Description:       match.probe.Description,
			Source:            domainapi.NamedSpecSource{Domain: source.Domain, Repo: source.Repo},
			SHA:               source.SHA,
			SourceCommittedAt: source.CommittedAt,
			Spec:              json.RawMessage(specJSON),
			CreatedBy:         currentUserPtr(),
		})
		if err != nil {
			return fmt.Errorf("%s: %w", match.path, err)
		}
		results = append(results, result)
	}

	renderPushResults(stdout, results)
	return nil
}

func printPushUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy spec push [<git-folder>]")
}

func handleList(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printListUsage(stderr)
		return nil
	}
	if len(args) > 0 {
		printListUsage(stderr)
		return errors.New("spec ls takes no arguments")
	}

	ctx := context.Background()
	base, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	specs, err := listNamedSpecs(ctx, base, client)
	if err != nil {
		return err
	}
	renderListResults(stdout, specs)
	return nil
}

func printListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy spec ls")
}

type discoveredNamedSpec struct {
	path  string
	probe namedSpecProbe
}

func discoverNamedSpecs(worktree string) ([]discoveredNamedSpec, error) {
	var matches []discoveredNamedSpec
	if err := filepath.WalkDir(worktree, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".yaml" {
			return nil
		}
		probe, ok := probeNamedSpecFile(path)
		if !ok {
			return nil
		}
		matches = append(matches, discoveredNamedSpec{path: path, probe: probe})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("discover named specs: %w", err)
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].path < matches[j].path
	})
	return matches, nil
}

func probeNamedSpecFile(path string) (namedSpecProbe, bool) {
	data, err := common.ReadFileRooted(path)
	if err != nil {
		return namedSpecProbe{}, false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return namedSpecProbe{}, false
	}
	root := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return namedSpecProbe{}, false
	}
	var probe namedSpecProbe
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		value := root.Content[i+1]
		if key.Kind != yaml.ScalarNode || value.Kind != yaml.ScalarNode {
			continue
		}
		switch key.Value {
		case "apiVersion":
			probe.APIVersion = value.Value
		case "name":
			probe.Name = value.Value
		case "description":
			probe.Description = value.Value
		}
	}
	probe.APIVersion = strings.TrimSpace(probe.APIVersion)
	probe.Name = strings.TrimSpace(probe.Name)
	probe.Description = strings.TrimSpace(probe.Description)
	if probe.APIVersion != "ploy.mig/v1alpha1" || probe.Name == "" {
		return namedSpecProbe{}, false
	}
	return probe, true
}

func resolveGitSpecSource(ctx context.Context, gitFolder string) (gitSpecSource, error) {
	worktree, err := gitOutput(ctx, gitFolder, "rev-parse", "--show-toplevel")
	if err != nil {
		return gitSpecSource{}, err
	}
	worktree = strings.TrimSpace(worktree)
	if worktree == "" {
		return gitSpecSource{}, errors.New("git worktree root is empty")
	}
	status, err := gitOutput(ctx, worktree, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return gitSpecSource{}, err
	}
	if strings.TrimSpace(status) != "" {
		return gitSpecSource{}, errors.New("git worktree must be clean before publishing specs")
	}
	sha, err := gitOutput(ctx, worktree, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return gitSpecSource{}, err
	}
	sha = strings.TrimSpace(sha)
	commitDate, err := gitOutput(ctx, worktree, "show", "-s", "--format=%cI", "HEAD")
	if err != nil {
		return gitSpecSource{}, err
	}
	committedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(commitDate))
	if err != nil {
		return gitSpecSource{}, fmt.Errorf("parse HEAD commit date: %w", err)
	}
	origin, err := gitOutput(ctx, worktree, "remote", "get-url", "origin")
	if err != nil {
		return gitSpecSource{}, err
	}
	domain, repo, err := parseNamedSpecSourceOrigin(origin)
	if err != nil {
		return gitSpecSource{}, err
	}
	return gitSpecSource{Worktree: worktree, SHA: sha, CommittedAt: committedAt, Domain: domain, Repo: repo}, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), detail)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func parseNamedSpecSourceOrigin(rawOrigin string) (string, string, error) {
	normalized := domaintypes.NormalizeRepoURLSchemless(rawOrigin)
	normalized = strings.Trim(normalized, "/")
	parts := strings.Split(normalized, "/")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("origin remote must normalize to domain/namespace/repo, got %q", normalized)
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", "", fmt.Errorf("origin remote contains an empty path component: %q", normalized)
		}
	}
	domain := parts[0]
	repo := strings.Join(parts[1:], "/")
	if strings.ContainsAny(domain, ":@") || strings.Contains(domain, "..") {
		return "", "", fmt.Errorf("origin remote domain is invalid: %q", domain)
	}
	if strings.ContainsAny(repo, ":@") || strings.Contains(repo, "//") {
		return "", "", fmt.Errorf("origin remote repo is invalid: %q", repo)
	}
	return domain, repo, nil
}

func publishNamedSpec(ctx context.Context, base *url.URL, client *http.Client, request domainapi.PublishNamedSpecRequest) (domainapi.NamedSpecSummary, error) {
	endpoint := base.JoinPath("v1", "specs")
	body, err := json.Marshal(request)
	if err != nil {
		return domainapi.NamedSpecSummary{}, fmt.Errorf("publish named spec: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return domainapi.NamedSpecSummary{}, fmt.Errorf("publish named spec: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return domainapi.NamedSpecSummary{}, fmt.Errorf("publish named spec: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return domainapi.NamedSpecSummary{}, common.ControlPlaneHTTPError(resp)
	}
	defer func() { _ = resp.Body.Close() }()
	var summary domainapi.NamedSpecSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return domainapi.NamedSpecSummary{}, fmt.Errorf("publish named spec: decode response: %w", err)
	}
	return summary, nil
}

func listNamedSpecs(ctx context.Context, base *url.URL, client *http.Client) ([]domainapi.NamedSpecSummary, error) {
	endpoint := base.JoinPath("v1", "specs")
	q := endpoint.Query()
	q.Set("named", "true")
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("list named specs: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list named specs: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, common.ControlPlaneHTTPError(resp)
	}
	defer func() { _ = resp.Body.Close() }()
	var list domainapi.NamedSpecListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("list named specs: decode response: %w", err)
	}
	return list.Specs, nil
}

func renderPushResults(out io.Writer, specs []domainapi.NamedSpecSummary) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "STATE\tNAME\tSOURCE\tSHA\tDATE")
	for _, spec := range specs {
		state := "updated"
		if spec.Skipped {
			state = "skipped"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", state, spec.Name, renderNamedSpecSource(spec.Source), shortSHA(spec.SHA), formatSpecTime(spec.SourceCommittedAt))
	}
	_ = w.Flush()
}

func renderListResults(out io.Writer, specs []domainapi.NamedSpecSummary) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSOURCE\tSHA\tDATE")
	for _, spec := range specs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", spec.Name, renderNamedSpecSource(spec.Source), shortSHA(spec.SHA), formatSpecTime(spec.SourceCommittedAt))
	}
	_ = w.Flush()
}

func renderNamedSpecSource(source domainapi.NamedSpecSource) string {
	if source.Domain == "" {
		return source.Repo
	}
	if source.Repo == "" {
		return source.Domain
	}
	return source.Domain + "/" + source.Repo
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

func formatSpecTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func currentUserPtr() *string {
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		return nil
	}
	return &user
}
