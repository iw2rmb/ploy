package orchestration

import (
    "fmt"
    "os"
    "time"

    nomadapi "github.com/hashicorp/nomad/api"
)

// Submit reads an HCL job file, parses and registers it via Nomad API.
func Submit(jobPath string) error {
    hcl, err := os.ReadFile(jobPath)
    if err != nil {
        return fmt.Errorf("read job file: %w", err)
    }
    client, err := newNomadClient()
    if err != nil {
        return err
    }
    jobs := client.Jobs()
    job, err := jobs.ParseHCL(string(hcl), true)
    if err != nil {
        return fmt.Errorf("parse HCL: %w", err)
    }
    _, _, err = jobs.Register(job, nil)
    if err != nil {
        return fmt.Errorf("register job: %w", err)
    }
    return nil
}

// WaitHealthy waits until a job has at least one healthy allocation or timeout.
func WaitHealthy(jobName string, timeout time.Duration) error {
    monitor := NewHealthMonitor()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if monitor.IsJobHealthy(jobName) {
            return nil
        }
        time.Sleep(2 * time.Second)
    }
    return fmt.Errorf("timeout waiting for job %s to be healthy", jobName)
}

func newNomadClient() (*nomadapi.Client, error) {
    cfg := nomadapi.DefaultConfig()
    if addr := getenv("NOMAD_ADDR", ""); addr != "" {
        cfg.Address = addr
    }
    return nomadapi.NewClient(cfg)
}

// DeregisterJob deregisters a job by name; if purge is true, allocations are purged
func DeregisterJob(jobName string, purge bool) error {
    client, err := newNomadClient()
    if err != nil { return err }
    _, _, err = client.Jobs().Deregister(jobName, purge, nil)
    if err != nil {
        return fmt.Errorf("deregister job: %w", err)
    }
    return nil
}
