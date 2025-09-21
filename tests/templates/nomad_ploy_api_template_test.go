package templates

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNomadAPITemplateExportsGitlabEnvVars(t *testing.T) {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "unable to determine caller path")
	baseDir := filepath.Dir(filename)
	templatePath := filepath.Join(baseDir, "..", "..", "iac", "common", "templates", "nomad-ploy-api.hcl.j2")
	content, err := os.ReadFile(templatePath)
	require.NoError(t, err)

	text := string(content)
	normalised := shrinkSpaces(text)

	expectedSnippets := []string{
		"GITLAB_URL = \"{{ ploy.gitlab_url }}\"",
		"GITLAB_TOKEN = \"{{ ploy.gitlab_token }}\"",
		"GIT_AUTHOR_NAME = \"{{ ploy.git_author_name }}\"",
		"GIT_AUTHOR_EMAIL = \"{{ ploy.git_author_email }}\"",
		"GIT_COMMITTER_NAME = \"{{ ploy.git_committer_name }}\"",
		"GIT_COMMITTER_EMAIL = \"{{ ploy.git_committer_email }}\"",
		"MODS_ORW_APPLY_IMAGE = \"{{ ploy.mods.orw_apply_image }}\"",
		"MODS_PLANNER_IMAGE = \"{{ ploy.mods.planner_image }}\"",
		"MODS_REDUCER_IMAGE = \"{{ ploy.mods.reducer_image }}\"",
		"MODS_LLM_EXEC_IMAGE = \"{{ ploy.mods.llm_exec_image }}\"",
		"MODS_REGISTRY = \"{{ ploy.mods.registry }}\"",
		"MODS_SKIP_DEPLOY_LANES = \"{{ ploy.mods.skip_deploy_lanes }}\"",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(normalised, snippet) {
			t.Fatalf("nomad template missing expected snippet %q", snippet)
		}
	}

	require.NotContains(t, text, "api.env", "template should no longer reference dedicated api.env file")
}

func shrinkSpaces(in string) string {
	out := in
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return out
}
