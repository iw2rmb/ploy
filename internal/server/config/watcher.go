package config

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Subscriber receives configuration reload notifications.
type Subscriber interface {
	// OnConfigReload is called when configuration is reloaded.
	// Implementations should not block; heavy work should be done asynchronously.
	OnConfigReload(ctx context.Context, cfg Config) error
}

// Watcher monitors configuration file changes and notifies subscribers.
type Watcher struct {
	mu          sync.Mutex
	path        string
	subscribers []Subscriber
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	running     bool
	logger      *slog.Logger
}

// WatcherOptions configure the config watcher.
type WatcherOptions struct {
	Path   string
	Logger *slog.Logger
}

// NewWatcher creates a new config file watcher.
func NewWatcher(opts WatcherOptions) (*Watcher, error) {
	if opts.Path == "" {
		return nil, errors.New("config: watcher path required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{
		path:        opts.Path,
		subscribers: make([]Subscriber, 0),
		logger:      logger,
	}, nil
}

// Subscribe registers a subscriber for config reload notifications.
func (w *Watcher) Subscribe(sub Subscriber) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subscribers = append(w.subscribers, sub)
}

// Start begins watching the config file for changes.
func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return errors.New("config: watcher already running")
	}
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.running = true
	w.mu.Unlock()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		return err
	}

	if err := watcher.Add(w.path); err != nil {
		_ = watcher.Close()
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		return err
	}

	w.wg.Add(1)
	go w.loop(watcher)

	return nil
}

// Stop terminates config file watching.
func (w *Watcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	cancel := w.cancel
	w.cancel = nil
	w.running = false
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Watcher) loop(watcher *fsnotify.Watcher) {
	defer w.wg.Done()
	defer func() { _ = watcher.Close() }()

	// Debounce rapid successive writes (editors often write multiple times).
	var debounceTimer *time.Timer
	const debounceDelay = 100 * time.Millisecond

	for {
		select {
		case <-w.ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Watch for Write or Create events (some editors replace files).
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(debounceDelay, func() {
					w.reload()
				})
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("config watcher error", "err", err)
		}
	}
}

func (w *Watcher) reload() {
	// Check if watcher is still running before processing reload.
	// This guards against debounce timer callbacks firing after Stop().
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	w.logger.Info("config file changed, reloading", "path", w.path)
	cfg, err := Load(w.path)
	if err != nil {
		w.logger.Error("reload config failed", "err", err, "path", w.path)
		return
	}

	w.mu.Lock()
	subs := make([]Subscriber, len(w.subscribers))
	copy(subs, w.subscribers)
	w.mu.Unlock()

	// Notify all subscribers.
	ctx := context.Background()
	for _, sub := range subs {
		if err := sub.OnConfigReload(ctx, cfg); err != nil {
			w.logger.Error("subscriber reload failed", "err", err)
		}
	}
	w.logger.Info("config reloaded successfully", "subscribers", len(subs))
}
