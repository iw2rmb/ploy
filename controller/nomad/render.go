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
}

func templateForLane(lane string) string {
	switch strings.ToUpper(lane) {
	case "A": return "platform/nomad/templates/lane-a-unikraft.hcl.tmpl"
	case "B": return "platform/nomad/templates/lane-b-unikraft-posix.hcl.tmpl"
	case "C": return "platform/nomad/templates/lane-c-osv.hcl.tmpl"
	case "D": return "platform/nomad/templates/lane-d-jail.hcl.tmpl"
	case "E": return "platform/nomad/templates/lane-e-oci-kontain.hcl.tmpl"
	case "F": return "platform/nomad/templates/lane-f-vm.hcl.tmpl"
	default: return "platform/nomad/templates/lane-c-osv.hcl.tmpl"
	}
}

func RenderTemplate(lane string, data RenderData) (string, error) {
	tplPath := templateForLane(lane)
	b, err := os.ReadFile(tplPath); if err != nil { return "", err }
	s := string(b)
	s = strings.ReplaceAll(s, "{{APP_NAME}}", data.App)
	s = strings.ReplaceAll(s, "{{IMAGE_PATH}}", data.ImagePath)
	s = strings.ReplaceAll(s, "{{DOCKER_IMAGE}}", data.DockerImage)
	out := filepath.Join(os.TempDir(), fmt.Sprintf("%s-lane-%s.hcl", data.App, strings.ToLower(lane)))
	if err := os.WriteFile(out, []byte(s), 0644); err != nil { return "", err }
	return out, nil
}
