package gitexec

import (
	"crypto/sha1"
	"fmt"
)

// RepositorySHA identifies a changed repository tree using Ploy's fixed commit
// metadata. It does not create a Git object or identify a no-op transition.
func RepositorySHA(tree, parent string) string {
	content := fmt.Sprintf("tree %s\nparent %s\nauthor node <node@ploy.local> 0 +0000\ncommitter node <node@ploy.local> 0 +0000\n\nploy repo_sha_v1\n", tree, parent)
	return fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("commit %d\x00%s", len(content), content))))
}
