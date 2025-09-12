package transflow

import "testing"

func TestResolveInfra_DefaultsAndDerivations(t *testing.T) {
    get := func(k string) string {
        switch k {
        case "PLOY_CONTROLLER":
            return "https://api.dev.ployman.app/v1"
        default:
            return ""
        }
    }
    inf := ResolveInfra(get)
    if inf.Controller == "" || inf.APIBase != "https://api.dev.ployman.app" {
        t.Fatalf("bad controller/apiBase: %+v", inf)
    }
    if inf.SeaweedURL == "" || inf.DC == "" {
        t.Fatalf("expected defaults for seaweed/dc: %+v", inf)
    }
}

