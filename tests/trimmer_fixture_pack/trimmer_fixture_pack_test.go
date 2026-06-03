package trimmerfixturepack

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/trimmer/java/gradle"
)

type reportItem struct {
	Source                string   `json:"source"`
	OutputLog             string   `json:"output_log"`
	OutputJSON            string   `json:"output_json"`
	RawBytes              int      `json:"raw_bytes"`
	TrimmedBytes          int      `json:"trimmed_bytes"`
	RawLines              int      `json:"raw_lines"`
	TrimmedLines          int      `json:"trimmed_lines"`
	ReductionPct          float64  `json:"reduction_pct"`
	Message               bool     `json:"message"`
	Evidence              bool     `json:"evidence"`
	EvidenceTask          string   `json:"evidence_task,omitempty"`
	EvidenceErrors        int      `json:"evidence_errors,omitempty"`
	ResidualException     bool     `json:"residual_exception"`
	ResidualTry           bool     `json:"residual_try"`
	ResidualDeprec        bool     `json:"residual_deprecation_footer"`
	ResidualCITail        bool     `json:"residual_ci_tail"`
	ResidualProgressNoise bool     `json:"residual_progress_noise"`
	KotlinIssueLines      int      `json:"kotlin_issue_lines"`
	JavaIssueLines        int      `json:"java_issue_lines"`
	FirstMessageLines     []string `json:"first_message_lines"`
	ContainsBuildFail     bool     `json:"contains_build_failed"`
	ContainsActionable    bool     `json:"contains_actionable_tasks"`
}

type report struct {
	Count int          `json:"count"`
	Items []reportItem `json:"items"`
}

var (
	javaIssueRe       = regexp.MustCompile(`(?m)^/workspace/.*\.java:[0-9]+: error: `)
	kotlinIssueRe     = regexp.MustCompile(`(?m)^e: (?:file://)?/workspace/.*\.kt:[0-9]+:[0-9]+ `)
	actionableTasksRe = regexp.MustCompile(`(?m)^[0-9]+ actionable tasks?: .+$`)
)

// Corpus fixture validation is intentionally one batch test: the assertion is
// the aggregate contract between raw logs, committed trimmed outputs, and report.
func TestGradleTrimmerFixturePack(t *testing.T) {
	t.Parallel()

	items := trimFixturePack(t)
	gotReport := marshalReport(t, report{Count: len(items), Items: items})

	if os.Getenv("PLOY_UPDATE_TRIMMER_FIXTURES") == "1" {
		writeFixtureOutputs(t, items, gotReport)
		return
	}

	assertFixtureOutputs(t, items, gotReport)
	assertFixtureQuality(t, items)
}

func trimFixturePack(t *testing.T) []reportItem {
	t.Helper()

	paths := collectLogs(t)
	items := make([]reportItem, 0, len(paths))
	for _, source := range paths {
		items = append(items, trimOne(t, source))
	}
	return items
}

func collectLogs(t *testing.T) []string {
	t.Helper()

	var paths []string
	for _, pattern := range []string{
		filepath.Join("logs", "*.log"),
		filepath.Join("public", "logs", "*.log"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	if len(paths) != 10 {
		t.Fatalf("fixture count = %d, want 10", len(paths))
	}
	return paths
}

func trimOne(t *testing.T, source string) reportItem {
	t.Helper()

	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	result := gradle.Trim(string(raw))
	outLog, outJSON := outputPaths(source)

	item := reportItem{
		Source:                source,
		OutputLog:             outLog,
		OutputJSON:            outJSON,
		RawBytes:              len(raw),
		TrimmedBytes:          len(result.Message),
		RawLines:              countLines(string(raw)),
		TrimmedLines:          countLines(result.Message),
		Message:               strings.TrimSpace(result.Message) != "",
		ResidualException:     strings.Contains(result.Message, "* Exception is:"),
		ResidualTry:           strings.Contains(result.Message, "* Try:"),
		ResidualDeprec:        strings.Contains(result.Message, "Deprecated Gradle features were used in this build"),
		ResidualCITail:        containsCITail(result.Message),
		ResidualProgressNoise: containsGradleProgressNoise(result.Message),
		KotlinIssueLines:      len(kotlinIssueRe.FindAllString(result.Message, -1)),
		JavaIssueLines:        len(javaIssueRe.FindAllString(result.Message, -1)),
		FirstMessageLines:     firstNonEmptyLines(result.Message, 8),
		ContainsBuildFail:     strings.Contains(result.Message, "BUILD FAILED"),
		ContainsActionable:    actionableTasksRe.MatchString(result.Message),
	}
	if item.RawBytes > 0 {
		item.ReductionPct = 100 * (1 - float64(item.TrimmedBytes)/float64(item.RawBytes))
	}
	if result.Evidence != nil {
		item.Evidence = true
		item.EvidenceTask = result.Evidence.Task
		item.EvidenceErrors = len(result.Evidence.Errors)
	}

	return item
}

func outputPaths(source string) (string, string) {
	name := strings.TrimSuffix(filepath.Base(source), ".log")
	if strings.HasPrefix(source, filepath.Join("public", "logs")+string(filepath.Separator)) {
		base := filepath.Join("public", "trimmed", name+".trimmed")
		return base + ".log", base + ".json"
	}
	base := filepath.Join("trimmed", name+".trimmed")
	return base + ".log", base + ".json"
}

func writeFixtureOutputs(t *testing.T, items []reportItem, reportData []byte) {
	t.Helper()

	for _, item := range items {
		raw, err := os.ReadFile(item.Source)
		if err != nil {
			t.Fatalf("read %s: %v", item.Source, err)
		}
		result := gradle.Trim(string(raw))
		if err := os.WriteFile(item.OutputLog, []byte(result.Message), 0o644); err != nil {
			t.Fatalf("write %s: %v", item.OutputLog, err)
		}
		if err := os.WriteFile(item.OutputJSON, marshalResult(t, result), 0o644); err != nil {
			t.Fatalf("write %s: %v", item.OutputJSON, err)
		}
	}
	if err := os.WriteFile("trim_batch_report.json", reportData, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

func assertFixtureOutputs(t *testing.T, items []reportItem, reportData []byte) {
	t.Helper()

	for _, item := range items {
		raw, err := os.ReadFile(item.Source)
		if err != nil {
			t.Fatalf("read %s: %v", item.Source, err)
		}
		result := gradle.Trim(string(raw))
		assertFileEqual(t, item.OutputLog, []byte(result.Message))
		assertFileEqual(t, item.OutputJSON, marshalResult(t, result))
	}
	assertFileEqual(t, "trim_batch_report.json", reportData)
}

func assertFileEqual(t *testing.T, path string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read expected %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale; run PLOY_UPDATE_TRIMMER_FIXTURES=1 go test ./tests/trimmer_fixture_pack", path)
	}
}

func assertFixtureQuality(t *testing.T, items []reportItem) {
	t.Helper()

	for _, item := range items {
		if !item.Evidence {
			t.Errorf("%s: expected evidence", item.Source)
		}
		if strings.HasPrefix(item.Source, "logs"+string(filepath.Separator)) && item.EvidenceTask == "" {
			t.Errorf("%s: expected structured compiler evidence", item.Source)
		}
		if item.Message && !item.ContainsBuildFail {
			t.Errorf("%s: expected BUILD FAILED in compact message", item.Source)
		}
		if !item.Message && item.EvidenceTask == "" {
			t.Errorf("%s: expected root message for fallback evidence", item.Source)
		}
		if item.ResidualException {
			t.Errorf("%s: compact message still contains * Exception is:", item.Source)
		}
		if item.ResidualTry {
			t.Errorf("%s: compact message still contains * Try:", item.Source)
		}
		if item.ResidualDeprec {
			t.Errorf("%s: compact message still contains Gradle deprecation footer", item.Source)
		}
		if item.ResidualCITail {
			t.Errorf("%s: compact message still contains CI cleanup tail", item.Source)
		}
		if item.ResidualProgressNoise {
			t.Errorf("%s: compact message still contains interleaved Gradle progress noise", item.Source)
		}
	}
}

func marshalResult(t *testing.T, result gradle.Result) []byte {
	t.Helper()

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return append(data, '\n')
}

func marshalReport(t *testing.T, r report) []byte {
	t.Helper()

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	return append(data, '\n')
}

func containsCITail(text string) bool {
	tails := []string{
		"Post job cleanup",
		"actions/upload-artifact",
		"Uploading artifacts",
		"Process completed with exit code",
		"gradle/actions: Writing build results",
		"Found dependency graph files:",
	}
	for _, tail := range tails {
		if strings.Contains(text, tail) {
			return true
		}
	}
	return false
}

func containsGradleProgressNoise(text string) bool {
	noise := []string{
		"Cached resource ",
		"Failed to get resource: HEAD.",
		"Build cache key for task ",
		"Skipping task ",
	}
	for _, marker := range noise {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return len(strings.Split(text, "\n"))
}

func firstNonEmptyLines(text string, n int) []string {
	lines := make([]string, 0, n)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) == n {
			break
		}
	}
	return lines
}
