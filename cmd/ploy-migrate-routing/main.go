package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/iw2rmb/ploy/internal/routing"
	"github.com/iw2rmb/ploy/internal/utils"
)

const (
	consulPrefix = "ploy/domains/"
)

type manifest struct {
	GeneratedAt time.Time    `json:"generated_at"`
	DryRun      bool         `json:"dry_run"`
	Apps        []appSummary `json:"apps"`
}

type appSummary struct {
	App             string   `json:"app"`
	ConsulRoutes    int      `json:"consul_routes"`
	JetStreamRoutes int      `json:"jetstream_routes"`
	Added           []string `json:"added,omitempty"`
	Updated         []string `json:"updated,omitempty"`
	Removed         []string `json:"removed,omitempty"`
	DomainsChanged  bool     `json:"domains_changed"`
	Error           string   `json:"error,omitempty"`
}

type migrateConfig struct {
	DryRun   bool
	Manifest string
	Timeout  time.Duration
}

type jetConfig struct {
	URL           string
	CredsPath     string
	User          string
	Password      string
	Bucket        string
	Stream        string
	SubjectPrefix string
	Replicas      int
}

func main() {
	cfg := parseFlags()
	if err := run(context.Background(), cfg); err != nil {
		log.Fatalf("routing migration failed: %v", err)
	}
}

func parseFlags() migrateConfig {
	var cfg migrateConfig
	var timeout time.Duration
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "print actions without writing to JetStream")
	flag.StringVar(&cfg.Manifest, "manifest", "", "optional path to write migration manifest")
	flag.DurationVar(&timeout, "timeout", 2*time.Minute, "overall migration timeout")
	flag.Parse()
	cfg.Timeout = timeout
	return cfg
}

func run(parent context.Context, cfg migrateConfig) error {
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	consulClient, err := newConsulClient()
	if err != nil {
		return err
	}

	routingStore, err := newRoutingStore(ctx)
	if err != nil {
		return err
	}

	summaries, err := migrate(ctx, consulClient, routingStore, cfg.DryRun)
	if err != nil {
		return err
	}

	for _, summary := range summaries {
		if summary.Error != "" {
			log.Printf("[ERROR] %s: %s", summary.App, summary.Error)
			continue
		}
		if summary.ConsulRoutes == 0 && summary.JetStreamRoutes == 0 && !summary.DomainsChanged {
			log.Printf("[SKIP] %s has no routes", summary.App)
			continue
		}
		log.Printf("[APP] %s added=%d updated=%d removed=%d domains_changed=%t", summary.App, len(summary.Added), len(summary.Updated), len(summary.Removed), summary.DomainsChanged)
	}

	if cfg.Manifest != "" {
		if err := writeManifest(cfg.Manifest, manifest{
			GeneratedAt: time.Now().UTC(),
			DryRun:      cfg.DryRun,
			Apps:        summaries,
		}); err != nil {
			return err
		}
		log.Printf("manifest written to %s", cfg.Manifest)
	}

	return nil
}

func newConsulClient() (*consulapi.Client, error) {
	config := consulapi.DefaultConfig()
	if addr := utils.Getenv("CONSUL_HTTP_ADDR", ""); addr != "" {
		config.Address = addr
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create consul client: %w", err)
	}
	return client, nil
}

func newRoutingStore(ctx context.Context) (*routing.Store, error) {
	jc := jetConfig{
		URL:           utils.Getenv("PLOY_ROUTING_JETSTREAM_URL", utils.Getenv("PLOY_JETSTREAM_URL", "")),
		CredsPath:     utils.Getenv("PLOY_ROUTING_JETSTREAM_CREDS", utils.Getenv("PLOY_JETSTREAM_CREDS", "")),
		User:          utils.Getenv("PLOY_ROUTING_JETSTREAM_USER", utils.Getenv("PLOY_JETSTREAM_USER", "")),
		Password:      utils.Getenv("PLOY_ROUTING_JETSTREAM_PASSWORD", utils.Getenv("PLOY_JETSTREAM_PASSWORD", "")),
		Bucket:        utils.Getenv("PLOY_ROUTING_OBJECT_BUCKET", "routing_maps"),
		Stream:        utils.Getenv("PLOY_ROUTING_EVENT_STREAM", "routing_events"),
		SubjectPrefix: utils.Getenv("PLOY_ROUTING_EVENT_SUBJECT_PREFIX", "routing.app"),
		Replicas:      atoiEnv("PLOY_ROUTING_JETSTREAM_REPLICAS", 3),
	}

	if jc.URL == "" {
		return nil, errors.New("PLOY_ROUTING_JETSTREAM_URL must be set")
	}

	opts := routing.StoreConfig{
		URL:           jc.URL,
		UserCreds:     jc.CredsPath,
		User:          jc.User,
		Password:      jc.Password,
		Bucket:        jc.Bucket,
		Stream:        jc.Stream,
		SubjectPrefix: jc.SubjectPrefix,
		Replicas:      jc.Replicas,
	}

	store, err := routing.NewStore(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("initialize routing store: %w", err)
	}
	return store, nil
}

func migrate(ctx context.Context, consulClient *consulapi.Client, store *routing.Store, dryRun bool) ([]appSummary, error) {
	routesByApp, domainsByApp, err := pullConsul(consulClient)
	if err != nil {
		return nil, err
	}

	apps := make([]string, 0, len(routesByApp))
	for app := range routesByApp {
		apps = append(apps, app)
	}
	for app := range domainsByApp {
		if _, ok := routesByApp[app]; !ok {
			apps = append(apps, app)
		}
	}
	sort.Strings(apps)

	summaries := make([]appSummary, 0, len(apps))
	for _, app := range apps {
		summary := processApp(ctx, store, app, routesByApp[app], domainsByApp[app], dryRun)
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func pullConsul(consulClient *consulapi.Client) (map[string]map[string]routing.DomainRoute, map[string][]string, error) {
	pairs, _, err := consulClient.KV().List(consulPrefix, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list consul keys: %w", err)
	}

	routes := make(map[string]map[string]routing.DomainRoute)
	domains := make(map[string][]string)

	for _, pair := range pairs {
		if pair == nil || pair.Key == "" {
			continue
		}

		rel := strings.TrimPrefix(pair.Key, consulPrefix)
		if rel == "" {
			continue
		}

		if strings.HasSuffix(rel, "/config") {
			app := strings.TrimSuffix(rel, "/config")
			list := []string{}
			if len(pair.Value) > 0 {
				if err := json.Unmarshal(pair.Value, &list); err != nil {
					log.Printf("[WARN] failed to decode domain list for %s: %v", app, err)
					continue
				}
			}
			domains[app] = list
			continue
		}

		var appRoutes map[string]routing.DomainRoute
		if len(pair.Value) > 0 {
			if err := json.Unmarshal(pair.Value, &appRoutes); err != nil {
				log.Printf("[WARN] failed to decode routes for %s: %v", rel, err)
				continue
			}
		}
		if appRoutes == nil {
			appRoutes = make(map[string]routing.DomainRoute)
		}
		routes[rel] = appRoutes
	}

	return routes, domains, nil
}

func processApp(ctx context.Context, store *routing.Store, app string, consulRoutes map[string]routing.DomainRoute, consulDomains []string, dryRun bool) appSummary {
	if consulRoutes == nil {
		consulRoutes = make(map[string]routing.DomainRoute)
	}
	consulDomains = dedupeStrings(consulDomains)

	summary := appSummary{App: app, ConsulRoutes: len(consulRoutes)}

	jsRoutes, err := store.GetAppRoutes(ctx, app)
	if err != nil {
		summary.Error = fmt.Sprintf("get jetstream routes: %v", err)
		return summary
	}
	summary.JetStreamRoutes = len(jsRoutes)

	existingDomains, err := store.GetDomains(ctx, app)
	if err != nil {
		summary.Error = fmt.Sprintf("get jetstream domains: %v", err)
		return summary
	}

	toRemove := diffDomains(jsRoutes, consulRoutes)
	changes := findChanges(consulRoutes, jsRoutes)

	summary.Added = changes.added
	summary.Updated = changes.updated
	summary.Removed = toRemove
	summary.DomainsChanged = !stringSlicesEqual(existingDomains, consulDomains)

	if dryRun {
		return summary
	}

	for _, domain := range toRemove {
		if err := store.DeleteAppRoute(ctx, app, domain); err != nil {
			summary.Error = fmt.Sprintf("delete %s: %v", domain, err)
			return summary
		}
	}

	for _, domain := range append(changes.added, changes.updated...) {
		route := consulRoutes[domain]
		if route.App == "" {
			route.App = app
		}
		if route.CreatedAt.IsZero() {
			route.CreatedAt = time.Now().UTC()
		}
		if err := store.SaveAppRoute(ctx, route); err != nil {
			summary.Error = fmt.Sprintf("save %s: %v", domain, err)
			return summary
		}
	}

	if summary.DomainsChanged {
		if err := store.ReplaceDomains(ctx, app, consulDomains); err != nil {
			summary.Error = fmt.Sprintf("replace domains: %v", err)
			return summary
		}
	}

	return summary
}

type changeSet struct {
	added   []string
	updated []string
}

func findChanges(consul map[string]routing.DomainRoute, jet map[string]routing.DomainRoute) changeSet {
	var cs changeSet
	for domain, route := range consul {
		existing, ok := jet[domain]
		if !ok {
			cs.added = append(cs.added, domain)
			continue
		}
		if !routesEqual(route, existing) {
			cs.updated = append(cs.updated, domain)
		}
	}
	sort.Strings(cs.added)
	sort.Strings(cs.updated)
	return cs
}

func diffDomains(jet map[string]routing.DomainRoute, consul map[string]routing.DomainRoute) []string {
	removed := make([]string, 0)
	for domain := range jet {
		if _, ok := consul[domain]; !ok {
			removed = append(removed, domain)
		}
	}
	sort.Strings(removed)
	return removed
}

func routesEqual(a, b routing.DomainRoute) bool {
	if a.App != b.App || a.Domain != b.Domain || a.Port != b.Port || a.AllocID != b.AllocID || a.AllocIP != b.AllocIP || a.HealthPath != b.HealthPath || a.TLSEnabled != b.TLSEnabled {
		return false
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		if !a.CreatedAt.IsZero() || !b.CreatedAt.IsZero() {
			return false
		}
	}
	aliasesA := append([]string(nil), a.Aliases...)
	aliasesB := append([]string(nil), b.Aliases...)
	sort.Strings(aliasesA)
	sort.Strings(aliasesB)
	if len(aliasesA) != len(aliasesB) {
		return false
	}
	for i := range aliasesA {
		if aliasesA[i] != aliasesB[i] {
			return false
		}
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	a = dedupeStrings(a)
	b = dedupeStrings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func dedupeStrings(values []string) []string {
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
	sort.Strings(out)
	return out
}

func writeManifest(path string, data manifest) error {
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func atoiEnv(key string, def int) int {
	val := utils.Getenv(key, "")
	if val == "" {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}
