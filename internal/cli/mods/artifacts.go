package mods

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "sort"
    "strings"

    modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// ArtifactsCommand lists artifacts attached to a Mods ticket by stage.
type ArtifactsCommand struct {
    Client  *http.Client
    BaseURL *url.URL
    Ticket  string
    Output  io.Writer
}

// Run performs GET /v1/mods/{ticket} and prints per-stage artifacts.
func (c ArtifactsCommand) Run(ctx context.Context) error {
    if c.Client == nil { return errors.New("mods artifacts: http client required") }
    if c.BaseURL == nil { return errors.New("mods artifacts: base url required") }
    ticket := strings.TrimSpace(c.Ticket)
    if ticket == "" { return errors.New("mods artifacts: ticket required") }
    endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "mods", url.PathEscape(ticket))
    if err != nil { return err }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil { return err }
    resp, err := c.Client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        data, _ := io.ReadAll(resp.Body)
        msg := strings.TrimSpace(string(data))
        if msg == "" { msg = resp.Status }
        return fmt.Errorf("mods artifacts: %s", msg)
    }
    var payload modsapi.TicketStatusResponse
    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil { return err }
    if c.Output == nil { return nil }
    // Stable iteration order by stage id.
    var stages []string
    for id := range payload.Ticket.Stages { stages = append(stages, id) }
    sort.Strings(stages)
    for _, id := range stages {
        st := payload.Ticket.Stages[id]
        _, _ = fmt.Fprintf(c.Output, "%s:\n", strings.TrimSpace(st.StageID))
        if len(st.Artifacts) == 0 { continue }
        // Stable artifact key order.
        var keys []string
        for k := range st.Artifacts { keys = append(keys, k) }
        sort.Strings(keys)
        for _, k := range keys {
            v := st.Artifacts[k]
            _, _ = fmt.Fprintf(c.Output, "  %s: %s\n", strings.TrimSpace(k), strings.TrimSpace(v))
        }
    }
    return nil
}

