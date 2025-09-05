package policy

import (
    "strings"
    cfg "github.com/iw2rmb/ploy/internal/config"
)

// ApplyFromConfig updates DefaultEnforcer from config service if fields present
func ApplyFromConfig(svc *cfg.Service) {
    if svc == nil { return }
    c := svc.Get()
    if c == nil { return }
    p := c.Policy
    e := NewDefaultEnforcer()
    if len(p.StrictEnvs) > 0 {
        e.strictEnvs = make(map[string]bool)
        for _, s := range p.StrictEnvs {
            s = strings.ToLower(strings.TrimSpace(s))
            if s != "" { e.strictEnvs[s] = true }
        }
    }
    if len(p.SizeCapsMB) > 0 {
        for k, v := range p.SizeCapsMB {
            k = strings.ToUpper(strings.TrimSpace(k))
            if v > 0 { e.sizeCapsMB[k] = v }
        }
    }
    DefaultEnforcer = e
}

