package guards

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGradleSBOMCollectorAwkTerminatesAndParses(t *testing.T) {
	t.Parallel()

	repoRoot := mustFindRepoRoot(t)
	scriptPath := filepath.Join(repoRoot, "images", "gates", "shared", "collect-java-classpath-gradle.sh")
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}

	awkProgram := extractGradleSBOMAwkProgram(t, string(raw))
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no override",
			input: "org.slf4j:slf4j-api:1.7.30\n",
			want:  "org.slf4j:slf4j-api\t1.7.30\n",
		},
		{
			name:  "with override",
			input: "|    +--- org.slf4j:slf4j-api:1.7.30 -> 2.0.16\n",
			want:  "org.slf4j:slf4j-api\t2.0.16\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "awk", awkProgram)
			cmd.Stdin = strings.NewReader(tc.input)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			out, err := cmd.Output()
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("awk timed out; probable infinite loop. input=%q", tc.input)
			}
			if err != nil {
				t.Fatalf("awk failed: %v, stderr=%q", err, stderr.String())
			}
			if got := string(out); got != tc.want {
				t.Fatalf("awk output mismatch\ninput: %q\ngot:  %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func extractGradleSBOMAwkProgram(t *testing.T, script string) string {
	t.Helper()

	startNeedle := "awk '\n"
	endNeedle := "\n' \"$deps_raw\" | sort -u > \"$deps_pairs\""
	start := strings.Index(script, startNeedle)
	if start < 0 {
		t.Fatalf("awk program start marker not found")
	}
	start += len(startNeedle)
	end := strings.Index(script[start:], endNeedle)
	if end < 0 {
		t.Fatalf("awk program end marker not found")
	}
	return script[start : start+end]
}
