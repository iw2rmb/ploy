package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/moby/moby/client"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

var newNodeActionDockerExecAPI = func() (step.DockerExecAPI, error) {
	return client.New(client.FromEnv)
}

func (r *runController) executeAction(ctx context.Context, req StartActionRequest) {
	started := time.Now()
	defer func() {
		r.mu.Lock()
		delete(r.jobs, req.ActionID)
		r.mu.Unlock()
		r.ReleaseSlot()
	}()

	output, err := executeNodeMaintenanceAction(ctx, strings.TrimSpace(req.ActionType))
	status := types.JobStatusSuccess
	builder := types.NewRunStatsBuilder().DurationMs(time.Since(started).Milliseconds())
	if strings.TrimSpace(output) != "" {
		builder.MetadataEntry("output", clipActionOutput(output))
	}
	if err != nil {
		status = types.JobStatusError
		builder.Error(err.Error())
	}
	if uploadErr := r.statusUploader.UploadActionStatus(ctx, req.ActionID, status.String(), builder.MustBuild()); uploadErr != nil {
		slog.Error("failed to upload action status", "action_id", req.ActionID, "action_type", req.ActionType, "status", status, "error", uploadErr)
	}
}

func executeNodeMaintenanceAction(ctx context.Context, actionType string) (string, error) {
	switch actionType {
	case types.NodeActionCleanupDisk:
		return execNodeUpdaterScript(ctx, cleanupDiskScript, "ploy-node-cleanup-disk")
	case types.NodeActionUpdateUpdater:
		return execNodeUpdaterScript(ctx, updateUpdaterScript, "ploy-node-update-updater")
	default:
		return "", fmt.Errorf("unsupported action_type %q", actionType)
	}
}

func execNodeUpdaterScript(ctx context.Context, script, argv0 string) (string, error) {
	api, err := newNodeActionDockerExecAPI()
	if err != nil {
		return "", fmt.Errorf("create docker client: %w", err)
	}
	if closer, ok := api.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	updaterID, err := step.FindNodeUpdaterContainer(ctx, api)
	if err != nil {
		return "", err
	}
	output, exitCode, err := step.ExecNodeUpdaterBash(ctx, api, updaterID, script, argv0)
	if err != nil {
		return output, err
	}
	if exitCode != 0 {
		return output, fmt.Errorf("node-updater exec exited with code %d: %s", exitCode, strings.TrimSpace(output))
	}
	return output, nil
}

func clipActionOutput(output string) string {
	const limit = 4000
	output = strings.TrimSpace(output)
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "...<truncated>"
}

const cleanupDiskScript = `set -euo pipefail
age="${PLOY_NODE_ACTION_CLEANUP_AGE:-1m}"
if [[ -r /usr/local/bin/ploy-node-updater ]]; then
  export PLOY_NODE_UPDATER_CLEANUP_AGE="$age"
  source /usr/local/bin/ploy-node-updater
  wait_for_jobs
  CLEANUP_AGE="$age"
  CLEANUP_AGE_MINUTES="$(cleanup_age_minutes "$CLEANUP_AGE")"
  run_cleanup_cycle
else
  echo "node-updater script not found; running docker prune only"
fi
docker container prune -f --filter "until=${age}"
docker image prune -a -f --filter "until=${age}"
docker builder prune -a -f --filter "until=${age}" || true
docker volume prune -f || true
if [[ -n "${PLOY_BUILDGATE_CACHE_ROOT:-}" && -d "${PLOY_BUILDGATE_CACHE_ROOT}" ]]; then
  find "${PLOY_BUILDGATE_CACHE_ROOT}" -mindepth 1 -maxdepth 1 -amin +1 -print -exec rm -rf -- {} +
else
  echo "buildgate cache root unavailable or not mounted: ${PLOY_BUILDGATE_CACHE_ROOT:-unset}"
fi`

const updateUpdaterScript = `set -euo pipefail
: "${PLOY_COMPOSE_PROJECT_DIR:?set PLOY_COMPOSE_PROJECT_DIR}"
: "${PLOY_COMPOSE_FILE:?set PLOY_COMPOSE_FILE}"
log_file="${PLOY_NODE_UPDATER_SELF_UPDATE_LOG:-/tmp/ploy-node-updater-self-update.log}"
(
  sleep 1
  if [[ -n "${PLOY_DP_SERVICE_ACCOUNT_KEY_FILE:-}" && -s "${PLOY_DP_SERVICE_ACCOUNT_KEY_FILE}" ]]; then
    dp auth service-acc --key-file "$PLOY_DP_SERVICE_ACCOUNT_KEY_FILE" >/dev/null || true
  fi
  docker compose --project-directory "$PLOY_COMPOSE_PROJECT_DIR" -f "$PLOY_COMPOSE_FILE" pull node-updater
  docker compose --project-directory "$PLOY_COMPOSE_PROJECT_DIR" -f "$PLOY_COMPOSE_FILE" up -d --no-deps --force-recreate node-updater
) >"$log_file" 2>&1 &
echo "node-updater self-update launched; log=${log_file}"`
