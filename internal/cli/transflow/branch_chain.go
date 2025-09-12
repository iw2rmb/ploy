package transflow

import (
    "context"
    "encoding/json"
    "fmt"
    "path/filepath"
    "strings"
)

// reconstructBranchState replays previous step diffs in a branch from root to HEAD.
// Best-effort: skips failures silently to match previous behavior.
func (r *TransflowRunner) reconstructBranchState(ctx context.Context, seaweed, execID, branchID, baseDir, repoPath string) error {
    headKey := fmt.Sprintf("transflow/%s/branches/%s/HEAD.json", execID, branchID)
    if b, code, _ := getJSONFn(seaweed, headKey); code == 200 {
        var head map[string]string
        _ = json.Unmarshal(b, &head)
        cur := head["step_id"]
        // Collect step_ids from head back to root
        chain := []string{}
        for cur != "" {
            chain = append(chain, cur)
            metaKey := fmt.Sprintf("transflow/%s/branches/%s/steps/%s/meta.json", execID, branchID, cur)
            if mb, mc, _ := getJSONFn(seaweed, metaKey); mc == 200 {
                var meta struct{ Prev string `json:"prev_step_id"` }
                _ = json.Unmarshal(mb, &meta)
                cur = meta.Prev
            } else {
                cur = ""
            }
        }
        // Reverse to root→head
        for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
            chain[i], chain[j] = chain[j], chain[i]
        }
        // Apply each recorded diff in order
        allow := ResolveDefaultsFromEnv().Allowlist
        for _, sid := range chain {
            url := strings.TrimRight(seaweed, "/") + "/artifacts/" + fmt.Sprintf("transflow/%s/branches/%s/steps/%s/diff.patch", execID, branchID, sid)
            tmp := filepath.Join(baseDir, "out", "chain-"+sid+".patch")
            _ = downloadToFileFn(url, tmp)
            if err := validateDiffPathsFn(tmp, allow); err == nil {
                _ = validateUnifiedDiffFn(ctx, repoPath, tmp)
                _ = applyUnifiedDiffFn(ctx, repoPath, tmp)
            }
        }
    }
    return nil
}
