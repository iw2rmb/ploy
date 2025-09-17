package build

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// fetchJobLogsFull fetches a broader tail from the newest allocation of the given job.
// It attempts to include both stdout and stderr when supported by the wrapper.
func fetchJobLogsFull(jobName string, lines int) string {
	if jobName == "" {
		return ""
	}
	alloc := runningAlloc(jobName)
	if alloc == "" {
		if ids := getAllocIDs(jobName); len(ids) > 0 {
			alloc = ids[0]
		}
	}
	if alloc == "" {
		return ""
	}
	out := runJobMgrLogs(alloc, lines, true)
	if out == "" {
		out = runJobMgrLogs(alloc, lines, false)
	}
	return out
}

// indirection for testability
var fetchJobLogsFullFn = fetchJobLogsFull

func jobMgrPath() string { return "/opt/hashicorp/bin/nomad-job-manager.sh" }

func runningAlloc(job string) string {
	cmd := exec.Command(jobMgrPath(), "running-alloc", "--job", job)
	b, _ := cmd.CombinedOutput()
	return extractLatestUUID(string(b))
}

func runJobMgrLogs(alloc string, lines int, both bool) string {
	base := []string{"logs", "--alloc-id", alloc, "--lines", fmt.Sprintf("%d", lines)}
	if both {
		base = append(base, "--both")
	}
	for _, task := range candidateLogTasks() {
		args := append(append([]string{}, base...), "--task", task)
		cmd := exec.Command(jobMgrPath(), args...)
		b, err := cmd.CombinedOutput()
		out := string(b)
		if err == nil && !strings.Contains(out, "must provide task name") && !strings.Contains(out, "task not found") {
			return out
		}
	}
	cmd := exec.Command(jobMgrPath(), base...)
	b, _ := cmd.CombinedOutput()
	return string(b)
}

func candidateLogTasks() []string {
	return []string{"kaniko", "build-wasm", "osv-pack", "osv-jvm", "compile", "builder"}
}

func getAllocIDs(job string) []string {
	cmd := exec.Command(jobMgrPath(), "allocs", "--job", job, "--format", "json")
	b, _ := cmd.CombinedOutput()
	type alloc struct {
		ID         string `json:"ID"`
		ModifyTime int64  `json:"ModifyTime"`
	}
	var arr []alloc
	use := jsonPayload(b)
	if len(use) == 0 || json.Unmarshal(use, &arr) != nil || len(arr) == 0 {
		// Fallback to extracting explicit ID fields from whatever text the wrapper emitted.
		reID := regexp.MustCompile(`"ID":"([0-9a-f\-]{36})"`)
		matches := reID.FindAllStringSubmatch(string(b), -1)
		if len(matches) > 0 {
			ids := make([]string, 0, len(matches))
			for _, m := range matches {
				ids = append(ids, m[1])
			}
			return ids
		}
		return extractAllUUIDs(string(b))
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].ModifyTime > arr[j].ModifyTime })
	ids := make([]string, 0, len(arr))
	for _, a := range arr {
		if strings.TrimSpace(a.ID) != "" {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func jsonPayload(raw []byte) []byte {
	s := string(raw)
	start := strings.Index(s, "[")
	if start == -1 {
		return nil
	}
	end := strings.LastIndex(s, "]")
	if end == -1 || end <= start {
		return nil
	}
	return []byte(s[start : end+1])
}

func extractLatestUUID(s string) string {
	uuids := extractAllUUIDs(s)
	if len(uuids) == 0 {
		return ""
	}
	return uuids[len(uuids)-1]
}

func extractAllUUIDs(s string) []string {
	re := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	return re.FindAllString(s, -1)
}
