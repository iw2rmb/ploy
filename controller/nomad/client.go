package nomad

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type alloc struct {
	ID     string `json:"ID"`
	ClientStatus string `json:"ClientStatus"`
}

type serviceChecks struct {
	Status string `json:"Status"`
}

func WaitHealthy(jobName string, timeout time.Duration) error {
	addr := getenv("NOMAD_ADDR","http://127.0.0.1:4646")
	client := &http.Client{ Timeout: 5 * time.Second }
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// simple approach: query /v1/job/<job>/allocations and ensure at least one runnning alloc
		u := fmt.Sprintf("%s/v1/job/%s/allocations", addr, jobName)
		resp, err := client.Get(u); if err != nil { time.Sleep(2*time.Second); continue }
		var allocs []alloc
		json.NewDecoder(resp.Body).Decode(&allocs)
		resp.Body.Close()
		for _, a := range allocs {
			if a.ClientStatus == "running" { return nil }
		}
		time.Sleep(2*time.Second)
	}
	return fmt.Errorf("job %s not healthy before timeout", jobName)
}

func getenv(k,d string) string { if v:=os.Getenv(k); v!="" { return v }; return d }
