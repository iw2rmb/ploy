package stepworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/controlplane"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/node/jobs"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/node/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
    javaexec "github.com/iw2rmb/ploy/internal/workflow/buildgate/javaexec"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	stepruntime "github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

const (
    envClusterURL     = "PLOY_IPFS_CLUSTER_API"
    envClusterToken   = "PLOY_IPFS_CLUSTER_TOKEN"
    envClusterUser    = "PLOY_IPFS_CLUSTER_USERNAME"
    envClusterPass    = "PLOY_IPFS_CLUSTER_PASSWORD"
    envClusterReplMin = "PLOY_IPFS_CLUSTER_REPL_MIN"
    envClusterReplMax = "PLOY_IPFS_CLUSTER_REPL_MAX"
    envClusterLocal   = "PLOY_IPFS_CLUSTER_LOCAL"
)

// stepRunner abstracts the step runtime runner for testability.
type stepRunner interface {
	Run(ctx context.Context, req stepruntime.Request) (stepruntime.Result, error)
}

// Executor executes control-plane assignments using the step runner pipeline.
type Executor struct {
    runner  stepRunner
    streams *logstream.Hub
    jobs    *jobs.Store
    now     func() time.Time
}

// Options configures the executor.
type Options struct {
    Runner  stepRunner
    Streams *logstream.Hub
    Jobs    *jobs.Store
    Clock   func() time.Time
}

// New constructs an executor from the provided options.
func New(opts Options) (*Executor, error) {
	if opts.Runner == nil {
		return nil, errors.New("stepworker: runner required")
	}
	if opts.Streams == nil {
		return nil, errors.New("stepworker: log streams required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
    return &Executor{
        runner:  opts.Runner,
        streams: opts.Streams,
        jobs:    opts.Jobs,
        now:     clock,
    }, nil
}

// FromConfig assembles a step executor using daemon configuration.
func FromConfig(cfg config.Config, streams *logstream.Hub, httpClient *http.Client, jobStore *jobs.Store) (*Executor, error) {
	clusterClient, err := newClusterClient(cfg)
	if err != nil {
		return nil, err
	}

	artifactFetcher := &clusterFetcher{client: clusterClient}

    // Prefer local-only pinning when explicitly requested to reduce
    // cross-peer failures in unstable clusters.
    localPin := strings.EqualFold(resolveConfigString(cfg, envClusterLocal), "1") ||
        strings.EqualFold(resolveConfigString(cfg, envClusterLocal), "true")

    publisher, err := artifacts.NewClusterPublisher(artifacts.ClusterPublisherOptions{
        Client: clusterClient,
        Local:  localPin,
    })
    if err != nil {
        return nil, fmt.Errorf("stepworker: cluster publisher: %w", err)
    }

    // Allow disabling artifact publication entirely (diff/log/gate) when the
    // backing store is unhealthy. When disabled, artifacts are not uploaded
    // and runs rely solely on live logs.
    publishArtifacts := true
    if v := strings.TrimSpace(resolveConfigString(cfg, "PLOY_ARTIFACT_PUBLISH")); v != "" {
        publishArtifacts = strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
    }
    var artifactPublisher stepruntime.ArtifactPublisher
    if publishArtifacts {
        artifactPublisher = publisher
    }

	var tokenSource hydration.TokenSource
	if httpClient != nil {
		baseEndpoint := strings.TrimSpace(cfg.ControlPlane.Endpoint)
		nodeID := strings.TrimSpace(cfg.ControlPlane.NodeID)
		if baseEndpoint != "" && nodeID != "" {
			src, err := hydration.NewSignerTokenSource(hydration.SignerTokenSourceOptions{
				BaseURL:    baseEndpoint,
				NodeID:     nodeID,
				HTTPClient: httpClient,
			})
			if err != nil {
				return nil, fmt.Errorf("stepworker: gitlab token source: %w", err)
			}
			tokenSource = src
		}
	}

    // Allow turning off snapshot publish to work around unstable IPFS clusters.
    publishSnapshots := true
    if v := strings.TrimSpace(resolveConfigString(cfg, "PLOY_HYDRATION_PUBLISH_SNAPSHOT")); v != "" {
        publishSnapshots = strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
    }
    repoFetcher, err := hydration.NewGitFetcher(hydration.GitFetcherOptions{
        Publisher:       publisher,
        TokenSource:     tokenSource,
        PublishSnapshot: publishSnapshots,
    })
	if err != nil {
		return nil, fmt.Errorf("stepworker: git fetcher: %w", err)
	}

	hydrator, err := stepruntime.NewFilesystemWorkspaceHydrator(stepruntime.FilesystemWorkspaceHydratorOptions{
		ArtifactRoot: strings.TrimSpace(cfg.Worker.ArtifactDir),
		Fetcher:      artifactFetcher,
		RepoFetcher:  repoFetcher,
	})
	if err != nil {
		return nil, fmt.Errorf("stepworker: workspace hydrator: %w", err)
	}

	diffGen := stepruntime.NewFilesystemDiffGenerator(stepruntime.FilesystemDiffGeneratorOptions{})

	containerRuntime, err := stepruntime.NewDockerContainerRuntime(stepruntime.DockerContainerRuntimeOptions{
		PullImage: true,
	})
	if err != nil {
		return nil, fmt.Errorf("stepworker: docker runtime: %w", err)
	}

    gateClient, err := newGateClient()
	if err != nil {
		return nil, err
	}

    runner := &stepruntime.Runner{
        Workspace:  hydrator,
        Containers: containerRuntime,
        Diffs:      diffGen,
        Gate:       gateClient,
        Artifacts:  artifactPublisher,
        Streams:    streams,
    }

    return New(Options{
        Runner:  runner,
        Streams: streams,
        Jobs:    jobStore,
    })
}

// Execute runs the assignment manifest through the step runtime.
func (e *Executor) Execute(ctx context.Context, assignment controlplane.Assignment) (controlplane.AssignmentResult, error) {
	var zero controlplane.AssignmentResult
	if e == nil || e.runner == nil {
		return zero, errors.New("stepworker: executor not configured")
	}

	manifest, err := decodeManifest(assignment.Metadata["step_manifest"])
	if err != nil {
		return zero, err
	}

	streamID := strings.TrimSpace(assignment.ID)
	if streamID == "" {
		return zero, errors.New("stepworker: assignment id required")
	}
    if e.streams != nil {
        e.streams.Ensure(streamID)
    }

    if e.jobs != nil {
        e.jobs.Start(streamID)
    }

    req := stepruntime.Request{
		Manifest:    manifest,
		LogStreamID: streamID,
	}

    // Mirror job logs into the Mods ticket stream when available so
    // `/v1/mods/{ticket}/logs/stream` can aggregate without extra plumbing.
    result, runErr := func() (stepruntime.Result, error) {
        if r, ok := e.runner.(*stepruntime.Runner); ok && strings.TrimSpace(assignment.Ticket) != "" && e.streams != nil {
            // Shallow copy the runner and override the Streams publisher
            // with a dual publisher that forwards to both job and ticket streams.
            cloned := *r
            cloned.Streams = dualPublisher{hub: e.streams, ticket: strings.TrimSpace(assignment.Ticket)}
            return cloned.Run(ctx, req)
        }
        return e.runner.Run(ctx, req)
    }()

    assignmentResult := e.buildResult(manifest, result, runErr)

    if e.jobs != nil {
        state := jobs.StateSucceeded
        if runErr != nil {
            state = jobs.StateFailed
        }
        msg := ""
        if runErr != nil {
            msg = runErr.Error()
        }
        e.jobs.Complete(streamID, state, msg)
    }
    return assignmentResult, runErr
}

// dualPublisher mirrors logstream events to the job stream (as requested)
// and to the Mods ticket stream, enabling aggregated ticket-level tails.
type dualPublisher struct {
    hub    *logstream.Hub
    ticket string
}

func (d dualPublisher) PublishLog(ctx context.Context, streamID string, record logstream.LogRecord) error {
    if d.hub == nil {
        return nil
    }
    _ = d.hub.PublishLog(ctx, streamID, record)
    if t := strings.TrimSpace(d.ticket); t != "" {
        _ = d.hub.PublishLog(ctx, t, record)
    }
    return nil
}

func (d dualPublisher) PublishRetention(ctx context.Context, streamID string, hint logstream.RetentionHint) error {
    if d.hub == nil {
        return nil
    }
    _ = d.hub.PublishRetention(ctx, streamID, hint)
    if t := strings.TrimSpace(d.ticket); t != "" {
        _ = d.hub.PublishRetention(ctx, t, hint)
    }
    return nil
}

func (d dualPublisher) PublishStatus(ctx context.Context, streamID string, status logstream.Status) error {
    if d.hub == nil {
        return nil
    }
    _ = d.hub.PublishStatus(ctx, streamID, status)
    if t := strings.TrimSpace(d.ticket); t != "" {
        _ = d.hub.PublishStatus(ctx, t, status)
    }
    return nil
}

// buildResult converts the step runtime result into an assignment result payload.
func (e *Executor) buildResult(manifest contracts.StepManifest, runResult stepruntime.Result, runErr error) controlplane.AssignmentResult {
	now := e.now().UTC()
	state := string(scheduler.JobStateSucceeded)
	if runErr != nil {
		state = string(scheduler.JobStateFailed)
	}

	artifacts := make(map[string]string)
	if cid := strings.TrimSpace(runResult.DiffArtifact.CID); cid != "" {
		artifacts["diff_cid"] = cid
		artifacts["diff_digest"] = strings.TrimSpace(runResult.DiffArtifact.Digest)
	}
	if cid := strings.TrimSpace(runResult.LogArtifact.CID); cid != "" {
		artifacts["logs_cid"] = cid
		artifacts["logs_digest"] = strings.TrimSpace(runResult.LogArtifact.Digest)
	}
	if container := strings.TrimSpace(runResult.ContainerID); container != "" {
		artifacts["container_id"] = container
	}

	bundles := make(map[string]scheduler.BundleRecord)
	if artifact, ok := firstHydrationSnapshot(runResult.HydrationSnapshots); ok {
		cid := strings.TrimSpace(artifact.CID)
		if cid != "" {
			artifacts[scheduler.HydrationSnapshotCIDKey] = cid
			if digest := strings.TrimSpace(artifact.Digest); digest != "" {
				artifacts[scheduler.HydrationSnapshotDigestKey] = digest
			}
			if artifact.Size > 0 {
				artifacts[scheduler.HydrationSnapshotSizeKey] = strconv.FormatInt(artifact.Size, 10)
			}
			ttl := scheduler.HydrationSnapshotTTL
			bundle := scheduler.BundleRecord{
				CID:      cid,
				Digest:   strings.TrimSpace(artifact.Digest),
				Size:     artifact.Size,
				Retained: true,
				TTL:      ttl,
			}
			if duration, err := time.ParseDuration(ttl); err == nil && duration > 0 {
				bundle.ExpiresAt = now.Add(duration).UTC().Format(time.RFC3339Nano)
			}
			bundles[scheduler.HydrationSnapshotBundleKey] = bundle
		}
	}
	logTTL := firstNonEmpty(runResult.RetentionTTL, manifest.Retention.TTL)
	if cid := strings.TrimSpace(runResult.LogArtifact.CID); cid != "" {
		bundles["logs"] = buildBundleRecord(runResult.LogArtifact, logTTL, manifest.Retention.RetainContainer, now)
	}
	if cid := strings.TrimSpace(runResult.DiffArtifact.CID); cid != "" {
		record := scheduler.BundleRecord{
			CID:    cid,
			Digest: strings.TrimSpace(runResult.DiffArtifact.Digest),
			Size:   runResult.DiffArtifact.Size,
		}
		bundles["diff"] = record
	}
        if cid := strings.TrimSpace(runResult.GateArtifact.CID); cid != "" {
            artifacts["gate_report_cid"] = cid
            artifacts["gate_report_digest"] = strings.TrimSpace(runResult.GateArtifact.Digest)
            bundle := scheduler.BundleRecord{
                CID:    cid,
                Digest: strings.TrimSpace(runResult.GateArtifact.Digest),
                Size:   runResult.GateArtifact.Size,
            }
            if logTTL != "" {
                bundle.TTL = logTTL
                if duration, err := time.ParseDuration(logTTL); err == nil && duration > 0 {
                    bundle.ExpiresAt = now.Add(duration).UTC().Format(time.RFC3339Nano)
                }
            }
            bundles["gate_report"] = bundle
        }

        var gateMetrics *scheduler.GateMetrics
        if runResult.GateReport.Duration > 0 || runResult.GateReport.Message != "" || !runResult.GateReport.Passed {
            metrics := scheduler.GateMetrics{
                Duration: runResult.GateReport.Duration,
            }
            if runResult.GateReport.Passed {
                metrics.Result = scheduler.GateResultPassed
            } else {
                metrics.Result = scheduler.GateResultFailed
            }
            gateMetrics = &metrics
        }

	retentionHint := buildRetentionHint(runResult, manifest.Retention, logTTL, now)

	var assignErr *controlplane.AssignmentError
	if runErr != nil {
		reason := "executor_error"
		if errors.Is(runErr, context.Canceled) {
			reason = "executor_canceled"
            } else if errors.Is(runErr, stepruntime.ErrBuildGateFailed) {
                reason = "build_gate_failed"
            }
		message := runErr.Error()
		if strings.TrimSpace(message) == "" {
			message = reason
		}
		assignErr = &controlplane.AssignmentError{
			Reason:  reason,
			Message: message,
		}
	}

	return controlplane.AssignmentResult{
		State:      state,
		Error:      assignErr,
		Artifacts:  artifacts,
		Bundles:    bundles,
            Gate:       gateMetrics,
            Inspection: manifest.Retention.RetainContainer && runErr != nil,
            Retention:  retentionHint,
        }
}

func firstHydrationSnapshot(snapshots map[string]stepruntime.PublishedArtifact) (stepruntime.PublishedArtifact, bool) {
	if len(snapshots) == 0 {
		return stepruntime.PublishedArtifact{}, false
	}
	keys := make([]string, 0, len(snapshots))
	for name := range snapshots {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return snapshots[keys[0]], true
}

// buildBundleRecord derives bundle retention metadata for scheduler completion payloads.
func buildBundleRecord(artifact stepruntime.PublishedArtifact, ttl string, retainContainer bool, now time.Time) scheduler.BundleRecord {
	record := scheduler.BundleRecord{
		CID:      strings.TrimSpace(artifact.CID),
		Digest:   strings.TrimSpace(artifact.Digest),
		Size:     artifact.Size,
		Retained: retainContainer,
	}
	ttl = strings.TrimSpace(ttl)
	if ttl != "" {
		if duration, err := time.ParseDuration(ttl); err == nil && duration > 0 {
			record.TTL = ttl
			record.ExpiresAt = now.Add(duration).UTC().Format(time.RFC3339Nano)
			record.Retained = true
		} else {
			record.TTL = ttl
		}
	}
	return record
}

// buildRetentionHint prepares the retention SSE payload emitted with log streams.
func buildRetentionHint(result stepruntime.Result, retention contracts.StepRetentionSpec, ttl string, now time.Time) *logstream.RetentionHint {
	hint := &logstream.RetentionHint{
		Retained: retention.RetainContainer || result.Retained,
		TTL:      strings.TrimSpace(ttl),
		Bundle:   strings.TrimSpace(result.LogArtifact.CID),
	}
	if hint.Bundle == "" && strings.TrimSpace(result.DiffArtifact.CID) != "" {
		hint.Bundle = strings.TrimSpace(result.DiffArtifact.CID)
	}
	if hint.TTL != "" {
		if dur, err := time.ParseDuration(hint.TTL); err == nil && dur > 0 {
			hint.Expires = now.Add(dur).UTC().Format(time.RFC3339Nano)
		}
	}
	if !hint.Retained && hint.TTL == "" {
		return nil
	}
	return hint
}

// decodeManifest loads a step manifest from the assignment metadata.
func decodeManifest(value string) (contracts.StepManifest, error) {
	var manifest contracts.StepManifest
	if strings.TrimSpace(value) == "" {
		return manifest, errors.New("stepworker: assignment missing step manifest")
	}
	if err := json.Unmarshal([]byte(value), &manifest); err != nil {
		return manifest, fmt.Errorf("stepworker: decode manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return manifest, fmt.Errorf("stepworker: manifest invalid: %w", err)
	}
	return manifest, nil
}

// newClusterClient constructs an IPFS Cluster client using config/environment overrides.
func newClusterClient(cfg config.Config) (*artifacts.ClusterClient, error) {
	baseURL := resolveConfigString(cfg, envClusterURL)
	if baseURL == "" {
		return nil, errors.New("stepworker: PLOY_IPFS_CLUSTER_API required")
	}
	opts := artifacts.ClusterClientOptions{
		BaseURL:           baseURL,
		AuthToken:         resolveConfigString(cfg, envClusterToken),
		BasicAuthUsername: resolveConfigString(cfg, envClusterUser),
		BasicAuthPassword: resolveConfigString(cfg, envClusterPass),
	}
	if min := resolveConfigInt(cfg, envClusterReplMin); min > 0 {
		opts.ReplicationFactorMin = min
	}
	if max := resolveConfigInt(cfg, envClusterReplMax); max > 0 {
		opts.ReplicationFactorMax = max
	}
	return artifacts.NewClusterClient(opts)
}

// newGateClient builds a Build Gate client backed by the default sandbox runner.
func newGateClient() (stepruntime.GateClient, error) {
    executor, err := javaexec.NewExecutor(javaexec.Options{})
    if err != nil {
        return nil, err
    }
    sandbox := buildgate.NewSandboxRunner(executor, buildgate.SandboxRunnerOptions{})
    gateRunner := &buildgate.Runner{
        Sandbox: sandbox,
    }
    return stepruntime.NewBuildGateClient(stepruntime.BuildGateClientOptions{Runner: gateRunner})
}

// resolveConfigString prefers config overrides before falling back to process env.
func resolveConfigString(cfg config.Config, key string) string {
	if cfg.Environment != nil {
		if value, ok := cfg.Environment[key]; ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return strings.TrimSpace(os.Getenv(key))
}

// resolveConfigInt parses integer overrides from config/environment.
func resolveConfigInt(cfg config.Config, key string) int {
	value := resolveConfigString(cfg, key)
	if value == "" {
		return 0
	}
	num, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return num
}

type clusterFetcher struct {
	client *artifacts.ClusterClient
}

func (f *clusterFetcher) Fetch(ctx context.Context, cid string) (stepruntime.RemoteArtifact, error) {
	if f == nil || f.client == nil {
		return stepruntime.RemoteArtifact{}, errors.New("stepworker: artifact fetcher not configured")
	}
	result, err := f.client.Fetch(ctx, cid)
	if err != nil {
		return stepruntime.RemoteArtifact{}, err
	}
	reader := bytes.NewReader(result.Data)
	return stepruntime.RemoteArtifact{
		CID:     result.CID,
		Digest:  result.Digest,
		Size:    result.Size,
		Content: io.NopCloser(reader),
	}, nil
}

// firstNonEmpty returns the first non-empty string in the provided list.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
