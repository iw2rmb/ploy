package transflow

import (
    "context"
    "encoding/json"
    "fmt"
    "path/filepath"
    "strings"
)

// BranchChainReplayer replays a branch's diff chain from root to head by
// fetching meta and diff patches from a storage base and applying them to repoPath.
type BranchChainReplayer struct {
    GetJSON              func(base, key string) ([]byte, int, error)
    DownloadToFile       func(url, dest string) error
    ValidateDiffPaths    func(diffPath string, allow []string) error
    ValidateUnifiedDiff  func(ctx context.Context, repoPath, diffPath string) error
    ApplyUnifiedDiff     func(ctx context.Context, repoPath, diffPath string) error
    Allowlist            []string
}

// Replay executes the reconstruction for a given execID/branchID
func (r *BranchChainReplayer) Replay(ctx context.Context, storageBase, execID, branchID, outDir, repoPath string) error {
    if r.GetJSON == nil || r.DownloadToFile == nil || r.ValidateDiffPaths == nil || r.ValidateUnifiedDiff == nil || r.ApplyUnifiedDiff == nil {
        return fmt.Errorf("replayer not fully configured")
    }
    // Read HEAD and collect chain from head back to root
    headKey := fmt.Sprintf("transflow/%s/branches/%s/HEAD.json", execID, branchID)
    if b, code, _ := r.GetJSON(storageBase, headKey); code == 200 {
        var head map[string]string
        _ = json.Unmarshal(b, &head)
        cur := head["step_id"]
        chain := []string{}
        for cur != "" {
            chain = append(chain, cur)
            metaKey := fmt.Sprintf("transflow/%s/branches/%s/steps/%s/meta.json", execID, branchID, cur)
            if mb, mc, _ := r.GetJSON(storageBase, metaKey); mc == 200 {
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
        allow := r.Allowlist
        for _, sid := range chain {
            url := strings.TrimRight(storageBase, "/") + "/artifacts/" + fmt.Sprintf("transflow/%s/branches/%s/steps/%s/diff.patch", execID, branchID, sid)
            tmp := filepath.Join(outDir, "chain-"+sid+".patch")
            _ = r.DownloadToFile(url, tmp)
            if err := r.ValidateDiffPaths(tmp, allow); err == nil {
                _ = r.ValidateUnifiedDiff(ctx, repoPath, tmp)
                _ = r.ApplyUnifiedDiff(ctx, repoPath, tmp)
            }
        }
    }
    return nil
}

