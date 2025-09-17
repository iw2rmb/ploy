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
	re := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	all := re.FindAllString(string(b), -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
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
	if err := json.Unmarshal(b, &arr); err != nil || len(arr) == 0 {
		re := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
		return re.FindAllString(string(b), -1)
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
