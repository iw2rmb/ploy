package daemon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/ployd/config"
	"github.com/iw2rmb/ploy/internal/workflow/runtime"
)

// Component defines a lifecycle managed by the daemon.
type Component interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Reloadable marks components capable of handling config reloads.
type Reloadable interface {
	Reload(ctx context.Context, cfg config.Config) error
}

// BootstrapRunner executes bootstrap workloads before worker mode starts.
type BootstrapRunner interface {
	Run(ctx context.Context, cfg config.Config) error
}

// Options configure the daemon instance.
type Options struct {
	Config               config.Config
	RuntimeRegistry      *runtime.Registry
	LogStreams           *logstream.Hub
	HTTP                 Component
	Metrics              Component
	ControlPlane         Component
	PKI                  Component
	Scheduler            Component
	Bootstrap            BootstrapRunner
	ControlPlaneShutdown func(context.Context) error
}

// Daemon orchestrates node services.
type Daemon struct {
	mu                   sync.RWMutex
	cfg                  config.Config
	runtimeRegistry      *runtime.Registry
	logStreams           *logstream.Hub
	bootstrap            BootstrapRunner
	components           []componentEntry
	started              bool
	running              bool
	controlPlaneShutdown func(context.Context) error
}

type componentEntry struct {
	name string
	comp Component
}

// New constructs a daemon instance.
func New(opts Options) (*Daemon, error) {
	if opts.RuntimeRegistry == nil {
		opts.RuntimeRegistry = runtime.NewRegistry()
	}
	if opts.LogStreams == nil {
		opts.LogStreams = logstream.NewHub(logstream.Options{})
	}
	if err := ensureComponent("http", opts.HTTP); err != nil {
		return nil, err
	}
	if err := ensureComponent("metrics", opts.Metrics); err != nil {
		return nil, err
	}
	if err := ensureComponent("control-plane", opts.ControlPlane); err != nil {
		return nil, err
	}
	if err := ensureComponent("pki", opts.PKI); err != nil {
		return nil, err
	}
	if err := ensureComponent("scheduler", opts.Scheduler); err != nil {
		return nil, err
	}

	d := &Daemon{
		cfg:                  opts.Config,
		runtimeRegistry:      opts.RuntimeRegistry,
		logStreams:           opts.LogStreams,
		bootstrap:            opts.Bootstrap,
		controlPlaneShutdown: opts.ControlPlaneShutdown,
		components: []componentEntry{
			{name: "http", comp: opts.HTTP},
			{name: "metrics", comp: opts.Metrics},
			{name: "pki", comp: opts.PKI},
			{name: "control-plane", comp: opts.ControlPlane},
			{name: "scheduler", comp: opts.Scheduler},
		},
	}
	return d, nil
}

func ensureComponent(name string, c Component) error {
	if c == nil {
		return fmt.Errorf("daemon: component %s is nil", name)
	}
	return nil
}

// Run starts the daemon, blocking until the context is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return errors.New("daemon: already started")
	}
	cfg := d.cfg
	bootstrap := d.bootstrap
	components := append([]componentEntry(nil), d.components...)
	d.started = true
	d.mu.Unlock()

	if cfg.Mode == config.ModeBootstrap && bootstrap != nil {
		if err := bootstrap.Run(ctx, cfg); err != nil {
			return fmt.Errorf("daemon: bootstrap failed: %w", err)
		}
		// Transition into worker mode after bootstrap success.
		d.mu.Lock()
		d.cfg.Mode = config.ModeWorker
		d.mu.Unlock()
	}

	if err := d.startComponents(ctx, components); err != nil {
		return err
	}
	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	<-ctx.Done()

	d.stopComponents(context.Background(), components)

	if d.controlPlaneShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.controlPlaneShutdown(shutdownCtx); err != nil {
			return fmt.Errorf("daemon: control-plane shutdown: %w", err)
		}
	}

	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	return nil
}

// Reload applies the supplied configuration across reloadable components.
func (d *Daemon) Reload(ctx context.Context, cfg config.Config) error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return errors.New("daemon: not running")
	}
	d.cfg = cfg
	components := append([]componentEntry(nil), d.components...)
	d.mu.Unlock()

	for _, entry := range components {
		if reloadable, ok := entry.comp.(Reloadable); ok {
			if err := reloadable.Reload(ctx, cfg); err != nil {
				return fmt.Errorf("daemon: reload %s: %w", entry.name, err)
			}
		}
	}
	return nil
}

func (d *Daemon) startComponents(ctx context.Context, components []componentEntry) error {
	for idx, entry := range components {
		if err := entry.comp.Start(ctx); err != nil {
			// Stop previously started components.
			for j := idx - 1; j >= 0; j-- {
				_ = components[j].comp.Stop(context.Background())
			}
			return fmt.Errorf("daemon: start %s: %w", entry.name, err)
		}
	}
	return nil
}

func (d *Daemon) stopComponents(ctx context.Context, components []componentEntry) {
	for i := len(components) - 1; i >= 0; i-- {
		_ = components[i].comp.Stop(ctx)
	}
}
