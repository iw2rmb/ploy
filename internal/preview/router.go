package preview

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "regexp"
    "strings"

    "github.com/gofiber/fiber/v2"
    orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

var previewHostRe = regexp.MustCompile(`^(?P<sha>[a-f0-9]{7,40})\.(?P<app>[a-z0-9-]+)\.ployd\.app(?::\d+)?$`)

func Router(c *fiber.Ctx) error {
	host := c.Hostname()
	m := previewHostRe.FindStringSubmatch(host)
	if m == nil { 
		return c.Next() 
	}
	
	sha := m[1]
	app := m[2]

	payload := strings.NewReader("")
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://localhost:%s/v1/apps/%s/builds?sha=%s", getenv("PORT","8081"), app, sha), payload)
	req.Header.Set("Content-Type","application/x-tar")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { 
		return c.Status(502).SendString("preview build failed") 
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	jobName := fmt.Sprintf("%s-%s", app, sha)
    monitor := orchestration.NewHealthMonitor()
    if monitor.IsJobHealthy(jobName) {
        endpoint, err := monitor.GetJobEndpoint(jobName)
        if err == nil {
            return c.Redirect(endpoint)
        }
    }
	
	c.Set("Content-Type","application/json")
	c.Set("Retry-After","3")
	return c.Status(resp.StatusCode).Send(b)
}

func getenv(k, d string) string { 
	if v := os.Getenv(k); v != "" { 
		return v 
	}
	return d 
}
