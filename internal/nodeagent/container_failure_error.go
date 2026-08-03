package nodeagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

const (
	maxDerivedJobErrorLogBytes = 256 * 1024
	maxDerivedJobErrorText     = 2048
)

func deriveContainerExitError(req StartRunRequest, result step.Result, dirs JobDirectories) string {
	if result.ExitCode == 0 {
		return ""
	}
	if msg := amataFailureMessageFromStdout(dirs.Stdout); msg != "" {
		return msg
	}
	if msg := finalMeaningfulLogLine(dirs.Stderr); msg != "" {
		return msg
	}
	return formatContainerExitError(req.JobType, result.ExitCode)
}

func amataFailureMessageFromStdout(stdoutPath string) string {
	data, err := readBoundedTail(stdoutPath, maxDerivedJobErrorLogBytes)
	if err != nil || len(data) == 0 {
		return ""
	}

	var failedStepMessage string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), int(maxDerivedJobErrorLogBytes))
	for scanner.Scan() {
		var event map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(scanner.Bytes()), &event); err != nil {
			continue
		}
		if kind, _ := event["kind"].(string); kind == "run_finished" {
			if msg := errorMessageFromFailureMap(asStringMap(event["failure"])); msg != "" {
				return msg
			}
		}
		stepPayload := asStringMap(event["step"])
		if stepPayload == nil {
			continue
		}
		status, _ := stepPayload["status"].(string)
		if status != "failed" {
			continue
		}
		if msg := errorMessageFromFailureMap(asStringMap(stepPayload["error"])); msg != "" {
			failedStepMessage = msg
		}
	}
	return failedStepMessage
}

func errorMessageFromFailureMap(failure map[string]any) string {
	if failure == nil {
		return ""
	}
	if providerError := asStringMap(asStringMap(failure["details"])["provider_error"]); providerError != nil {
		if msg := boundedErrorText(stringValue(providerError, "message")); msg != "" {
			return msg
		}
	}
	return boundedErrorText(stringValue(failure, "message"))
}

func finalMeaningfulLogLine(stderrPath string) string {
	data, err := readBoundedTail(stderrPath, maxDerivedJobErrorLogBytes)
	if err != nil || len(data) == 0 {
		return ""
	}
	lines := bytes.Split(data, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		line := boundedErrorText(string(bytes.TrimSpace(lines[i])))
		if line != "" {
			return line
		}
	}
	return ""
}

func formatContainerExitError(jobType types.JobType, exitCode int) string {
	name := strings.TrimSpace(jobType.String())
	if name == "" {
		return fmt.Sprintf("job failed with exit code %d", exitCode)
	}
	return fmt.Sprintf("job %s failed with exit code %d", name, exitCode)
}

func readBoundedTail(path string, limit int64) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || info.Size() <= limit {
		return io.ReadAll(file)
	}
	if _, err := file.Seek(info.Size()-limit, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}

func asStringMap(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func stringValue(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func boundedErrorText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if len(text) <= maxDerivedJobErrorText {
		return text
	}
	return text[:maxDerivedJobErrorText]
}
