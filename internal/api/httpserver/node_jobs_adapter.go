package httpserver

import (
    "github.com/iw2rmb/ploy/internal/node/jobs"
)

// jobProvider wraps a jobs.Store and presents JSON-friendly maps.
type jobProvider struct {
    store *jobs.Store
}

// NewJobProvider adapts a jobs.Store to JobProvider.
func NewJobProvider(store *jobs.Store) JobProvider {
    if store == nil {
        return nil
    }
    return &jobProvider{store: store}
}

// List returns newest-first job maps.
func (p *jobProvider) List() []map[string]any {
    recs := p.store.List()
    out := make([]map[string]any, 0, len(recs))
    for _, r := range recs {
        out = append(out, map[string]any{
            "id":           r.ID,
            "state":        string(r.State),
            "started_at":   r.StartedAt,
            "completed_at": r.CompletedAt,
            "log_stream":   r.LogStream,
        })
    }
    return out
}

// GetMap returns a single job map.
func (p *jobProvider) GetMap(id string) (map[string]any, bool) {
    r, ok := p.store.Get(id)
    if !ok {
        return nil, false
    }
    m := map[string]any{
        "id":           r.ID,
        "state":        string(r.State),
        "started_at":   r.StartedAt,
        "completed_at": r.CompletedAt,
        "log_stream":   r.LogStream,
    }
    if r.Error != "" {
        m["error"] = r.Error
    }
    return m, true
}

