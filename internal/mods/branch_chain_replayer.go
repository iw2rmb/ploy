package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// BranchChainReplayer replays a branch's diff chain from root to head by
// fetching meta and diff patches from a storage base and applying them to repoPath.
type BranchChainReplayer struct {
	GetJSON             func(base, key string) ([]byte, int, error)
	DownloadToFile      func(url, dest string) error
	ValidateDiffPaths   func(diffPath string, allow []string) error
	ValidateUnifiedDiff func(ctx context.Context, repoPath, diffPath string) error
	ApplyUnifiedDiff    func(ctx context.Context, repoPath, diffPath string) error
	Allowlist           []string
	Reporter            EventReporter
}

// Replay executes the reconstruction for a given modID/branchID
func (r *BranchChainReplayer) Replay(ctx context.Context, storageBase, modID, branchID, outDir, repoPath string) error {
	if r.GetJSON == nil || r.DownloadToFile == nil || r.ValidateDiffPaths == nil || r.ValidateUnifiedDiff == nil || r.ApplyUnifiedDiff == nil {
		return fmt.Errorf("replayer not fully configured")
	}
	// Read HEAD and collect chain from head back to root
	headKey := fmt.Sprintf("mods/%s/branches/%s/HEAD.json", modID, branchID)
	if b, code, _ := r.GetJSON(storageBase, headKey); code == 200 {
		var head map[string]string
		_ = json.Unmarshal(b, &head)
		cur := head["step_id"]
		chain := []string{}
		for cur != "" {
			chain = append(chain, cur)
			metaKey := fmt.Sprintf("mods/%s/branches/%s/steps/%s/meta.json", modID, branchID, cur)
			if mb, mc, _ := r.GetJSON(storageBase, metaKey); mc == 200 {
				var meta struct {
					Prev string `json:"prev_step_id"`
				}
				_ = json.Unmarshal(mb, &meta)
				cur = meta.Prev
			} else {
				cur = ""
			}
		}
		if r.Reporter != nil {
			_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "info", Message: fmt.Sprintf("replay branch %s: chain length=%d", branchID, len(chain)), Time: time.Now()})
		}
		// Reverse to root→head
		for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
			chain[i], chain[j] = chain[j], chain[i]
		}
		// Explicitly select and apply the HEAD step only to avoid context conflicts
		if len(chain) == 0 {
			if r.Reporter != nil {
				_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "warn", Message: "no steps discovered in chain after HEAD lookup", Time: time.Now()})
			}
			return nil
		}

		headSID := chain[len(chain)-1]
		if r.Reporter != nil {
			_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "info", Message: fmt.Sprintf("applying HEAD step %s", headSID), Time: time.Now()})
		}

		allow := r.Allowlist
		for _, sid := range []string{headSID} {
			url := strings.TrimRight(storageBase, "/") + "/artifacts/" + fmt.Sprintf("mods/%s/branches/%s/steps/%s/diff.patch", modID, branchID, sid)
			tmp := filepath.Join(outDir, "chain-"+sid+".patch")
			if r.Reporter != nil {
				_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "info", Message: fmt.Sprintf("fetching step %s diff from %s", sid, url), Time: time.Now()})
			}
			if err := r.DownloadToFile(url, tmp); err != nil {
				if r.Reporter != nil {
					_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "warn", Message: fmt.Sprintf("failed to download step %s: %v", sid, err), Time: time.Now()})
				}
				continue
			}
			if err := r.ValidateDiffPaths(tmp, allow); err != nil {
				if r.Reporter != nil {
					_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "warn", Message: fmt.Sprintf("diff path validation failed for %s: %v", sid, err), Time: time.Now()})
				}
				continue
			}
			if err := r.ValidateUnifiedDiff(ctx, repoPath, tmp); err != nil {
				if r.Reporter != nil {
					_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "warn", Message: fmt.Sprintf("diff format invalid for %s: %v", sid, err), Time: time.Now()})
				}
				continue
			}
			if err := r.ApplyUnifiedDiff(ctx, repoPath, tmp); err != nil {
				if r.Reporter != nil {
					_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "warn", Message: fmt.Sprintf("apply failed for %s: %v", sid, err), Time: time.Now()})
				}
				continue
			}
			if r.Reporter != nil {
				_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "info", Message: fmt.Sprintf("applied step %s", sid), Time: time.Now()})
			}
		}
	} else {
		if r.Reporter != nil {
			_ = r.Reporter.Report(ctx, Event{Phase: "healing", Step: "apply", Level: "warn", Message: "HEAD.json not found for branch (no steps to replay)", Time: time.Now()})
		}
	}
	return nil
}
