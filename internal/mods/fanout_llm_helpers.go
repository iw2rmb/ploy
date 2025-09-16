package mods

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// firstJavaPathFromError extracts the first token that looks like a Java source path from an error string.
func firstJavaPathFromError(s string) string {
	re := regexp.MustCompile(`([A-Za-z0-9_./\\-]+\.java)`) // greedy enough to catch paths; includes Windows separators
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		// Normalize backslashes to slashes
		p := strings.ReplaceAll(m[1], "\\", "/")
		return p
	}
	return ""
}

// extractJavaPathsFromError returns up to max unique .java paths found in the error text.
func extractJavaPathsFromError(s string, max int) []string {
	re := regexp.MustCompile(`([A-Za-z0-9_./\\-]+\.java)`) // includes Windows separators
	matches := re.FindAllStringSubmatch(s, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		p := strings.ReplaceAll(m[1], "\\", "/")
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

// parseClassNamesFromError extracts up to max class names mentioned in common compiler messages
func parseClassNamesFromError(s string, max int) []string {
	var out []string
	seen := make(map[string]struct{})
	// Patterns: "symbol: class Foo", "class Foo", "cannot find symbol Foo"
	pats := []*regexp.Regexp{
		regexp.MustCompile(`symbol:\s*class\s+([A-Za-z0-9_]+)`),
		regexp.MustCompile(`class\s+([A-Za-z0-9_]+)\b`),
		regexp.MustCompile(`cannot\s+find\s+symbol\s+([A-Za-z0-9_]+)\b`),
	}
	for _, re := range pats {
		ms := re.FindAllStringSubmatch(s, -1)
		for _, m := range ms {
			if len(m) < 2 {
				continue
			}
			name := m[1]
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
			if max > 0 && len(out) >= max {
				return out
			}
		}
	}
	return out
}

// findJavaFilesByBasename walks repoRoot/src and returns relative paths for files matching basenames.
func findJavaFilesByBasename(repoRoot string, names []string, max int) []string {
	if len(names) == 0 {
		return nil
	}
	want := make(map[string]struct{})
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			continue
		}
		want[n+".java"] = struct{}{}
	}
	var out []string
	root := filepath.Join(repoRoot, "src")
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if _, ok := want[base]; ok {
			rel, _ := filepath.Rel(repoRoot, path)
			rel = filepath.ToSlash(rel)
			out = append(out, rel)
			if max > 0 && len(out) >= max {
				return io.EOF
			}
		}
		return nil
	})
	return out
}

// parseMCPFromInputs converts map[string]interface{} to MCPConfig struct
func parseMCPFromInputs(inputs map[string]interface{}) (*MCPConfig, error) {
	config := &MCPConfig{}

	// Parse tools
	if toolsData, ok := inputs["tools"]; ok {
		if toolsList, ok := toolsData.([]interface{}); ok {
			for _, toolData := range toolsList {
				if toolMap, ok := toolData.(map[string]interface{}); ok {
					tool := MCPTool{}
					if name, ok := toolMap["name"].(string); ok {
						tool.Name = name
					}
					if endpoint, ok := toolMap["endpoint"].(string); ok {
						tool.Endpoint = endpoint
					}
					if configData, ok := toolMap["config"].(map[string]interface{}); ok {
						tool.Config = make(map[string]string)
						for k, v := range configData {
							if vStr, ok := v.(string); ok {
								tool.Config[k] = vStr
							}
						}
					}
					config.Tools = append(config.Tools, tool)
				}
			}
		}
	}

	// Parse context
	if contextData, ok := inputs["context"]; ok {
		if contextList, ok := contextData.([]interface{}); ok {
			for _, ctxData := range contextList {
				if ctxStr, ok := ctxData.(string); ok {
					config.Context = append(config.Context, ctxStr)
				}
			}
		}
	}

	// Parse prompts
	if promptsData, ok := inputs["prompts"]; ok {
		if promptsList, ok := promptsData.([]interface{}); ok {
			for _, promptData := range promptsList {
				if promptStr, ok := promptData.(string); ok {
					config.Prompts = append(config.Prompts, promptStr)
				}
			}
		}
	}

	// Parse model
	if model, ok := inputs["model"].(string); ok {
		config.Model = model
	}

	// Parse budgets
	if budgetsData, ok := inputs["budgets"]; ok {
		if budgetsMap, ok := budgetsData.(map[string]interface{}); ok {
			if maxTokens, ok := budgetsMap["max_tokens"].(int); ok {
				config.Budgets.MaxTokens = maxTokens
			}
			if maxCost, ok := budgetsMap["max_cost"].(int); ok {
				config.Budgets.MaxCost = maxCost
			}
			if timeout, ok := budgetsMap["timeout"].(string); ok {
				config.Budgets.Timeout = timeout
			}
		}
	}

	return config, nil
}
