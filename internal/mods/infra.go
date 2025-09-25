package mods

import "strings"

// Infra holds resolved infrastructure endpoints and settings.
type Infra struct {
	Controller   string // e.g., https://api.dev.ployman.app/v1
	APIBase      string // e.g., https://api.dev.ployman.app (derived from Controller)
	SeaweedURL   string // filer URL
	JetStreamURL string // nats://host:port for JetStream access
	DC           string // Nomad datacenter
}

// ResolveInfra resolves infrastructure values using provided getter with Defaults fallbacks.
func ResolveInfra(get func(string) string) Infra {
	d := ResolveDefaults(get)
	controller := get("PLOY_CONTROLLER")
	apiBase := strings.TrimSuffix(controller, "/v1")
	seaweed := get("PLOY_SEAWEEDFS_URL")
	if seaweed == "" {
		seaweed = d.SeaweedURL
	}
	jetstream := get("PLOY_JETSTREAM_URL")
	if jetstream == "" {
		jetstream = d.JetStreamURL
	}
	dc := get("NOMAD_DC")
	if dc == "" {
		dc = d.DC
	}
	return Infra{Controller: controller, APIBase: apiBase, SeaweedURL: seaweed, JetStreamURL: jetstream, DC: dc}
}

// ResolveInfraFromEnv resolves using os.Getenv via defaultGetenv.
func ResolveInfraFromEnv() Infra { return ResolveInfra(getenv) }
