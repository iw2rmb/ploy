package nodeagent

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestAdvanceWorkspaceBaseline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupWorkspace func(t *testing.T, dir string)
		wantCommit     bool
		wantMessage    string
	}{
		{
			name: "commits workspace changes",
			setupWorkspace: func(t *testing.T, dir string) {
				initRepoWithFile(t, dir, "base.txt", "base content\n")
				writeFile(t, filepath.Join(dir, "step0.txt"), "step 0 changes\n")
			},
			wantCommit:  true,
			wantMessage: "Ploy: apply changes",
		},
		{
			name: "no changes is no-op",
			setupWorkspace: func(t *testing.T, dir string) {
				initRepoWithFile(t, dir, "base.txt", "base content\n")
			},
			wantCommit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workspace := t.TempDir()
			tt.setupWorkspace(t, workspace)
			before := strings.TrimSpace(string(gitrepo.Run(t, workspace, "rev-parse", "HEAD")))

			err := advanceWorkspaceBaseline(context.Background(), workspace, types.RunID("run_baseline"), types.JobID("job_baseline"), true)
			checkErr(t, false, err)

			after := strings.TrimSpace(string(gitrepo.Run(t, workspace, "rev-parse", "HEAD")))
			if tt.wantCommit && after == before {
				t.Fatal("expected baseline commit to advance HEAD")
			}
			if !tt.wantCommit && after != before {
				t.Fatal("expected unchanged workspace to keep HEAD")
			}
			if tt.wantMessage != "" {
				logOut := string(gitrepo.Run(t, workspace, "log", "--oneline", "-1"))
				if !strings.Contains(logOut, tt.wantMessage) {
					t.Errorf("expected baseline commit message to contain %q, got: %s", tt.wantMessage, logOut)
				}
			}
			statusOut := string(gitrepo.Run(t, workspace, "status", "--porcelain"))
			if statusOut != "" {
				t.Errorf("expected clean working tree, got: %s", statusOut)
			}
		})
	}
}

func TestStickyWorkspaceDiffsStayIncrementalAfterBaselineAdvance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workspace := t.TempDir()
	initRepoWithFile(t, workspace, "counter.txt", "0\n")

	type stepChange struct {
		counterValue   string
		extraFiles     map[string]string
		mustContain    []string
		mustNotContain []string
	}

	steps := []stepChange{
		{
			counterValue: "1\n",
			mustContain:  []string{"-0", "+1"},
		},
		{
			counterValue:   "2\n",
			mustContain:    []string{"-1", "+2"},
			mustNotContain: []string{"-0"},
		},
		{
			counterValue:   "3\n",
			extraFiles:     map[string]string{"added.txt": "hello from step 2\n"},
			mustContain:    []string{"-2", "+3", "+hello from step 2"},
			mustNotContain: []string{"-0\n", "-1\n"},
		},
	}

	for i, change := range steps {
		writeFile(t, filepath.Join(workspace, "counter.txt"), change.counterValue)
		for file, content := range change.extraFiles {
			writeFile(t, filepath.Join(workspace, file), content)
		}

		diff, err := step.NewFilesystemDiffGenerator().Generate(ctx, workspace)
		if err != nil {
			t.Fatalf("step %d: generate diff: %v", i, err)
		}
		for _, s := range change.mustContain {
			if !bytes.Contains(diff, []byte(s)) {
				t.Errorf("step %d: diff should contain %q, got:\n%s", i, s, diff)
			}
		}
		for _, s := range change.mustNotContain {
			if bytes.Contains(diff, []byte(s)) {
				t.Errorf("step %d: diff should NOT contain %q, got:\n%s", i, s, diff)
			}
		}

		if err := advanceWorkspaceBaseline(ctx, workspace, types.RunID("run_incremental"), types.JobID("job_incremental"), true); err != nil {
			t.Fatalf("step %d: advance baseline: %v", i, err)
		}
	}

	assertFileContent(t, filepath.Join(workspace, "counter.txt"), "3\n")
	assertFileContent(t, filepath.Join(workspace, "added.txt"), "hello from step 2\n")
}
