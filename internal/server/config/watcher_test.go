package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestWatcherRequiresPath(t *testing.T) {
	_, err := config.NewWatcher(config.WatcherOptions{})
	if err == nil {
		t.Fatal("expected error when path is empty")
	}
}

func TestWatcherNotifiesSubscribersOnConfigChange(t *testing.T) {
	// Create temporary config file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	initialContent := "" +
		"logging:\n" +
		"  level: info\n"
	if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := config.NewWatcher(config.WatcherOptions{
		Path: configPath,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	// Create test subscriber.
	sub := &testSubscriber{ch: make(chan config.Config, 1)}
	watcher.Subscribe(sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = watcher.Stop(context.Background())
	}()

	// Modify the config file.
	updatedContent := "" +
		"logging:\n" +
		"  level: debug\n"
	if err := os.WriteFile(configPath, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Wait for reload notification.
	select {
	case cfg := <-sub.ch:
		if cfg.Logging.Level != "debug" {
			t.Fatalf("expected level=debug, got %s", cfg.Logging.Level)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected reload notification")
	}
}

func TestWatcherStartAlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(""+
		"logging:\n"+
		"  level: info\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := config.NewWatcher(config.WatcherOptions{
		Path: configPath,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	ctx := context.Background()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = watcher.Stop(context.Background())
	}()

	// Second start should fail.
	if err := watcher.Start(ctx); err == nil {
		t.Fatal("expected error when starting already running watcher")
	}
}

func TestWatcherStopNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(""+
		"logging:\n"+
		"  level: info\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := config.NewWatcher(config.WatcherOptions{
		Path: configPath,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	if err := watcher.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() when not running should not error, got %v", err)
	}
}

func TestWatcherMultipleSubscribers(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(""+
		"logging:\n"+
		"  level: info\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := config.NewWatcher(config.WatcherOptions{
		Path: configPath,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	sub1 := &testSubscriber{ch: make(chan config.Config, 1)}
	sub2 := &testSubscriber{ch: make(chan config.Config, 1)}
	watcher.Subscribe(sub1)
	watcher.Subscribe(sub2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = watcher.Stop(context.Background())
	}()

	// Modify config.
	if err := os.WriteFile(configPath, []byte(""+
		"logging:\n"+
		"  level: error\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Both subscribers should receive notification.
	timeout := time.After(1 * time.Second)
	select {
	case <-sub1.ch:
	case <-timeout:
		t.Fatal("subscriber 1 did not receive notification")
	}

	select {
	case <-sub2.ch:
	case <-timeout:
		t.Fatal("subscriber 2 did not receive notification")
	}
}

func TestWatcherHandlesInvalidConfigGracefully(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(""+
		"logging:\n"+
		"  level: info\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := config.NewWatcher(config.WatcherOptions{
		Path: configPath,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	sub := &testSubscriber{ch: make(chan config.Config, 1)}
	watcher.Subscribe(sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = watcher.Stop(context.Background())
	}()

	// Write invalid YAML.
	if err := os.WriteFile(configPath, []byte("invalid: yaml: [[["), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Subscriber should not receive notification for invalid config.
	select {
	case <-sub.ch:
		t.Fatal("subscriber should not receive notification for invalid config")
	case <-time.After(300 * time.Millisecond):
		// Expected: no notification.
	}
}

type testSubscriber struct {
	ch chan config.Config
}

func (s *testSubscriber) OnConfigReload(ctx context.Context, cfg config.Config) error {
	select {
	case s.ch <- cfg:
	default:
	}
	return nil
}
