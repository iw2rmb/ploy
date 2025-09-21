package routing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	nats "github.com/nats-io/nats.go"
)

const (
	defaultChunkSize = 128 * 1024
)

// Store encapsulates routing persistence across Consul and JetStream.
type Store struct {
	conn          *nats.Conn
	js            nats.JetStreamContext
	bucket        nats.ObjectStore
	stream        string
	subjectPrefix string
	chunkSize     int
	replicas      int
	mu            sync.Mutex
}

// StoreConfig configures a routing store instance.
type StoreConfig struct {
	Conn          *nats.Conn
	URL           string
	Credentials   nats.Option
	UserCreds     string
	User          string
	Password      string
	Bucket        string
	Stream        string
	SubjectPrefix string
	ChunkSize     int
	Replicas      int
}

// NewStore constructs a routing store.
func NewStore(ctx context.Context, cfg StoreConfig) (*Store, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("routing store bucket is required")
	}
	if cfg.Stream == "" {
		return nil, fmt.Errorf("routing store stream is required")
	}
	if cfg.SubjectPrefix == "" {
		return nil, fmt.Errorf("routing store subject prefix is required")
	}

	store := &Store{
		stream:        cfg.Stream,
		subjectPrefix: cfg.SubjectPrefix,
		chunkSize:     cfg.ChunkSize,
		replicas:      cfg.Replicas,
	}
	if store.chunkSize <= 0 {
		store.chunkSize = defaultChunkSize
	}
	if store.replicas <= 0 {
		store.replicas = 1
	}

	conn := cfg.Conn
	if conn == nil {
		if cfg.URL == "" {
			return nil, fmt.Errorf("routing store JetStream URL is required")
		}

		options := []nats.Option{nats.Name("ploy-routing-store")}
		if cfg.Credentials != nil {
			options = append(options, cfg.Credentials)
		}
		if cfg.UserCreds != "" {
			options = append(options, nats.UserCredentials(cfg.UserCreds))
		}
		if cfg.User != "" {
			options = append(options, nats.UserInfo(cfg.User, cfg.Password))
		}

		c, err := nats.Connect(cfg.URL, options...)
		if err != nil {
			return nil, fmt.Errorf("connect to JetStream: %w", err)
		}
		conn = c
	}
	store.conn = conn

	js, err := conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}
	store.js = js

	if err := store.ensureBucket(ctx, cfg.Bucket); err != nil {
		return nil, err
	}
	if err := store.ensureStream(ctx); err != nil {
		return nil, err
	}

	return store, nil
}

// SaveAppRoute persists or updates a domain route.
func (s *Store) SaveAppRoute(ctx context.Context, route DomainRoute) error {
	if s == nil {
		return fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	routes, prevInfo, err := s.loadRoutes(ctx, route.App)
	if err != nil {
		return err
	}
	if routes == nil {
		routes = make(map[string]DomainRoute)
	}
	routes[route.Domain] = route

	return s.persist(ctx, route.App, route.Domain, "upsert", routes, prevInfo)
}

// DeleteAppRoute removes a persisted route for the provided domain.
func (s *Store) DeleteAppRoute(ctx context.Context, app, domain string) error {
	if s == nil {
		return fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	routes, prevInfo, err := s.loadRoutes(ctx, app)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		return nil
	}

	if _, ok := routes[domain]; !ok {
		return nil
	}
	delete(routes, domain)

	return s.persist(ctx, app, domain, "delete", routes, prevInfo)
}

// GetAppRoutes loads the stored routes for an app.
func (s *Store) GetAppRoutes(ctx context.Context, app string) (map[string]DomainRoute, error) {
	if s == nil {
		return nil, fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	routes, _, err := s.loadRoutes(ctx, app)
	return routes, err
}

func (s *Store) ensureBucket(ctx context.Context, bucket string) error {
	b, err := s.js.ObjectStore(bucket)
	if err != nil {
		if errors.Is(err, nats.ErrStreamNotFound) || errors.Is(err, nats.ErrObjectNotFound) {
			cfg := &nats.ObjectStoreConfig{
				Bucket:   bucket,
				Replicas: s.replicas,
			}
			b, err = s.js.CreateObjectStore(cfg)
		}
	}
	if err != nil {
		return fmt.Errorf("ensure routing object store: %w", err)
	}
	s.bucket = b
	return nil
}

func (s *Store) ensureStream(ctx context.Context) error {
	_, err := s.js.StreamInfo(s.stream)
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("lookup routing stream: %w", err)
	}

	cfg := &nats.StreamConfig{
		Name:              s.stream,
		Subjects:          []string{fmt.Sprintf("%s.*", s.subjectPrefix)},
		Retention:         nats.LimitsPolicy,
		Duplicates:        time.Hour,
		MaxMsgsPerSubject: 1,
		MaxAge:            24 * time.Hour,
		Storage:           nats.FileStorage,
		Replicas:          s.replicas,
	}

	_, err = s.js.AddStream(cfg)
	if err != nil {
		return fmt.Errorf("create routing stream: %w", err)
	}
	return nil
}

func (s *Store) loadRoutes(ctx context.Context, app string) (map[string]DomainRoute, *nats.ObjectInfo, error) {
	key := s.objectKey(app)
	result, err := s.bucket.Get(key, nats.Context(ctx))
	if errors.Is(err, nats.ErrObjectNotFound) {
		return make(map[string]DomainRoute), nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get routing object: %w", err)
	}
	defer func() {
		_ = result.Close()
	}()

	data, err := readAll(result, s.chunkSize)
	if err != nil {
		return nil, nil, err
	}

	info, err := result.Info()
	if err != nil {
		return nil, nil, fmt.Errorf("routing object info: %w", err)
	}

	var routes map[string]DomainRoute
	if len(data) == 0 {
		routes = make(map[string]DomainRoute)
	} else if err := json.Unmarshal(data, &routes); err != nil {
		return nil, nil, fmt.Errorf("decode routing object: %w", err)
	}
	return routes, info, nil
}

func (s *Store) persist(ctx context.Context, app, domain, change string, routes map[string]DomainRoute, prevInfo *nats.ObjectInfo) error {
	data, err := json.Marshal(routes)
	if err != nil {
		return fmt.Errorf("encode routing map: %w", err)
	}

	r := bytes.NewReader(data)
	checksum := sha256.Sum256(data)
	checksumHex := hex.EncodeToString(checksum[:])

	meta := &nats.ObjectMeta{
		Name: s.objectKey(app),
		Metadata: map[string]string{
			"app":      app,
			"checksum": checksumHex,
		},
		Opts: &nats.ObjectMetaOptions{ChunkSize: uint32(s.chunkSize)},
	}

	info, err := s.bucket.Put(meta, r, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("put routing object: %w", err)
	}

	return s.publish(ctx, app, domain, change, checksumHex, prevInfo, info)
}

func (s *Store) publish(ctx context.Context, app, domain, change, checksum string, prevInfo, info *nats.ObjectInfo) error {
	revision := info.NUID
	prevRevision := ""
	if prevInfo != nil {
		prevRevision = prevInfo.NUID
	}

	event := map[string]any{
		"app":           app,
		"domain":        domain,
		"change":        change,
		"revision":      revision,
		"checksum":      checksum,
		"updated_at":    time.Now().UTC().Format(time.RFC3339),
		"object_key":    s.objectKey(app),
		"prev_revision": prevRevision,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal routing event: %w", err)
	}

	msg := &nats.Msg{
		Subject: s.subject(app),
		Header:  nats.Header{},
		Data:    payload,
	}
	msg.Header.Set("X-Ploy-Change", change)
	msg.Header.Set("X-Ploy-Revision", revision)
	msg.Header.Set("X-Ploy-Checksum", checksum)
	if prevRevision != "" {
		msg.Header.Set("X-Ploy-Prev-Revision", prevRevision)
	}

	if _, err := s.js.PublishMsg(msg, nats.Context(ctx)); err != nil {
		return fmt.Errorf("publish routing event: %w", err)
	}
	return nil
}

func (s *Store) subject(app string) string {
	return fmt.Sprintf("%s.%s", s.subjectPrefix, app)
}

func (s *Store) objectKey(app string) string {
	return fmt.Sprintf("apps/%s/routes.json", app)
}

func (s *Store) domainKey(app string) string {
	return fmt.Sprintf("apps/%s/domains.json", app)
}

// ReplaceDomains overwrites the domain list for an application.
func (s *Store) ReplaceDomains(ctx context.Context, app string, domains []string) error {
	if s == nil {
		return fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.bucket.Delete(s.domainKey(app)); err != nil && !errors.Is(err, nats.ErrObjectNotFound) {
		return err
	}

	return s.saveDomains(ctx, app, domains)
}

func (s *Store) RebroadcastApp(ctx context.Context, app string) error {
	if s == nil {
		return fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	routes, info, err := s.loadRoutes(ctx, app)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		return fmt.Errorf("no routes to broadcast for app %s", app)
	}
	checksum := info.Digest
	checksum = strings.TrimPrefix(checksum, "SHA-256=")
	if err := s.publish(ctx, app, "", "rebroadcast", checksum, nil, info); err != nil {
		return err
	}
	return nil
}

// AppendDomain records a custom domain for the provided app.
func (s *Store) AppendDomain(ctx context.Context, app, domain string) error {
	if s == nil {
		return fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	domains, err := s.loadDomains(ctx, app)
	if err != nil {
		return err
	}
	for _, existing := range domains {
		if existing == domain {
			return nil
		}
	}
	domains = append(domains, domain)
	sort.Strings(domains)
	return s.saveDomains(ctx, app, domains)
}

// GetDomains returns the known domain list for an app.
func (s *Store) GetDomains(ctx context.Context, app string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	domains, err := s.loadDomains(ctx, app)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), domains...), nil
}

// RemoveDomain removes a domain association for the given app.
func (s *Store) RemoveDomain(ctx context.Context, app, domain string) error {
	if s == nil {
		return fmt.Errorf("routing store is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	domains, err := s.loadDomains(ctx, app)
	if err != nil {
		return err
	}

	filtered := make([]string, 0, len(domains))
	for _, existing := range domains {
		if existing != domain {
			filtered = append(filtered, existing)
		}
	}
	return s.saveDomains(ctx, app, filtered)
}

func readAll(result nats.ObjectResult, chunkSize int) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	tmp := make([]byte, chunkSize)
	for {
		n, err := result.Read(tmp)
		if n > 0 {
			if _, wErr := buf.Write(tmp[:n]); wErr != nil {
				return nil, fmt.Errorf("read routing object: %w", wErr)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read routing object: %w", err)
		}
	}
	return buf.Bytes(), nil
}

func (s *Store) loadDomains(ctx context.Context, app string) ([]string, error) {
	result, err := s.bucket.Get(s.domainKey(app), nats.Context(ctx))
	if errors.Is(err, nats.ErrObjectNotFound) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = result.Close()
	}()

	data, err := readAll(result, s.chunkSize)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []string{}, nil
	}

	var domains []string
	if err := json.Unmarshal(data, &domains); err != nil {
		return nil, fmt.Errorf("decode routing domains: %w", err)
	}
	sort.Strings(domains)
	return domains, nil
}

func (s *Store) saveDomains(ctx context.Context, app string, domains []string) error {
	domains = uniqueStrings(domains)
	sort.Strings(domains)
	data, err := json.Marshal(domains)
	if err != nil {
		return fmt.Errorf("encode routing domains: %w", err)
	}

	meta := &nats.ObjectMeta{
		Name: s.domainKey(app),
		Metadata: map[string]string{
			"app": app,
		},
		Opts: &nats.ObjectMetaOptions{ChunkSize: uint32(s.chunkSize)},
	}

	_, err = s.bucket.Put(meta, bytes.NewReader(data), nats.Context(ctx))
	return err
}

func uniqueStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out
}
