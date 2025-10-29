package httpserver

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/iw2rmb/ploy/internal/api/admin"
	"github.com/iw2rmb/ploy/internal/api/config"
    "github.com/iw2rmb/ploy/internal/node/logstream"
)

// StatusProvider returns node status snapshots for the /v1/node/status endpoint.
type StatusProvider interface {
	Snapshot(ctx context.Context) (map[string]any, error)
}

// AdminService provides administrative node operations.
type AdminService interface {
	RegisterNode(ctx context.Context, req admin.NodeRegistrationRequest) (admin.NodeRegistrationResponse, error)
}

// Options configure the HTTP server.
type Options struct {
    Config       config.Config
    Streams      *logstream.Hub
    Status       StatusProvider
    Admin        AdminService
    Jobs         JobProvider
    ControlPlane http.Handler
}

// Server exposes node and control-plane APIs.
type Server struct {
    mu        sync.Mutex
    cfg       config.Config
    streams   *logstream.Hub
    status    StatusProvider
    admin     AdminService
    jobs      JobProvider
    control   http.Handler
    app       *fiber.App
    listener  net.Listener
    serveDone chan struct{}
    running   bool
	startCtx  context.Context
}

// New constructs the server instance.
func New(opts Options) (*Server, error) {
	if opts.Streams == nil {
		return nil, errors.New("httpserver: streams hub is required")
	}
	if opts.Status == nil {
		opts.Status = noopStatus{}
	}
    s := &Server{
        cfg:     opts.Config,
        streams: opts.Streams,
        status:  opts.Status,
        admin:   opts.Admin,
        jobs:    opts.Jobs,
        control: opts.ControlPlane,
    }
	if err := s.ensureApp(); err != nil {
		return nil, err
	}
	return s, nil
}

// App exposes the underlying Fiber application (primarily for tests).
func (s *Server) App() *fiber.App {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ensureAppLocked()
	return s.app
}

// Config returns the currently active configuration snapshot.
func (s *Server) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Address returns the bound listener address if the server is running.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// Start begins serving requests.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("httpserver: already running")
	}
	if err := s.ensureAppLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	listener, err := s.listenLocked()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.listener = listener
	s.startCtx = ctx
	s.serveDone = make(chan struct{})
	s.running = true
	app := s.app
	done := s.serveDone
	s.mu.Unlock()

	go s.serve(app, listener, done)
	return nil
}

// Stop terminates the server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	app := s.app
	listener := s.listener
	done := s.serveDone
	s.listener = nil
	s.serveDone = nil
	s.running = false
	s.mu.Unlock()

	cancelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if listener != nil {
		_ = listener.Close()
	}
	if app != nil {
		if err := app.ShutdownWithContext(cancelCtx); err != nil {
			return fmt.Errorf("httpserver: shutdown: %w", err)
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-cancelCtx.Done():
			return fmt.Errorf("httpserver: shutdown timeout")
		}
	}
	return nil
}

// Reload applies the provided configuration, restarting the listener if required.
func (s *Server) Reload(ctx context.Context, cfg config.Config) error {
	s.mu.Lock()
	requiresRestart := restartRequired(s.cfg, cfg)
	running := s.running
	startCtx := s.startCtx
	s.cfg = cfg
	if err := s.ensureAppLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	if requiresRestart && running {
		if err := s.Stop(ctx); err != nil {
			return err
		}
		if startCtx == nil {
			startCtx = context.Background()
		}
		return s.Start(startCtx)
	}
	return nil
}

func (s *Server) ensureApp() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureAppLocked()
}

func (s *Server) ensureAppLocked() error {
	if s.app != nil {
		return nil
	}
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ReadTimeout:           s.cfg.HTTP.ReadTimeout,
		WriteTimeout:          s.cfg.HTTP.WriteTimeout,
		IdleTimeout:           s.cfg.HTTP.IdleTimeout,
	})
	app.Use(recover.New())
	s.mountRoutes(app)
	s.app = app
	return nil
}

func (s *Server) mountRoutes(app *fiber.App) {
    app.Get("/v1/node/status", s.handleStatus)
    app.Get("/v1/node/health", s.handleStatus)
    app.Get("/v1/node/jobs", s.handleNodeJobsList)
    app.Get("/v1/node/jobs/:jobID", s.handleNodeJobsDetail)
    app.Get("/v1/node/jobs/:jobID/logs/stream", s.handleLogStream)
    app.Post("/v1/admin/nodes", s.handleAdminNodeCreate)
    if s.control != nil {
		handler := adaptor.HTTPHandler(s.control)
		app.All("/v1", handler)
		app.All("/v1/*", handler)
		app.All("/metrics", handler)
	}
}

func (s *Server) serve(app *fiber.App, listener net.Listener, done chan<- struct{}) {
	defer close(done)
	if err := app.Listener(listener); err != nil && !errors.Is(err, net.ErrClosed) && !isUseOfClosedNetworkError(err) {
		fmt.Fprintf(os.Stderr, "[ployd/http] listener stopped: %v\n", err) //nolint:forbidigo
	}
}

func (s *Server) listenLocked() (net.Listener, error) {
	address := strings.TrimSpace(s.cfg.HTTP.Listen)
	if address == "" {
		address = ":8443"
	}
	if s.cfg.HTTP.TLS.Enabled {
		certPath := strings.TrimSpace(s.cfg.HTTP.TLS.CertPath)
		keyPath := strings.TrimSpace(s.cfg.HTTP.TLS.KeyPath)
		if certPath == "" || keyPath == "" {
			return nil, errors.New("httpserver: tls enabled but certificate or key missing")
		}
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("httpserver: load certificate: %w", err)
		}
		ln, err := tls.Listen("tcp", address, &tls.Config{Certificates: []tls.Certificate{cert}})
		if err != nil {
			return nil, err
		}
		return ln, nil
	}
	return net.Listen("tcp", address)
}

func (s *Server) handleStatus(c *fiber.Ctx) error {
	status, err := s.status.Snapshot(c.UserContext())
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	if status == nil {
		status = map[string]any{"state": "unknown"}
	}
	return c.Status(fiber.StatusOK).JSON(status)
}

func (s *Server) handleLogStream(c *fiber.Ctx) error {
	if s.streams == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "log streaming unavailable")
	}
	jobID := strings.TrimSpace(c.Params("jobID"))
	if jobID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job id required")
	}
	var sinceID int64
	if raw := strings.TrimSpace(c.Get("Last-Event-ID")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid Last-Event-ID")
		}
		if id > 0 {
			sinceID = id
		}
	}

	sub, err := s.streams.Subscribe(c.UserContext(), jobID, sinceID)
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	defer sub.Cancel()

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		_, _ = w.WriteString(":ok\n\n")
		_ = w.Flush()
		for {
			evt, ok := <-sub.Events
			if !ok {
				return
			}
			if err := writeEvent(w, evt); err != nil {
				return
			}
			if strings.EqualFold(evt.Type, "done") {
				return
			}
		}
	})
	return nil
}

func (s *Server) handleAdminNodeCreate(c *fiber.Ctx) error {
	if s.admin == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "admin service unavailable")
	}
	var req admin.NodeRegistrationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	resp, err := s.admin.RegisterNode(c.UserContext(), req)
	if err != nil {
		var httpErr *admin.HTTPError
		if errors.As(err, &httpErr) {
			return fiber.NewError(httpErr.Code, httpErr.Message)
		}
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

type noopStatus struct{}

func (noopStatus) Snapshot(context.Context) (map[string]any, error) {
	return map[string]any{"state": "unknown"}, nil
}

func writeEvent(w *bufio.Writer, evt logstream.Event) error {
	if evt.ID > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", evt.ID); err != nil {
			return err
		}
	}
	if evt.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", evt.Type); err != nil {
			return err
		}
	}
	if len(evt.Data) == 0 {
		if _, err := w.WriteString("data:\n\n"); err != nil {
			return err
		}
		return w.Flush()
	}
	lines := strings.Split(string(evt.Data), "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	if _, err := w.WriteString("\n"); err != nil {
		return err
	}
	return w.Flush()
}

func restartRequired(prev, next config.Config) bool {
	if strings.TrimSpace(prev.HTTP.Listen) != strings.TrimSpace(next.HTTP.Listen) {
		return true
	}
	if prev.HTTP.TLS.Enabled != next.HTTP.TLS.Enabled {
		return true
	}
	if strings.TrimSpace(prev.HTTP.TLS.CertPath) != strings.TrimSpace(next.HTTP.TLS.CertPath) {
		return true
	}
	if strings.TrimSpace(prev.HTTP.TLS.KeyPath) != strings.TrimSpace(next.HTTP.TLS.KeyPath) {
		return true
	}
	return false
}

func isUseOfClosedNetworkError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}
