package nodeagent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"golang.org/x/sys/unix"
)

func (r *runController) cleanupRunState(ctx context.Context, runID types.RunID) (types.RunSummary, error) {
	if r.uploader == nil {
		return types.RunSummary{}, fmt.Errorf("cleanup run reader is unavailable")
	}
	u := MustBuildURL(r.cfg.ServerURL, "/v1/runs/"+runID.String())
	run, err := httpx.DoJSON[types.RunSummary](ctx, r.uploader.client, http.MethodGet, u, nil, http.StatusOK, "get cleanup run state")
	if err != nil {
		return run, err
	}
	if run.ID != runID || run.Attempt < 1 || run.ResumeCount < 0 {
		return run, fmt.Errorf("invalid cleanup run identity: %s", runID)
	}
	if run.CleanupEligible {
		if run.FinishedAt == nil || run.FinishedAt.IsZero() || (run.Status != types.RunStatusSuccess && run.Status != types.RunStatusFail && run.Status != types.RunStatusCancelled) {
			return run, fmt.Errorf("incomplete eligible cleanup run: %s", runID)
		}
	}
	return run, nil
}

func (r *runController) pruneExpiredRunData(ctx context.Context, docker cleanupDockerClient, mountInfoPath string) error {
	root := cacheRootDir()
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := cleanupCanonicalDirectory(root); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var passErr error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(passErr, err)
		}
		var runID types.RunID
		if err := runID.UnmarshalText([]byte(entry.Name())); err != nil {
			continue
		}
		if err := r.pruneRunData(ctx, runID, docker, mountInfoPath); err != nil {
			slog.Error("preserve run cleanup data", "run_id", runID, "error", err)
			passErr = errors.Join(passErr, err)
		}
	}
	return passErr
}

func (r *runController) pruneRunData(ctx context.Context, runID types.RunID, docker cleanupDockerClient, mountInfoPath string) error {
	var paths []string
	for _, path := range []string{workspaceDir(runID), runRuntimeShareDir(runID)} {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := cleanupCanonicalDirectory(path); err != nil {
			return err
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil
	}
	observed, err := r.cleanupRunState(ctx, runID)
	if err != nil || !observed.CleanupEligible {
		return err
	}
	for _, path := range paths {
		mounts, err := cleanupMountTargets(mountInfoPath)
		if err != nil {
			return err
		}
		containers, err := inspectCleanupContainers(ctx, docker)
		if err != nil {
			return err
		}
		if err := checkCleanupPath(path, mounts, containers, r.cfg.NodeID.String()); err != nil {
			return err
		}
		current, err := r.cleanupRunState(ctx, runID)
		if err != nil {
			return err
		}
		if !current.CleanupEligible || current.Status != observed.Status || current.Attempt != observed.Attempt || current.ResumeCount != observed.ResumeCount || !current.FinishedAt.Equal(*observed.FinishedAt) {
			return fmt.Errorf("run cleanup identity changed: %s", runID)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove expired run data %s: %w", path, err)
		}
		slog.Info("removed expired run data", "run_id", runID, "path", path, "attempt", current.Attempt)
	}
	return nil
}

func cleanupCanonicalDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil || abs != path {
		return fmt.Errorf("cleanup path is not absolute: %s", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve cleanup path %s: %w", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if resolved != path || !info.IsDir() {
		return fmt.Errorf("cleanup path is not a canonical directory: %s", path)
	}
	return nil
}

func cleanupMountTargets(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read cleanup host mounts: %w", err)
	}
	defer file.Close()
	var targets []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			return nil, fmt.Errorf("malformed host mountinfo")
		}
		separator := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || len(fields)-separator != 4 {
			return nil, fmt.Errorf("malformed host mountinfo separator")
		}
		target, err := unescapeMountInfo(fields[4])
		if err != nil || !filepath.IsAbs(target) {
			return nil, fmt.Errorf("malformed host mount target: %q", fields[4])
		}
		targets = append(targets, filepath.Clean(target))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read cleanup host mounts: %w", err)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("host mount inspection returned no mounts")
	}
	return targets, nil
}

func unescapeMountInfo(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		if i+3 >= len(value) {
			return "", fmt.Errorf("truncated mount escape")
		}
		n, err := strconv.ParseUint(value[i+1:i+4], 8, 8)
		if err != nil || (n != 32 && n != 9 && n != 10 && n != 92) {
			return "", fmt.Errorf("invalid mount escape")
		}
		out.WriteByte(byte(n))
		i += 3
	}
	return out.String(), nil
}

func cleanupPathWithin(parent, path string) bool {
	return parent == path || strings.HasPrefix(path, strings.TrimSuffix(parent, string(filepath.Separator))+string(filepath.Separator))
}

func checkCleanupPath(path string, mounts []string, containers []container.InspectResponse, nodeID string) error {
	if err := cleanupCanonicalDirectory(path); err != nil {
		return err
	}
	for _, target := range mounts {
		if target == filepath.Dir(path) || cleanupPathWithin(path, target) {
			return fmt.Errorf("cleanup path contains a host mount: %s", path)
		}
	}
	for _, c := range containers {
		if nodeID != "" && c.Config.Labels["com.ploy.component"] == "node" && c.Config.Labels["com.ploy.node_id"] == nodeID {
			continue
		}
		for _, m := range c.Mounts {
			if m.Type == "" {
				return fmt.Errorf("container %s has an unknown mount type", c.ID)
			}
			if m.Type != mount.TypeBind {
				continue
			}
			source, err := cleanupMountSource(m.Source)
			if err != nil {
				return fmt.Errorf("resolve container mount %s: %w", m.Source, err)
			}
			if cleanupPathWithin(path, source) || cleanupPathWithin(source, path) {
				return fmt.Errorf("cleanup path is referenced by container %s: %s", c.ID, path)
			}
		}
	}
	return nil
}

func cleanupMountSource(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("container bind source is not absolute")
	}
	path = filepath.Clean(path)
	var missing []string
	links := 0
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) || path == string(filepath.Separator) {
			return "", err
		}
		info, statErr := os.Lstat(path)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 255 {
				return "", fmt.Errorf("too many container mount symlinks")
			}
			target, err := os.Readlink(path)
			if err != nil {
				return "", err
			}
			if filepath.IsAbs(target) {
				path = filepath.Clean(target)
			} else {
				path = filepath.Join(filepath.Dir(path), target)
			}
			continue
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", statErr
		}
		// Stopped containers can retain bind sources removed by per-job cleanup.
		missing = append(missing, filepath.Base(path))
		path = filepath.Dir(path)
	}
}

func pruneCleanupGitClones(ctx context.Context, age time.Duration, now time.Time) error {
	base := os.Getenv("PLOYD_CACHE_HOME")
	if base == "" {
		return nil
	}
	root := filepath.Join(base, "git-clones")
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	}
	if err := cleanupCanonicalDirectory(root); err != nil {
		return err
	}
	var expired []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() || entry.Name() != ".git" {
			return nil
		}
		accessed, err := cleanupAccessTime(path)
		if err != nil {
			return err
		}
		// Match find -amin +N: round elapsed minutes down before comparing.
		if now.Sub(accessed)/time.Minute > age/time.Minute {
			expired = append(expired, filepath.Dir(path))
		}
		return filepath.SkipDir
	})
	if err != nil {
		return fmt.Errorf("inspect Git clones: %w", err)
	}
	for _, path := range expired {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := cleanupCanonicalDirectory(path); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove expired Git clone: %w", err)
		}
	}
	return nil
}

func cleanupAccessTime(path string) (time.Time, error) {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return time.Time{}, err
	}
	return time.Unix(stat.Atim.Sec, stat.Atim.Nsec), nil
}
