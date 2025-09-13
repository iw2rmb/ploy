package orchestration

import (
	"fmt"
	"sort"
	"strings"
)

// RenderServiceDockerJobHCL renders a simple service job with one docker task, HTTP port, env, and optional Traefik tags.
func RenderServiceDockerJobHCL(jobName, groupName, taskName, image string, env map[string]string, traefikHost string, traefikTLSResolver string, environment string) string {
	// Stable order for env to keep diffs consistent
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var envLines []string
	for _, k := range keys {
		v := env[k]
		envLines = append(envLines, fmt.Sprintf("        %s = \"%s\"", k, v))
	}
	envBlock := strings.Join(envLines, "\n")

	var tags []string
	if traefikHost != "" {
		tags = append(tags, "traefik.enable=true")
		tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.rule=Host(\"%s\")", jobName, traefikHost))
		tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.tls=true", jobName))
		if traefikTLSResolver != "" {
			tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.tls.certresolver=%s", jobName, traefikTLSResolver))
		}
	}
	tagsLines := ""
	if len(tags) > 0 {
		tagsLines = "        tags = [\n          \"" + strings.Join(tags, "\",\n          \"") + "\"\n        ]"
	}

	hcl := fmt.Sprintf(`
job "%s" {
  datacenters = ["dc1"]
  type = "service"
  priority = 80

  group "%s" {
    count = 2
    update {
      max_parallel = 1
      min_healthy_time  = "30s"
      healthy_deadline  = "5m"
      auto_revert = true
      canary = 1
    }
    restart {
      attempts = 3
      interval = "10m"
      delay    = "30s"
      mode     = "fail"
    }
    task "%s" {
      driver = "docker"
      config {
        image = "%s"
        ports = ["http"]
      }
      env = {
%s
      }
      resources {
        cpu    = 500
        memory = 512
        network {
          port "http" { }
        }
      }
      service {
        name = "%s"
        port = "http"
%s
        check {
          type     = "http"
          path     = "/health"
          interval = "15s"
          timeout  = "10s"
        }
        check {
          name     = "readiness"
          type     = "http"
          path     = "/ready"
          interval = "20s"
          timeout  = "15s"
        }
      }
    }
  }
}
`, jobName, groupName, taskName, image, envBlock, jobName, tagsLines)
	return hcl
}

// RenderBatchDockerJobHCL renders a simple batch job with one docker task, env, and optional single artifact download to local/.
func RenderBatchDockerJobHCL(jobName, groupName, taskName, image string, env map[string]string, artifactURL string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var envLines []string
	for _, k := range keys {
		envLines = append(envLines, fmt.Sprintf("        %s = \"%s\"", k, env[k]))
	}
	envBlock := strings.Join(envLines, "\n")

	artifactBlock := ""
	if artifactURL != "" {
		artifactBlock = fmt.Sprintf(`
      artifact {
        source = "%s"
        destination = "local/"
        options = { archive = "false" }
      }`, artifactURL)
	}

	hcl := fmt.Sprintf(`
job "%s" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 50

  group "%s" {
    count = 1
    task "%s" {
      driver = "docker"
      config { image = "%s" }
      env = {
%s
      }
      resources { cpu = 500, memory = 2048 }
%s
    }
  }
}
`, jobName, groupName, taskName, image, envBlock, artifactBlock)
	return hcl
}
