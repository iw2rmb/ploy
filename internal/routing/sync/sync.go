package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	nats "github.com/nats-io/nats.go"
	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/routing"
)

var domainSanitizer = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// DynamicConfig represents the Traefik dynamic configuration structure that this
// package reads and writes.
type DynamicConfig struct {
	HTTP *HTTPSection   `yaml:"http,omitempty"`
	TLS  map[string]any `yaml:"tls,omitempty"`
}

// HTTPSection captures only the pieces we mutate.
type HTTPSection struct {
	Routers     map[string]*Router        `yaml:"routers,omitempty"`
	Middlewares map[string]map[string]any `yaml:"middlewares,omitempty"`
	Services    map[string]map[string]any `yaml:"services,omitempty"`
}

// Router encapsulates a Traefik router definition.
type Router struct {
	Rule        string     `yaml:"rule"`
	EntryPoints []string   `yaml:"entryPoints,omitempty"`
	Service     string     `yaml:"service"`
	Middlewares []string   `yaml:"middlewares,omitempty"`
	TLS         *RouterTLS `yaml:"tls,omitempty"`
	Priority    int        `yaml:"priority,omitempty"`
}

// RouterTLS captures TLS settings.
type RouterTLS struct {
	CertResolver string `yaml:"certResolver,omitempty"`
}

// GenerateDynamicConfig merges the supplied base YAML configuration with
// routers derived from the provided app routing maps.
func GenerateDynamicConfig(base []byte, routes map[string]map[string]routing.DomainRoute) ([]byte, error) {
	cfg, err := parseBaseConfig(base)
	if err != nil {
		return nil, err
	}

	if cfg.HTTP == nil {
		cfg.HTTP = &HTTPSection{}
	}
	if cfg.HTTP.Middlewares == nil {
		cfg.HTTP.Middlewares = defaultMiddlewares()
	}

	cfg.HTTP.Routers = buildRouters(routes)

	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func parseBaseConfig(base []byte) (*DynamicConfig, error) {
	cfg := &DynamicConfig{}
	if len(bytes.TrimSpace(base)) == 0 {
		cfg.HTTP = &HTTPSection{Middlewares: defaultMiddlewares()}
		cfg.TLS = defaultTLS()
		return cfg, nil
	}
	if err := yaml.Unmarshal(base, cfg); err != nil {
		return nil, fmt.Errorf("parse base config: %w", err)
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &HTTPSection{}
	}
	if cfg.HTTP.Middlewares == nil {
		cfg.HTTP.Middlewares = defaultMiddlewares()
	}
	if cfg.TLS == nil {
		cfg.TLS = defaultTLS()
	}
	return cfg, nil
}

func defaultMiddlewares() map[string]map[string]any {
	return map[string]map[string]any{
		"https-redirect": {
			"redirectScheme": map[string]any{
				"scheme":    "https",
				"permanent": true,
			},
		},
		"secure-headers": {
			"headers": map[string]any{
				"stsSeconds":           31536000,
				"stsIncludeSubdomains": true,
				"stsPreload":           true,
				"forceSTSHeader":       true,
				"sslRedirect":          true,
				"browserXssFilter":     true,
				"contentTypeNosniff":   true,
			},
		},
	}
}

func defaultTLS() map[string]any {
	return map[string]any{
		"options": map[string]any{
			"default": map[string]any{
				"minVersion": "VersionTLS12",
			},
		},
	}
}

func buildRouters(routes map[string]map[string]routing.DomainRoute) map[string]*Router {
	result := make(map[string]*Router)
	apps := make([]string, 0, len(routes))
	for app := range routes {
		apps = append(apps, app)
	}
	sort.Strings(apps)
	for _, app := range apps {
		domains := make([]string, 0, len(routes[app]))
		for domain := range routes[app] {
			domains = append(domains, domain)
		}
		sort.Strings(domains)
		for _, domain := range domains {
			route := routes[app][domain]
			routerName := fmt.Sprintf("%s--%s", app, sanitizeDomain(domain))
			hosts := append([]string{route.Domain}, route.Aliases...)
			rule := fmt.Sprintf("Host(`%s`)", strings.Join(hosts, "`,`"))
			entryPoints := []string{"web"}
			middlewares := []string{"secure-headers@file"}
			var tlsCfg *RouterTLS
			if route.TLSEnabled {
				entryPoints = []string{"websecure"}
				middlewares = append([]string{"https-redirect@file"}, middlewares...)
				tlsCfg = &RouterTLS{CertResolver: "default-acme"}
			}
			result[routerName] = &Router{
				Rule:        rule,
				EntryPoints: entryPoints,
				Service:     fmt.Sprintf("%s@consulcatalog", route.App),
				Middlewares: middlewares,
				TLS:         tlsCfg,
			}
		}
	}
	return result
}

func sanitizeDomain(domain string) string {
	clean := domainSanitizer.ReplaceAllString(domain, "-")
	clean = strings.Trim(clean, "-")
	if clean == "" {
		return "domain"
	}
	return clean
}

// RouteWriter captures the behaviour required to persist rendered routes.
type RouteWriter interface {
	Write(ctx context.Context, routes map[string]map[string]routing.DomainRoute) error
}

// FileWriter writes rendered routes into a YAML file.
type FileWriter struct {
	Path string
}

// Write renders the full configuration and atomically updates the target file.
func (w *FileWriter) Write(ctx context.Context, routes map[string]map[string]routing.DomainRoute) error {
	_ = ctx
	base := []byte{}
	if b, err := os.ReadFile(w.Path); err == nil {
		base = b
	}

	content, err := GenerateDynamicConfig(base, routes)
	if err != nil {
		return err
	}

	dir := filepath.Dir(w.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "routing-*.yml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, w.Path); err != nil {
		return err
	}
	success = true
	return nil
}

// Syncer listens for routing events and writes dynamic Traefik configuration files.
type Syncer struct {
	bucket        nats.ObjectStore
	stream        string
	subjectPrefix string
	durable       string
	js            nats.JetStreamContext
	writer        RouteWriter
	routes        map[string]map[string]routing.DomainRoute
	mx            sync.Mutex
}

// Config contains startup parameters for the Syncer.
type Config struct {
	Bucket        nats.ObjectStore
	Stream        string
	SubjectPrefix string
	Durable       string
	JetStream     nats.JetStreamContext
	Writer        RouteWriter
}

// NewSyncer initialises a Syncer instance.
func NewSyncer(cfg Config) *Syncer {
	return &Syncer{
		bucket:        cfg.Bucket,
		stream:        cfg.Stream,
		subjectPrefix: cfg.SubjectPrefix,
		durable:       cfg.Durable,
		js:            cfg.JetStream,
		writer:        cfg.Writer,
		routes:        make(map[string]map[string]routing.DomainRoute),
	}
}

type routingEvent struct {
	App          string `json:"app"`
	Domain       string `json:"domain"`
	Change       string `json:"change"`
	Revision     string `json:"revision"`
	PrevRevision string `json:"prev_revision"`
	Checksum     string `json:"checksum"`
	UpdatedAt    string `json:"updated_at"`
	ObjectKey    string `json:"object_key"`
}

// Start begins pulling events until the context is cancelled.
func (s *Syncer) Start(ctx context.Context) error {
	if s.js == nil {
		return fmt.Errorf("jetstream context required")
	}
	if err := s.initialSync(ctx); err != nil {
		return err
	}

	pattern := fmt.Sprintf("%s.*", s.subjectPrefix)
	sub, err := s.js.PullSubscribe(pattern, s.durable, nats.BindStream(s.stream), nats.ManualAck())
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
		if errors.Is(err, nats.ErrTimeout) {
			continue
		}
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			var evt routingEvent
			if err := json.Unmarshal(msg.Data, &evt); err != nil {
				_ = msg.Nak()
				return fmt.Errorf("decode routing event: %w", err)
			}
			if err := s.handleEvent(ctx, evt); err != nil {
				_ = msg.Nak()
				return err
			}
			_ = msg.Ack()
		}
	}
}

func (s *Syncer) handleEvent(ctx context.Context, event routingEvent) error {
	routeMap, err := s.fetchRoutes(ctx, event.ObjectKey)
	if err != nil {
		return err
	}

	s.mx.Lock()
	if len(routeMap) == 0 {
		delete(s.routes, event.App)
	} else {
		s.routes[event.App] = routeMap
	}
	cloned := cloneRoutes(s.routes)
	s.mx.Unlock()

	return s.writer.Write(ctx, cloned)
}

func (s *Syncer) fetchRoutes(ctx context.Context, objectKey string) (map[string]routing.DomainRoute, error) {
	result, err := s.bucket.Get(objectKey, nats.Context(ctx))
	if errors.Is(err, nats.ErrObjectNotFound) {
		return map[string]routing.DomainRoute{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = result.Close()
	}()

	data, err := readAll(result)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]routing.DomainRoute{}, nil
	}

	var routes map[string]routing.DomainRoute
	if err := json.Unmarshal(data, &routes); err != nil {
		return nil, fmt.Errorf("decode routing object: %w", err)
	}
	return routes, nil
}

func (s *Syncer) initialSync(ctx context.Context) error {
	objects, err := s.bucket.List()
	if err != nil {
		return err
	}
	for _, obj := range objects {
		if !strings.HasSuffix(obj.Name, "routes.json") {
			continue
		}
		parts := strings.Split(obj.Name, "/")
		if len(parts) < 3 {
			continue
		}
		app := parts[1]
		routes, err := s.fetchRoutes(ctx, obj.Name)
		if err != nil {
			return err
		}
		if len(routes) == 0 {
			continue
		}
		s.routes[app] = routes
	}

	return s.writer.Write(ctx, cloneRoutes(s.routes))
}

func readAll(result nats.ObjectResult) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(result); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cloneRoutes(src map[string]map[string]routing.DomainRoute) map[string]map[string]routing.DomainRoute {
	clone := make(map[string]map[string]routing.DomainRoute, len(src))
	for app, domains := range src {
		inner := make(map[string]routing.DomainRoute, len(domains))
		for domain, route := range domains {
			inner[domain] = route
		}
		clone[app] = inner
	}
	return clone
}
