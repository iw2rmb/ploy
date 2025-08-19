package nomad

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RenderData struct {
	App        string
	ImagePath  string
	DockerImage string
	EnvVars    map[string]string
}

func templateForLane(lane string) string {
	switch strings.ToUpper(lane) {
	case "A": return "platform/nomad/lane-a-unikraft.hcl"
	case "B": return "platform/nomad/lane-b-unikraft-posix.hcl"
	case "C": return "platform/nomad/lane-c-osv.hcl"
	case "D": return "platform/nomad/lane-d-jail.hcl"
	case "E": return "platform/nomad/lane-e-oci-kontain.hcl"
	case "F": return "platform/nomad/lane-f-vm.hcl"
	default: return "platform/nomad/lane-c-osv.hcl"
	}
}

func RenderTemplate(lane string, data RenderData) (string, error) {
	tplPath := templateForLane(lane)
	b, err := os.ReadFile(tplPath); if err != nil { return "", err }
	s := string(b)
	s = strings.ReplaceAll(s, "{{APP_NAME}}", data.App)
	s = strings.ReplaceAll(s, "{{IMAGE_PATH}}", data.ImagePath)
	s = strings.ReplaceAll(s, "{{DOCKER_IMAGE}}", data.DockerImage)
	s = strings.ReplaceAll(s, "{{ENV_VARS}}", renderEnvVars(data.EnvVars))
	out := filepath.Join(os.TempDir(), fmt.Sprintf("%s-lane-%s.hcl", data.App, strings.ToLower(lane)))
	if err := os.WriteFile(out, []byte(s), 0644); err != nil { return "", err }
	return out, nil
}

func renderEnvVars(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}
	
	var envLines []string
	envLines = append(envLines, "      env {")
	for key, value := range envVars {
		envLines = append(envLines, fmt.Sprintf("        %s = %q", key, value))
	}
	envLines = append(envLines, "      }")
	
	return strings.Join(envLines, "\n")
}
