package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/workflowkit"
	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestCrashReconcile_StartupRunsBeforeFirstClaim_Contract(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewRunOrchestrationScenario()
	var (
		mu    sync.Mutex
		calls []string
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()
		switch {
		case r.URL.Path == "/v1/jobs/"+s.JobID.String()+"/complete":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/claim":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	controller := &mockRunController{}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{
		docker: &fakeDockerClient{
			listResult: client.ContainerListResult{Items: []containertypes.Summary{
				{
					ID:     "ctr-terminal",
					Labels: map[string]string{types.LabelRunID: s.RunID.String(), types.LabelJobID: s.JobID.String()},
				},
			}},
			inspectByID: map[string]client.ContainerInspectResult{
				"ctr-terminal": {
					Container: containertypes.InspectResponse{
						State: &containertypes.State{
							Running:    false,
							Status:     containertypes.ContainerState("exited"),
							ExitCode:   0,
							StartedAt:  "2026-02-26T15:00:00Z",
							FinishedAt: "2026-02-26T15:00:02Z",
						},
					},
				},
			},
			waitByID: map[string]containertypes.WaitResponse{
				"ctr-terminal": {StatusCode: 0},
			},
		},
		now:            func() time.Time { return time.Date(2026, 2, 26, 15, 0, 10, 0, time.UTC) },
		terminalWindow: 120 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 220*time.Millisecond)
	defer cancel()

	if err := claimer.Start(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start() error = %v, want context deadline exceeded", err)
	}

	mu.Lock()
	defer mu.Unlock()

	completeIdx := -1
	claimIdx := -1
	for i, path := range calls {
		if path == "/v1/jobs/"+s.JobID.String()+"/complete" && completeIdx == -1 {
			completeIdx = i
		}
		if path == "/v1/nodes/"+testNodeID+"/claim" && claimIdx == -1 {
			claimIdx = i
		}
	}
	if completeIdx == -1 {
		t.Fatalf("missing startup completion call: %v", calls)
	}
	if claimIdx == -1 {
		t.Fatalf("missing claim call: %v", calls)
	}
	if completeIdx > claimIdx {
		t.Fatalf("startup completion call must happen before first claim, calls=%v", calls)
	}
}

func TestCrashReconcile_ClassifiesByRuntimeState_Contract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 26, 14, 0, 0, 0, time.UTC)
	fakeDocker := &fakeDockerClient{
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{
				ID:     "running-2",
				State:  containertypes.ContainerState("exited"), // Summary state must not decide classification.
				Labels: map[string]string{types.LabelRunID: "run-r2", types.LabelJobID: "job-r2"},
			},
			{
				ID:     "terminal-1",
				State:  containertypes.StateRunning, // Summary state must not decide classification.
				Labels: map[string]string{types.LabelRunID: "run-t1", types.LabelJobID: "job-t1"},
			},
			{
				ID:     "terminal-0",
				State:  containertypes.StateRunning,
				Labels: map[string]string{types.LabelRunID: "run-t0", types.LabelJobID: "job-t0"},
			},
			{
				ID:     "created-0",
				State:  containertypes.StateRunning,
				Labels: map[string]string{types.LabelRunID: "run-c0", types.LabelJobID: "job-c0"},
			},
			{
				ID:     "missing-job",
				State:  containertypes.StateRunning,
				Labels: map[string]string{types.LabelRunID: "run-missing"},
			},
			{
				ID:     "unmanaged",
				State:  containertypes.StateRunning,
				Labels: map[string]string{},
			},
		}},
		inspectByID: map[string]client.ContainerInspectResult{
			"running-2": inspectWithState(true, containertypes.StateRunning, ""),
			"terminal-1": inspectWithState(
				false,
				containertypes.ContainerState("exited"),
				now.Add(-30*time.Second).Format(time.RFC3339Nano),
			),
			"terminal-0": inspectWithState(
				false,
				containertypes.ContainerState("dead"),
				now.Add(-45*time.Second).Format(time.RFC3339Nano),
			),
			"created-0": inspectWithState(false, containertypes.ContainerState("created"), ""),
		},
	}

	reconciler := &startupCrashReconciler{
		docker:         fakeDocker,
		now:            func() time.Time { return now },
		terminalWindow: 120 * time.Second,
	}
	got, err := reconciler.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	wantRunning := []recoveredRunningContainer{
		{ContainerID: "running-2", RunID: types.RunID("run-r2"), JobID: types.JobID("job-r2")},
	}
	if !reflect.DeepEqual(got.Running, wantRunning) {
		t.Fatalf("Running = %#v, want %#v", got.Running, wantRunning)
	}

	wantTerminalIDs := []string{"terminal-0", "terminal-1"}
	gotTerminalIDs := make([]string, 0, len(got.RecentTerminal))
	for _, c := range got.RecentTerminal {
		gotTerminalIDs = append(gotTerminalIDs, c.ContainerID)
	}
	if !reflect.DeepEqual(gotTerminalIDs, wantTerminalIDs) {
		t.Fatalf("RecentTerminal IDs = %v, want %v", gotTerminalIDs, wantTerminalIDs)
	}
	if got.RecentTerminal[0].JobID != types.JobID("job-t0") || got.RecentTerminal[1].JobID != types.JobID("job-t1") {
		t.Fatalf("RecentTerminal job IDs = [%s %s], want [job-t0 job-t1]", got.RecentTerminal[0].JobID, got.RecentTerminal[1].JobID)
	}
}

func TestCrashReconcile_UsesFinishedAtCutoff_Contract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 26, 15, 0, 0, 0, time.UTC)
	cutoff := now.Add(-120 * time.Second)
	fakeDocker := &fakeDockerClient{
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{
				ID:      "boundary",
				Created: 1, // Created is intentionally old; cutoff must use FinishedAt.
				Labels:  map[string]string{types.LabelRunID: "run-b", types.LabelJobID: "job-b"},
			},
			{
				ID:      "fresh-created-stale-finished",
				Created: now.Unix(), // Created is intentionally recent; stale FinishedAt must be skipped.
				Labels:  map[string]string{types.LabelRunID: "run-s", types.LabelJobID: "job-s"},
			},
			{
				ID:      "inside-window",
				Created: 2,
				Labels:  map[string]string{types.LabelRunID: "run-i", types.LabelJobID: "job-i"},
			},
		}},
		inspectByID: map[string]client.ContainerInspectResult{
			"boundary": inspectWithState(false, containertypes.ContainerState("exited"), cutoff.Format(time.RFC3339Nano)),
			"fresh-created-stale-finished": inspectWithState(
				false,
				containertypes.ContainerState("exited"),
				cutoff.Add(-1*time.Nanosecond).Format(time.RFC3339Nano),
			),
			"inside-window": inspectWithState(
				false,
				containertypes.ContainerState("dead"),
				cutoff.Add(1*time.Nanosecond).Format(time.RFC3339Nano),
			),
		},
	}

	reconciler := &startupCrashReconciler{
		docker:         fakeDocker,
		now:            func() time.Time { return now },
		terminalWindow: 120 * time.Second,
	}
	got, err := reconciler.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	gotTerminalIDs := make([]string, 0, len(got.RecentTerminal))
	for _, c := range got.RecentTerminal {
		gotTerminalIDs = append(gotTerminalIDs, c.ContainerID)
	}
	wantTerminalIDs := []string{"boundary", "inside-window"}
	if !reflect.DeepEqual(gotTerminalIDs, wantTerminalIDs) {
		t.Fatalf("RecentTerminal IDs = %v, want %v", gotTerminalIDs, wantTerminalIDs)
	}
}

func TestCrashReconcile_SkipsStaleTerminalContainers_Contract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 26, 16, 0, 0, 0, time.UTC)
	cutoff := now.Add(-120 * time.Second)
	fakeDocker := &fakeDockerClient{
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{ID: "stale", Labels: map[string]string{types.LabelRunID: "run-stale", types.LabelJobID: "job-stale"}},
			{ID: "empty-finished", Labels: map[string]string{types.LabelRunID: "run-empty", types.LabelJobID: "job-empty"}},
			{ID: "bad-finished", Labels: map[string]string{types.LabelRunID: "run-bad", types.LabelJobID: "job-bad"}},
			{ID: "zero-finished", Labels: map[string]string{types.LabelRunID: "run-zero", types.LabelJobID: "job-zero"}},
			{ID: "no-state", Labels: map[string]string{types.LabelRunID: "run-ns", types.LabelJobID: "job-ns"}},
		}},
		inspectByID: map[string]client.ContainerInspectResult{
			"stale":          inspectWithState(false, containertypes.ContainerState("exited"), cutoff.Add(-1*time.Second).Format(time.RFC3339Nano)),
			"empty-finished": inspectWithState(false, containertypes.ContainerState("exited"), ""),
			"bad-finished":   inspectWithState(false, containertypes.ContainerState("dead"), "not-a-time"),
			"zero-finished":  inspectWithState(false, containertypes.ContainerState("dead"), time.Time{}.Format(time.RFC3339Nano)),
			"no-state":       {Container: containertypes.InspectResponse{}},
		},
	}

	reconciler := &startupCrashReconciler{
		docker:         fakeDocker,
		now:            func() time.Time { return now },
		terminalWindow: 120 * time.Second,
	}
	got, err := reconciler.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got.RecentTerminal) != 0 {
		t.Fatalf("RecentTerminal len = %d, want 0", len(got.RecentTerminal))
	}
}

func TestCrashReconcile_RecoveredRunningMonitor_UploadsLogsAndTerminalStatus(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewRunOrchestrationScenario()
	containerID := "ctr-running-1"
	logsCh := make(chan struct{}, 1)
	completeCh := make(chan struct {
		Status   string `json:"status"`
		ExitCode int32  `json:"exit_code"`
	}, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/logs":
			select {
			case logsCh <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v1/jobs/"+s.JobID.String()+"/complete":
			var completePayload struct {
				Status   string `json:"status"`
				ExitCode int32  `json:"exit_code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&completePayload)
			select {
			case completeCh <- completePayload:
			default:
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fakeDocker := &fakeDockerClient{
		waitByID: map[string]containertypes.WaitResponse{
			containerID: {StatusCode: 0},
		},
		inspectByID: map[string]client.ContainerInspectResult{
			containerID: {
				Container: containertypes.InspectResponse{
					State: &containertypes.State{
						ExitCode:   0,
						Status:     containertypes.ContainerState("exited"),
						StartedAt:  "2026-02-26T15:00:00Z",
						FinishedAt: "2026-02-26T15:00:02Z",
					},
				},
			},
		},
		logsByID: map[string][]byte{
			containerID: multiplexedDockerLogs("stdout line\n", stdcopy.Stdout),
		},
	}

	controller := &mockRunController{}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{docker: fakeDocker}

	claimer.startRecoveredRunningMonitors(context.Background(), []recoveredRunningContainer{
		{ContainerID: containerID, RunID: s.RunID, JobID: s.JobID},
	})

	var (
		logsCalled      bool
		completeCalled  bool
		completePayload struct {
			Status   string `json:"status"`
			ExitCode int32  `json:"exit_code"`
		}
	)
	timeout := time.After(2 * time.Second)
	for {
		if logsCalled && completeCalled && controller.ReleaseCalls() == 1 {
			break
		}
		select {
		case <-logsCh:
			logsCalled = true
		case completePayload = <-completeCh:
			completeCalled = true
		case <-timeout:
			t.Fatalf(
				"timeout waiting for recovered monitor upload, logs_called=%v complete_called=%v release_calls=%d",
				logsCalled,
				completeCalled,
				controller.ReleaseCalls(),
			)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if completePayload.Status != types.JobStatusSuccess.String() {
		t.Fatalf("status = %q, want %q", completePayload.Status, types.JobStatusSuccess.String())
	}
	if completePayload.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", completePayload.ExitCode)
	}
	if controller.AcquireCalls() != 1 {
		t.Fatalf("AcquireSlot calls = %d, want 1", controller.AcquireCalls())
	}
	if controller.ReleaseCalls() != 1 {
		t.Fatalf("ReleaseSlot calls = %d, want 1", controller.ReleaseCalls())
	}
}

func TestCrashReconcile_RecoveredRunningMonitor_ExitCodeAboveOneReportsError(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewRunOrchestrationScenario()
	containerID := "ctr-running-exit-two"
	completeCh := make(chan struct {
		Status   string `json:"status"`
		ExitCode int32  `json:"exit_code"`
	}, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/logs":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v1/jobs/"+s.JobID.String()+"/complete":
			var completePayload struct {
				Status   string `json:"status"`
				ExitCode int32  `json:"exit_code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&completePayload)
			select {
			case completeCh <- completePayload:
			default:
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fakeDocker := &fakeDockerClient{
		waitByID: map[string]containertypes.WaitResponse{
			containerID: {StatusCode: 2},
		},
		inspectByID: map[string]client.ContainerInspectResult{
			containerID: {
				Container: containertypes.InspectResponse{
					State: &containertypes.State{
						ExitCode:   2,
						Status:     containertypes.ContainerState("exited"),
						StartedAt:  "2026-02-26T15:00:00Z",
						FinishedAt: "2026-02-26T15:00:02Z",
					},
				},
			},
		},
		logsByID: map[string][]byte{
			containerID: multiplexedDockerLogs("stderr line\n", stdcopy.Stderr),
		},
	}

	controller := &mockRunController{}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{docker: fakeDocker}

	claimer.startRecoveredRunningMonitors(context.Background(), []recoveredRunningContainer{
		{ContainerID: containerID, RunID: s.RunID, JobID: s.JobID},
	})

	timeout := time.After(2 * time.Second)
	for {
		select {
		case completePayload := <-completeCh:
			if completePayload.Status != types.JobStatusError.String() {
				t.Fatalf("status = %q, want %q", completePayload.Status, types.JobStatusError.String())
			}
			if completePayload.ExitCode != 2 {
				t.Fatalf("exit_code = %d, want 2", completePayload.ExitCode)
			}
			return
		case <-timeout:
			t.Fatal("timeout waiting for recovered running completion")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestCrashReconcile_RecoveredRunningMonitor_WaitErrorReportsError(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewRunOrchestrationScenario()
	containerID := "ctr-running-wait-error"
	completeCh := make(chan struct {
		Status   string `json:"status"`
		ExitCode int32  `json:"exit_code"`
	}, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/jobs/"+s.JobID.String()+"/complete":
			var completePayload struct {
				Status   string `json:"status"`
				ExitCode int32  `json:"exit_code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&completePayload)
			select {
			case completeCh <- completePayload:
			default:
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/logs":
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fakeDocker := &fakeDockerClient{
		waitErrByID: map[string]error{
			containerID: errors.New("wait failed"),
		},
	}

	controller := &mockRunController{}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{docker: fakeDocker}

	claimer.startRecoveredRunningMonitors(context.Background(), []recoveredRunningContainer{
		{ContainerID: containerID, RunID: s.RunID, JobID: s.JobID},
	})

	timeout := time.After(2 * time.Second)
	for {
		select {
		case completePayload := <-completeCh:
			if completePayload.Status != types.JobStatusError.String() {
				t.Fatalf("status = %q, want %q", completePayload.Status, types.JobStatusError.String())
			}
			if completePayload.ExitCode != -1 {
				t.Fatalf("exit_code = %d, want -1", completePayload.ExitCode)
			}
			return
		case <-timeout:
			t.Fatal("timeout waiting for recovered wait-error completion")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestCrashReconcile_RecoveredTerminalContainer_ExitCodeAboveOneReportsError(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewRunOrchestrationScenario()
	containerID := "ctr-terminal-exit-two"
	completeCh := make(chan struct {
		Status   string `json:"status"`
		ExitCode int32  `json:"exit_code"`
	}, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/jobs/"+s.JobID.String()+"/complete":
			var completePayload struct {
				Status   string `json:"status"`
				ExitCode int32  `json:"exit_code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&completePayload)
			select {
			case completeCh <- completePayload:
			default:
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fakeDocker := &fakeDockerClient{
		waitByID: map[string]containertypes.WaitResponse{
			containerID: {StatusCode: 2},
		},
		inspectByID: map[string]client.ContainerInspectResult{
			containerID: {
				Container: containertypes.InspectResponse{
					State: &containertypes.State{
						ExitCode:   2,
						Status:     containertypes.ContainerState("exited"),
						StartedAt:  "2026-02-26T15:00:00Z",
						FinishedAt: "2026-02-26T15:00:02Z",
					},
				},
			},
		},
	}

	controller := &mockRunController{}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{docker: fakeDocker}

	claimer.reconcileRecoveredTerminalContainers(context.Background(), []recoveredTerminalContainer{
		{ContainerID: containerID, RunID: s.RunID, JobID: s.JobID},
	})

	select {
	case completePayload := <-completeCh:
		if completePayload.Status != types.JobStatusError.String() {
			t.Fatalf("status = %q, want %q", completePayload.Status, types.JobStatusError.String())
		}
		if completePayload.ExitCode != 2 {
			t.Fatalf("exit_code = %d, want 2", completePayload.ExitCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for recovered terminal completion")
	}
}

func TestCrashReconcile_RecoveredRunningMonitor_CompletionConflictIsNonFatal(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewRunOrchestrationScenario()
	containerID := "ctr-running-conflict"
	var (
		mu            sync.Mutex
		logsCalled    bool
		completeCalls int
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/logs":
			mu.Lock()
			logsCalled = true
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v1/jobs/"+s.JobID.String()+"/complete":
			mu.Lock()
			completeCalls++
			mu.Unlock()
			w.WriteHeader(http.StatusConflict)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fakeDocker := &fakeDockerClient{
		waitByID: map[string]containertypes.WaitResponse{
			containerID: {StatusCode: 0},
		},
		inspectByID: map[string]client.ContainerInspectResult{
			containerID: {
				Container: containertypes.InspectResponse{
					State: &containertypes.State{
						ExitCode:   0,
						Status:     containertypes.ContainerState("exited"),
						StartedAt:  "2026-02-26T15:00:00Z",
						FinishedAt: "2026-02-26T15:00:02Z",
					},
				},
			},
		},
		logsByID: map[string][]byte{
			containerID: multiplexedDockerLogs("stdout line\n", stdcopy.Stdout),
		},
	}

	controller := &mockRunController{}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{docker: fakeDocker}

	claimer.startRecoveredRunningMonitors(context.Background(), []recoveredRunningContainer{
		{ContainerID: containerID, RunID: s.RunID, JobID: s.JobID},
	})

	timeout := time.After(2 * time.Second)
	for {
		mu.Lock()
		localLogsCalled := logsCalled
		localCompleteCalls := completeCalls
		mu.Unlock()

		if localLogsCalled && localCompleteCalls > 0 && controller.ReleaseCalls() == 1 {
			break
		}
		select {
		case <-timeout:
			t.Fatalf(
				"timeout waiting for recovered monitor completion, logs_called=%v complete_calls=%d release_calls=%d",
				localLogsCalled,
				localCompleteCalls,
				controller.ReleaseCalls(),
			)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	mu.Lock()
	localCompleteCalls := completeCalls
	mu.Unlock()
	if localCompleteCalls != 1 {
		t.Fatalf("completion calls = %d, want 1", localCompleteCalls)
	}
	if controller.AcquireCalls() != 1 {
		t.Fatalf("AcquireSlot calls = %d, want 1", controller.AcquireCalls())
	}
	if controller.ReleaseCalls() != 1 {
		t.Fatalf("ReleaseSlot calls = %d, want 1", controller.ReleaseCalls())
	}
}

func TestCrashReconcile_RecoveredRunningMonitor_IsolatedFailures(t *testing.T) {
	t.Parallel()

	sFail := workflowkit.NewRunOrchestrationScenario()
	sOK := workflowkit.NewRunOrchestrationScenario()
	jobFail := sFail.JobID
	jobOK := sOK.JobID
	runFail := sFail.RunID
	runOK := sOK.RunID
	failContainer := "ctr-fail"
	okContainer := "ctr-ok"

	var mu sync.Mutex
	completedJobs := make(map[string]bool)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/logs":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v1/jobs/"+jobFail.String()+"/complete":
			mu.Lock()
			completedJobs[jobFail.String()] = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/jobs/"+jobOK.String()+"/complete":
			mu.Lock()
			completedJobs[jobOK.String()] = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fakeDocker := &fakeDockerClient{
		waitErrByID: map[string]error{
			failContainer: errors.New("wait failed"),
		},
		waitByID: map[string]containertypes.WaitResponse{
			okContainer: {StatusCode: 0},
		},
		inspectByID: map[string]client.ContainerInspectResult{
			okContainer: {
				Container: containertypes.InspectResponse{
					State: &containertypes.State{
						ExitCode:   0,
						Status:     containertypes.ContainerState("exited"),
						StartedAt:  "2026-02-26T15:00:00Z",
						FinishedAt: "2026-02-26T15:00:03Z",
					},
				},
			},
		},
		logsByID: map[string][]byte{
			okContainer: multiplexedDockerLogs("ok\n", stdcopy.Stdout),
		},
	}

	controller := &mockRunController{}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{docker: fakeDocker}

	claimer.startRecoveredRunningMonitors(context.Background(), []recoveredRunningContainer{
		{ContainerID: failContainer, RunID: runFail, JobID: jobFail},
		{ContainerID: okContainer, RunID: runOK, JobID: jobOK},
	})

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		failDone := completedJobs[jobFail.String()]
		okDone := completedJobs[jobOK.String()]
		mu.Unlock()
		if failDone && okDone {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for completion uploads, got=%v", completedJobs)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if controller.AcquireCalls() != 2 {
		t.Fatalf("AcquireSlot calls = %d, want 2", controller.AcquireCalls())
	}

	releaseDeadline := time.After(2 * time.Second)
	for controller.ReleaseCalls() != 2 {
		select {
		case <-releaseDeadline:
			t.Fatalf("timeout waiting for ReleaseSlot calls, got %d", controller.ReleaseCalls())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	if controller.ReleaseCalls() != 2 {
		t.Fatalf("ReleaseSlot calls = %d, want 2", controller.ReleaseCalls())
	}
}
