package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
	"github.com/moby/moby/api/pkg/stdcopy"
)

// Invalid Docker configuration reports a terminal error without committing pending changes.
func TestExecuteJobInvalidRuntimeFailsWithoutAdvancingWorkspace(t *testing.T) {
	for _, jobType := range []types.JobType{types.JobTypeMig, types.JobTypePostGate} {
		t.Run(jobType.String(), func(t *testing.T) {
			t.Setenv("PLOYD_CACHE_HOME", t.TempDir())
			t.Setenv("DOCKER_HOST", "tcp://%")
			req := newStartRunRequest()
			req.RunID = types.NewRunID()
			req.JobID = types.NewJobID()
			req.RepoID = types.NewRepoID()
			req.JobType = jobType
			dirs := jobDirectories(req.RunID, req.JobID)
			if err := ensureJobDirectories(dirs); err != nil {
				t.Fatal(err)
			}
			workspace := workspaceDir(req.RunID)
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			initRepoWithFile(t, workspace, "README.md", "base\n")
			before := gitrepo.RevParse(t, workspace, "HEAD")
			gitrepo.WriteFile(t, filepath.Join(workspace, "README.md"), "pending change\n")
			server, status := newStatusCaptureServer(t, req.JobID.String())
			controller := newTestController(t, newAgentConfig(server.URL))
			mounts, err := jobMounts(dirs, req.RunID, jobType)
			if err != nil {
				t.Fatal(err)
			}
			if jobType == types.JobTypeMig {
				controller.executeMigJob(context.Background(), req, mounts)
			} else {
				controller.executeGateJob(context.Background(), req, mounts)
			}
			if status.Status != types.JobStatusError.String() || status.ExitCode == nil || *status.ExitCode != -1 {
				t.Fatalf("terminal status = %+v, want Error with exit -1", status)
			}
			if !strings.Contains(fmt.Sprint(status.Stats["error"]), "create container runtime") {
				t.Fatalf("wrong failure cause: %v", status.Stats["error"])
			}
			if got := gitrepo.RevParse(t, workspace, "HEAD"); got != before {
				t.Fatalf("workspace HEAD changed: %s != %s", got, before)
			}
			if got := string(mustReadFile(t, filepath.Join(workspace, "README.md"))); got != "pending change\n" {
				t.Fatalf("workspace changes lost: %q", got)
			}
		})
	}
}

// Both job paths release Docker connections and flush the final log before the artifact upload.
func TestExecuteJobReleasesRuntimeBeforeArtifactUpload(t *testing.T) {
	for _, jobType := range []types.JobType{types.JobTypeMig, types.JobTypePostGate} {
		for _, outcome := range []string{"success", "execution failure", "cancelled", "setup failure"} {
			t.Run(jobType.String()+"/"+outcome, func(t *testing.T) {
				t.Setenv("PLOYD_CACHE_HOME", t.TempDir())
				t.Setenv("DOCKER_API_VERSION", "1.52")
				t.Setenv("DOCKER_TLS_VERIFY", "")
				t.Setenv("DOCKER_CERT_PATH", "")
				t.Setenv("PLOY_DOCKER_AUTH_CONFIG_FILE", "")
				t.Setenv("PLOY_DOCKER_AUTH_REFRESH_SOCKET", "")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var connections sync.WaitGroup
				var requests atomic.Int32
				daemon := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					switch {
					case strings.HasSuffix(r.URL.Path, "/images/create"):
						_, _ = io.WriteString(w, `{"status":"Downloaded"}`)
					case strings.HasSuffix(r.URL.Path, "/containers/create"):
						if outcome == "setup failure" {
							w.WriteHeader(http.StatusInternalServerError)
							_, _ = io.WriteString(w, `{"message":"create failed"}`)
							return
						}
						w.WriteHeader(http.StatusCreated)
						_, _ = io.WriteString(w, `{"Id":"container-1"}`)
					case strings.HasSuffix(r.URL.Path, "/start") || r.Method == http.MethodDelete:
						w.WriteHeader(http.StatusNoContent)
					case strings.HasSuffix(r.URL.Path, "/wait"):
						if outcome == "cancelled" {
							cancel()
							<-r.Context().Done()
							return
						}
						code := 0
						if outcome == "execution failure" {
							code = 1
						}
						_, _ = fmt.Fprintf(w, `{"StatusCode":%d}`, code)
					case strings.HasSuffix(r.URL.Path, "/logs"):
						_, _ = w.Write(multiplexedDockerLogs("final log bytes\n", stdcopy.Stdout))
					case strings.HasSuffix(r.URL.Path, "/json") || strings.HasSuffix(r.URL.Path, "/stats"):
						_, _ = io.WriteString(w, `{}`)
					default:
						t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				daemon.Config.ConnState = func(_ net.Conn, state http.ConnState) {
					switch state {
					case http.StateNew:
						connections.Add(1)
					case http.StateClosed:
						connections.Done()
					}
				}
				daemon.Start()
				defer daemon.Close()
				t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(daemon.URL, "http://"))

				req := newStartRunRequest()
				req.RunID = types.NewRunID()
				req.JobID = types.NewJobID()
				req.RepoID = types.NewRepoID()
				req.JobType = jobType
				req.MigSpec = &contracts.MigSpec{BuildGate: &contracts.BuildGateConfig{Disabled: jobType == types.JobTypeMig}}
				if jobType == types.JobTypeMig {
					req.MigSpec.Steps = []contracts.MigStep{{Image: contracts.JobImage{Universal: "test:latest"}}}
				}
				dirs := jobDirectories(req.RunID, req.JobID)
				if err := ensureRunDirectories(req.RunID); err != nil {
					t.Fatal(err)
				}
				if err := ensureJobDirectories(dirs); err != nil {
					t.Fatal(err)
				}
				workspace := workspaceDir(req.RunID)
				if err := os.MkdirAll(workspace, 0700); err != nil {
					t.Fatal(err)
				}
				initRepoWithFile(t, workspace, "pom.xml", `<project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>test</artifactId><version>1</version><properties><maven.compiler.release>17</maven.compiler.release></properties></project>`)
				req.CommitSHA = types.CommitSHA(gitrepo.RevParse(t, workspace, "HEAD"))
				req.RepoSHAIn = req.CommitSHA
				var logsFlushed atomic.Bool
				var uploaded atomic.Bool
				server, status := newStatusCaptureServer(t, req.JobID.String(), withStatusExtraHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.HasSuffix(r.URL.Path, "/logs"):
						logsFlushed.Store(true)
					case strings.HasSuffix(r.URL.Path, "/artifact"):
						closed := make(chan struct{})
						go func() { connections.Wait(); close(closed) }()
						select {
						case <-closed:
						case <-time.After(3 * time.Second):
							t.Error("artifact upload started before Docker connections closed")
						}
						var payload struct {
							Bundle []byte `json:"bundle"`
						}
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Error(err)
						}
						if outcome == "success" || outcome == "execution failure" {
							if !logsFlushed.Load() {
								t.Error("artifact upload started before final log flush")
							}
							entries := tarEntriesFromBundle(t, payload.Bundle)
							if got := string(entries["artifacts/"+req.JobID.String()+"/stdout.log"].Content); got != "final log bytes\n" {
								t.Errorf("uploaded stdout = %q, want final log bytes", got)
							}
						}
						uploaded.Store(true)
						w.WriteHeader(http.StatusCreated)
						_, _ = io.WriteString(w, `{"artifact_bundle_id":"artifact-1","cid":"cid-1"}`)
					}
				})))
				controller := newTestController(t, newAgentConfig(server.URL))
				mounts, err := jobMounts(dirs, req.RunID, jobType)
				if err != nil {
					t.Fatal(err)
				}
				if jobType == types.JobTypeMig {
					controller.executeMigJob(ctx, req, mounts)
				} else {
					controller.executeGateJob(ctx, req, mounts)
				}
				wantStatus := map[string]types.JobStatus{"success": types.JobStatusSuccess, "execution failure": types.JobStatusFail, "cancelled": types.JobStatusCancelled, "setup failure": types.JobStatusError}[outcome]
				if status.Status != wantStatus.String() {
					t.Errorf("status = %s, want %s; stats=%v", status.Status, wantStatus, status.Stats)
				}
				if requests.Load() == 0 || !uploaded.Load() {
					t.Fatalf("execution or final upload missing: Docker requests=%d uploaded=%v", requests.Load(), uploaded.Load())
				}

				if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
					t.Fatalf("job cleanup removed retained workspace: %v", err)
				}
			})
		}
	}
}

// Log setup failure closes the runtime before returning the original error.
func TestInitExecutionContextLogSetupFailure(t *testing.T) {
	closed := 0
	runtime := &mockContainerRuntime{closeFn: func() error { closed++; return nil }}
	controller := &runController{
		cfg:      Config{HTTP: HTTPConfig{TLS: TLSConfig{Enabled: true, CertPath: filepath.Join(t.TempDir(), "missing-cert")}}},
		uploader: &baseUploader{},
	}
	execCtx, cleanup, err := controller.initExecutionContext(context.Background(), "run", "job", func(step.ContainerRuntimeOptions) (step.ContainerRuntime, error) {
		return runtime, nil
	})
	if err == nil || !strings.Contains(err.Error(), "create log streamer") {
		t.Fatalf("initialization error = %v, want log streamer configuration failure", err)
	}
	if closed != 1 {
		t.Fatalf("runtime close calls = %d, want 1", closed)
	}
	if cleanup != nil || execCtx.runner.Containers != nil || execCtx.logStreamer != nil {
		t.Fatal("failed initialization returned live execution resources")
	}
}
