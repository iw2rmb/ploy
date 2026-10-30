package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func maintenanceLockPath() string {
	if path := os.Getenv("PLOY_NODE_MAINTENANCE_LOCK_FILE"); path != "" {
		return path
	}
	return "/run/lock/ploy-node-maintenance.lock"
}

// The host provisions the inode. Creating a private file inside the node would
// silently detach job activity from host maintenance after a missing mount.
func acquireMaintenanceActivity(ctx context.Context) (*os.File, error) {
	file, err := os.OpenFile(maintenanceLockPath(), os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open maintenance lock: %w", err)
	}
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		err = unix.Flock(int(file.Fd()), unix.LOCK_SH|unix.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			break
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
	_ = file.Close()
	return nil, fmt.Errorf("acquire maintenance activity: %w", err)
}

func maintenanceCapability() map[string]any {
	hostname, _ := os.Hostname()
	var stat unix.Stat_t
	if err := unix.Stat(maintenanceLockPath(), &stat); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"protocol": 1, "lock_path": maintenanceLockPath(), "lock_inode": stat.Ino, "hostname": hostname}
}
