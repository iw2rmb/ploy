package transflow

import "strings"

// Infra holds resolved infrastructure endpoints and settings.
type Infra struct {
    Controller string // e.g., https://api.dev.ployman.app/v1
    APIBase    string // e.g., https://api.dev.ployman.app (derived from Controller)
    SeaweedURL string // filer URL
    DC         string // Nomad datacenter
}

// ResolveInfra resolves infrastructure values using provided getter with Defaults fallbacks.
func ResolveInfra(get func(string) string) Infra {
    d := ResolveDefaults(get)
    controller := get("PLOY_CONTROLLER")
    apiBase := strings.TrimSuffix(controller, "/v1")
    seaweed := get("PLOY_SEAWEEDFS_URL")
    if seaweed == "" { seaweed = d.SeaweedURL }
    dc := get("NOMAD_DC")
    if dc == "" { dc = d.DC }
    return Infra{Controller: controller, APIBase: apiBase, SeaweedURL: seaweed, DC: dc}
}

// ResolveInfraFromEnv resolves using os.Getenv via defaultGetenv.
func ResolveInfraFromEnv() Infra { return ResolveInfra(getenv) }

