package platformnomad

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestJetstreamSpecValidatesWithNomadCLI(t *testing.T) {
	content := GetEmbeddedTemplate("platform/nomad/jetstream.nomad.hcl")
	if len(content) == 0 {
		t.Fatalf("jetstream template not embedded")
	}

	bin, err := exec.LookPath("nomad")
	if err != nil {
		t.Skip("nomad CLI not found; skipping validation check")
	}

	workDir := t.TempDir()
	jobPath := filepath.Join(workDir, "jetstream.nomad.hcl")
	if err := os.WriteFile(jobPath, content, 0o600); err != nil {
		t.Fatalf("failed writing temp job file: %v", err)
	}

	cmd := exec.Command(bin, "job", "validate", jobPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("nomad job validate failed: %v\n%s", err, out.String())
	}
}
