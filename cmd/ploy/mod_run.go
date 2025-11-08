package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	modplan "github.com/iw2rmb/ploy/internal/workflow/mods/plan"
	"gopkg.in/yaml.v3"
)

// handleModRun executes the Mods-specific run command.
func handleModRun(args []string, stderr io.Writer) error {
	return executeModRun(args, stderr)
}

// stringSlice is a simple flag.Value for collecting repeated values.
type stringSlice []string

func (s *stringSlice) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func executeModRun(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	specFile := fs.String("spec", "", "Path to YAML/JSON spec file")
	repoURL := fs.String("repo-url", "", "Git repository URL to materialise for Mods execution")
	repoBaseRef := fs.String("repo-base-ref", "", "Git base ref used for materialisation")
	repoTargetRef := fs.String("repo-target-ref", "", "Git target ref created for the run")
	repoWorkspace := fs.String("repo-workspace-hint", "", "Optional subdirectory hint when preparing the workspace")
	follow := fs.Bool("follow", false, "follow ticket events until completion")
	capDuration := fs.Duration("cap", 0, "optional overall time cap for --follow (e.g., 5m)")
	cancelOnCap := fs.Bool("cancel-on-cap", false, "when set with --cap, cancel the ticket if the cap is exceeded")
	artifactDir := fs.String("artifact-dir", "", "directory to download final artifacts into (with manifest.json)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON summary to stdout")
	maxRetries := fs.Int("max-retries", 5, "max reconnect attempts for event stream (-1 for unlimited)")
	retryWait := fs.Duration("retry-wait", 500*time.Millisecond, "wait between event stream reconnects")
	// Allow passing Mod env via repeated --mod-env KEY=VALUE
	var modEnvs stringSlice
	fs.Var(&modEnvs, "mod-env", "Mod environment KEY=VALUE (repeatable)")
	// Allow specifying the mod container image (paths fixed; image entrypoint runs)
	modImage := fs.String("mod-image", "", "Container image for the mod step (optional)")
	// Optional: retain container after run for inspection
	retain := fs.Bool("retain-container", false, "Retain the mod container after execution (for debugging)")
	// Optional: override container command (string, executed via sh -c on the node)
	modCommand := fs.String("mod-command", "", "Container command override (string or JSON array)")
	// GitLab MR flags (per-run overrides)
	gitlabPAT := fs.String("gitlab-pat", "", "GitLab Personal Access Token for this run (overrides server default)")
	gitlabDomain := fs.String("gitlab-domain", "", "GitLab domain for this run (overrides server default)")
	mrSuccess := fs.Bool("mr-success", false, "Create a merge request on success")
	mrFail := fs.Bool("mr-fail", false, "Create a merge request on failure")
	// DEPRECATED: --heal-on-build injects a default build_gate_healing when spec lacks it
	healOnBuild := fs.Bool("heal-on-build", false, "DEPRECATED: inject default build_gate_healing (use --spec with build_gate_healing instead)")

	if err := fs.Parse(args); err != nil {
		printModRunUsage(stderr)
		return err
	}

	repoSpec := struct {
		URL           string
		BaseRef       string
		TargetRef     string
		WorkspaceHint string
	}{
		URL:           strings.TrimSpace(*repoURL),
		BaseRef:       strings.TrimSpace(*repoBaseRef),
		TargetRef:     strings.TrimSpace(*repoTargetRef),
		WorkspaceHint: strings.TrimSpace(*repoWorkspace),
	}
	if repoSpec.URL != "" && repoSpec.TargetRef == "" {
		printModRunUsage(stderr)
		return fmt.Errorf("repo target ref required when repo url is set")
	}

	// Note: validation of GitLab flags is deferred to the server/node.
	// Avoid empty branches and keep CLI permissive for per-run overrides.

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	request := modsapi.TicketSubmitRequest{
		Submitter:  strings.TrimSpace(os.Getenv("USER")),
		Repository: repoSpec.URL,
		Metadata:   make(map[string]string),
		Stages:     defaultStageDefinitions(),
	}
	if repoSpec.BaseRef != "" {
		request.Metadata["repo_base_ref"] = repoSpec.BaseRef
	}
	if repoSpec.TargetRef != "" {
		request.Metadata["repo_target_ref"] = repoSpec.TargetRef
	}
	if repoSpec.WorkspaceHint != "" {
		request.Metadata["repo_workspace_hint"] = repoSpec.WorkspaceHint
	}

	// Load spec from file (if provided) and merge with CLI overrides.
	// CLI flags take precedence over spec file values.
	specPayload, err := buildSpecPayload(
		strings.TrimSpace(*specFile),
		modEnvs,
		strings.TrimSpace(*modImage),
		*retain,
		strings.TrimSpace(*modCommand),
		strings.TrimSpace(*gitlabPAT),
		strings.TrimSpace(*gitlabDomain),
		*mrSuccess,
		*mrFail,
		*healOnBuild,
	)
	if err != nil {
		return fmt.Errorf("build spec: %w", err)
	}

	cmd := mods.SubmitCommand{
		Client:  httpClient,
		BaseURL: base,
		Request: request,
		Spec:    specPayload,
	}
	summary, err := cmd.Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "Mods ticket %s submitted (state: %s)\n", summary.TicketID, summary.State)

	initialState := strings.ToLower(string(summary.State))
	finalState := ""

	if *follow {
		followCtx := ctx
		var cancel context.CancelFunc
		if *capDuration > 0 {
			followCtx, cancel = context.WithTimeout(ctx, *capDuration)
			defer cancel()
		}
		ev := mods.EventsCommand{
			Client: stream.Client{
				HTTPClient:   cloneForStream(httpClient),
				MaxRetries:   *maxRetries,
				RetryBackoff: *retryWait,
			},
			BaseURL: base,
			Ticket:  string(summary.TicketID),
			Output:  stderr,
		}
		final, err := ev.Run(followCtx)
		if err != nil {
			if *capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
				if *cancelOnCap {
					_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting ticket cancellation...")
					_ = mods.CancelCommand{BaseURL: base, Client: httpClient, Ticket: string(summary.TicketID), Reason: "cap exceeded", Output: stderr}.Run(context.Background())
				} else {
					_, _ = fmt.Fprintf(stderr, "Follow capped after %s; ticket %s continues running in the background.\n", capDuration.String(), summary.TicketID)
				}
				return nil
			}
			return err
		}
		_, _ = fmt.Fprintf(stderr, "Final state: %s\n", strings.ToLower(string(final)))
		if final != modsapi.TicketStateSucceeded {
			return fmt.Errorf("mod run ended in %s", strings.ToLower(string(final)))
		}
		finalState = strings.ToLower(string(final))
		if strings.TrimSpace(*artifactDir) != "" {
			if err := downloadTicketArtifacts(ctx, base, httpClient, string(summary.TicketID), strings.TrimSpace(*artifactDir), stderr); err != nil {
				return err
			}
		}
	}

	if *jsonOut {
		// Optional: probe MR URL from ticket status metadata.
		mrURL, _ := fetchMRURL(ctx, base, httpClient, string(summary.TicketID))
		type runJSON struct {
			TicketID    string `json:"ticket_id"`
			Initial     string `json:"initial_state,omitempty"`
			Final       string `json:"final_state,omitempty"`
			ArtifactDir string `json:"artifact_dir,omitempty"`
			MRURL       string `json:"mr_url,omitempty"`
		}
		out := runJSON{TicketID: string(summary.TicketID), Initial: initialState, Final: finalState}
		if s := strings.TrimSpace(*artifactDir); s != "" {
			out.ArtifactDir = s
		}
		if mrURL != "" {
			out.MRURL = mrURL
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
	}
	return nil
}

func printModRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run [--spec <file>] [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> --repo-workspace-hint <dir>] [--mod-env KEY=VALUE ...] [--mod-image <image>] [--mod-command <cmd>] [--retain-container] [--gitlab-pat <token>] [--gitlab-domain <domain>] [--mr-success] [--mr-fail] [--heal-on-build (deprecated)] [--follow] [--cap <duration>] [--artifact-dir <dir>] [--json] [--max-retries N] [--retry-wait D]")
}

// resolveEnvFromFile reads a file path (expanding ~), returns its content as a string.
// Redacts the content in any returned error messages for security.
func resolveEnvFromFile(path string) (string, error) {
	// Expand ~ to user home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir for path %s: %w", path, err)
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// Redact file path to avoid leaking sensitive locations
		return "", fmt.Errorf("read env file (path redacted): %w", err)
	}
	return string(data), nil
}

// resolveEnvFromFileInPlace processes env and env_from_file from a spec section,
// resolving file references and merging them into the env map in-place.
// Removes env_from_file after resolution. Supports both env_from_file map and inline {from_file: path} syntax.
func resolveEnvFromFileInPlace(spec map[string]any) error {
	// Prepare the merged env map
	mergedEnv := make(map[string]any)

	// First, process env_from_file if present
	if envFromFile, ok := spec["env_from_file"].(map[string]any); ok {
		for k, v := range envFromFile {
			path, ok := v.(string)
			if !ok {
				return fmt.Errorf("env_from_file[%s]: expected string path, got %T", k, v)
			}
			content, err := resolveEnvFromFile(path)
			if err != nil {
				return fmt.Errorf("env_from_file[%s]: %w", k, err)
			}
			mergedEnv[k] = content
		}
		// Remove env_from_file after processing
		delete(spec, "env_from_file")
	}

	// Then, process env (including inline {from_file: path} syntax)
	if env, ok := spec["env"].(map[string]any); ok {
		for k, v := range env {
			switch val := v.(type) {
			case string:
				// Direct string value
				mergedEnv[k] = val
			case map[string]any:
				// Check for {from_file: "path"} syntax
				if fromFile, ok := val["from_file"].(string); ok {
					content, err := resolveEnvFromFile(fromFile)
					if err != nil {
						return fmt.Errorf("env[%s].from_file: %w", k, err)
					}
					mergedEnv[k] = content
				} else {
					return fmt.Errorf("env[%s]: expected string or {from_file: path}, got unsupported map structure", k)
				}
			default:
				return fmt.Errorf("env[%s]: expected string or {from_file: path}, got %T", k, v)
			}
		}
	}

	// Update spec with merged env (only if we have any env values)
	if len(mergedEnv) > 0 {
		spec["env"] = mergedEnv
	}

	return nil
}

// buildSpecPayload loads a spec from file (YAML or JSON) and merges it with CLI flag overrides.
// CLI flags take precedence over spec file values. Returns raw JSON bytes.
func buildSpecPayload(
	specFile string,
	modEnvs []string,
	modImage string,
	retain bool,
	modCommand string,
	gitlabPAT string,
	gitlabDomain string,
	mrSuccess bool,
	mrFail bool,
	healOnBuild bool,
) ([]byte, error) {
	// Start with spec from file (if provided)
	var base map[string]any
	if specFile != "" {
		data, err := os.ReadFile(specFile)
		if err != nil {
			return nil, fmt.Errorf("read spec file %s: %w", specFile, err)
		}
		// Try JSON first, fallback to YAML
		if err := json.Unmarshal(data, &base); err != nil {
			// Not JSON; try YAML
			if err := yaml.Unmarshal(data, &base); err != nil {
				return nil, fmt.Errorf("parse spec file %s (not valid JSON or YAML): %w", specFile, err)
			}
		}
	} else {
		base = make(map[string]any)
	}

	// Resolve env_from_file references in the top-level spec (mod section)
	if err := resolveEnvFromFileInPlace(base); err != nil {
		return nil, fmt.Errorf("resolve env from file (mod): %w", err)
	}

	// Resolve env_from_file references in build_gate_healing.mods[] if present
	if healing, ok := base["build_gate_healing"].(map[string]any); ok {
		if mods, ok := healing["mods"].([]any); ok {
			for i, m := range mods {
				if modEntry, ok := m.(map[string]any); ok {
					if err := resolveEnvFromFileInPlace(modEntry); err != nil {
						return nil, fmt.Errorf("resolve env from file (build_gate_healing.mods[%d]): %w", i, err)
					}
				}
			}
		}
	}

	// Merge CLI flag overrides (CLI flags take precedence)
	hasOverrides := len(modEnvs) > 0 || modImage != "" || retain || modCommand != "" ||
		gitlabPAT != "" || gitlabDomain != "" || mrSuccess || mrFail || healOnBuild

	// Only proceed if we have a spec file or CLI overrides
	if len(base) == 0 && !hasOverrides {
		return nil, nil
	}

	// Apply CLI overrides to the base spec
	if len(modEnvs) > 0 {
		env := make(map[string]string)
		// Preserve existing env from spec file if present
		if existingEnv, ok := base["env"].(map[string]any); ok {
			for k, v := range existingEnv {
				if s, ok := v.(string); ok {
					env[k] = s
				}
			}
		}
		// Apply CLI overrides
		for _, kv := range modEnvs {
			kv = strings.TrimSpace(kv)
			if kv == "" {
				continue
			}
			var k, v string
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				k = strings.TrimSpace(kv[:idx])
				v = kv[idx+1:]
			} else {
				k = kv
				v = ""
			}
			if k != "" {
				env[k] = v
			}
		}
		if len(env) > 0 {
			base["env"] = env
		}
	}

	if modImage != "" {
		base["image"] = modImage
	}

	if retain {
		base["retain_container"] = true
	}

	if modCommand != "" {
		// Allow JSON array for command to pass argv directly to containers with ENTRYPOINT.
		// Fallback to shell string (wrapped as ["/bin/sh","-c",cmd]) when not a JSON array.
		var asArray []string
		if strings.HasPrefix(modCommand, "[") && strings.HasSuffix(modCommand, "]") {
			if err := json.Unmarshal([]byte(modCommand), &asArray); err == nil && len(asArray) > 0 {
				base["command"] = asArray
			} else {
				base["command"] = modCommand
			}
		} else {
			base["command"] = modCommand
		}
	}

	// Add GitLab options (never print PAT in logs; node agent will handle redaction)
	if gitlabPAT != "" {
		base["gitlab_pat"] = gitlabPAT
	}
	if gitlabDomain != "" {
		base["gitlab_domain"] = gitlabDomain
	}
	if mrSuccess {
		base["mr_on_success"] = true
	}
	if mrFail {
		base["mr_on_fail"] = true
	}

	// DEPRECATED: --heal-on-build injects a default build_gate_healing when spec lacks it.
	// This is a back-compat shim kept for one release cycle.
	if healOnBuild {
		if _, exists := base["build_gate_healing"]; !exists {
			base["build_gate_healing"] = map[string]any{
				"retries": 1,
				"mods":    []any{},
			}
		}
	}

	if len(base) == 0 {
		return nil, nil
	}

	// Marshal to JSON for submission
	return json.Marshal(base)
}

// (stringSlice implements flag.Value above)

func defaultStageDefinitions() []modsapi.StageDefinition {
	return []modsapi.StageDefinition{
		{ID: domaintypes.StageID(modplan.StageNamePlan), Lane: "mods-plan", Priority: "default", MaxAttempts: 1},
		{ID: domaintypes.StageID(modplan.StageNameORWApply), Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNamePlan)}},
		{ID: domaintypes.StageID(modplan.StageNameORWGenerate), Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNamePlan)}},
		{ID: domaintypes.StageID(modplan.StageNameLLMPlan), Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNamePlan)}},
		{ID: domaintypes.StageID(modplan.StageNameLLMExec), Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []domaintypes.StageID{domaintypes.StageID(modplan.StageNameORWApply), domaintypes.StageID(modplan.StageNameORWGenerate), domaintypes.StageID(modplan.StageNameLLMPlan)}},
	}
}

// downloadTicketArtifacts fetches ticket status and downloads referenced artifacts into dir.
func downloadTicketArtifacts(ctx context.Context, base *url.URL, httpClient *http.Client, ticketID, dir string, out io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir %s: %w", dir, err)
	}
	// Fetch ticket status
	statusURL, err := url.JoinPath(base.String(), "v1", "mods", url.PathEscape(strings.TrimSpace(ticketID)))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return fmt.Errorf("build status request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch ticket status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return controlPlaneHTTPError(resp)
	}
	var payload modsapi.TicketStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode ticket status: %w", err)
	}
	// Collect artifacts via control-plane HTTP
	type manifestItem struct {
		Stage  string `json:"stage"`
		Name   string `json:"name"`
		CID    string `json:"cid"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
		Path   string `json:"path"`
	}
	items := make([]manifestItem, 0)
	var downloaded int
	for stageID, st := range payload.Ticket.Stages {
		for name, cid := range st.Artifacts {
			// Lookup artifact by CID
			lookupURL, err := url.Parse(base.String())
			if err != nil {
				return err
			}
			lookupURL.Path, err = url.JoinPath(lookupURL.Path, "v1", "artifacts")
			if err != nil {
				return err
			}
			q := lookupURL.Query()
			q.Set("cid", cid)
			lookupURL.RawQuery = q.Encode()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL.String(), nil)
			if err != nil {
				return err
			}
			lr, err := httpClient.Do(req)
			if err != nil {
				return err
			}
			var listing struct {
				Artifacts []struct {
					ID, CID, Digest, Name string
					Size                  int64
				} `json:"artifacts"`
			}
			if lr.StatusCode != http.StatusOK {
				_ = lr.Body.Close()
				return controlPlaneHTTPError(lr)
			}
			if err := json.NewDecoder(lr.Body).Decode(&listing); err != nil {
				_ = lr.Body.Close()
				return fmt.Errorf("decode artifact listing: %w", err)
			}
			_ = lr.Body.Close()
			if len(listing.Artifacts) == 0 {
				return fmt.Errorf("no artifact found for CID %s", cid)
			}
			art := listing.Artifacts[0]
			// Download content
			dlURL, err := url.Parse(base.String())
			if err != nil {
				return err
			}
			dlURL.Path, err = url.JoinPath(dlURL.Path, "v1", "artifacts", url.PathEscape(art.ID))
			if err != nil {
				return err
			}
			q2 := dlURL.Query()
			q2.Set("download", "true")
			dlURL.RawQuery = q2.Encode()
			dreq, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL.String(), nil)
			if err != nil {
				return err
			}
			dresp, err := httpClient.Do(dreq)
			if err != nil {
				return err
			}
			if dresp.StatusCode != http.StatusOK {
				_ = dresp.Body.Close()
				return controlPlaneHTTPError(dresp)
			}
			filename := buildArtifactFilename(stageID, name, cid, art.Digest)
			path := filepath.Join(dir, filename)
			data, _ := io.ReadAll(dresp.Body)
			_ = dresp.Body.Close()
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return fmt.Errorf("write artifact %s: %w", filename, err)
			}
			items = append(items, manifestItem{Stage: stageID, Name: name, CID: cid, Digest: art.Digest, Size: int64(len(data)), Path: path})
			downloaded++
		}
	}
	// Write manifest
	manifestPath := filepath.Join(dir, "manifest.json")
	data, _ := json.MarshalIndent(struct {
		Artifacts []manifestItem `json:"artifacts"`
	}{Artifacts: items}, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Downloaded %d artifacts to %s\n", downloaded, dir)
	return nil
}

func buildArtifactFilename(stage, name, cid, digest string) string {
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, "\\", "_")
		return s
	}
	stage = clean(stage)
	name = clean(name)
	cid = clean(cid)
	if d := strings.TrimSpace(digest); d != "" {
		d = strings.ReplaceAll(d, ":", "-")
		if len(d) > 20 {
			d = d[:20]
		}
		return fmt.Sprintf("%s_%s_%s.bin", d, stage, name)
	}
	return fmt.Sprintf("%s_%s_%s.bin", cid, stage, name)
}

// fetchMRURL loads the ticket status and extracts the MR URL from metadata when present.
func fetchMRURL(ctx context.Context, base *url.URL, httpClient *http.Client, ticketID string) (string, error) {
	statusURL, err := url.JoinPath(base.String(), "v1", "mods", url.PathEscape(strings.TrimSpace(ticketID)))
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", controlPlaneHTTPError(resp)
	}
	var payload modsapi.TicketStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Ticket.Metadata != nil {
		if v, ok := payload.Ticket.Metadata["mr_url"]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), nil
		}
	}
	return "", nil
}
