package nodeagent

import (
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

// initRepoWithFile initializes a git repo, writes a single file, and commits.
func initRepoWithFile(t *testing.T, dir, file, content string) {
	t.Helper()
	gitrepo.Init(t, dir)
	gitrepo.WriteFile(t, filepath.Join(dir, file), content)
	gitrepo.CommitAll(t, dir, "initial commit")
}
