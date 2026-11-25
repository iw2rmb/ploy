package tui

import (
	"net/url"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/iw2rmb/ploy/internal/cli/common"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func runDetailItems(run domaintypes.RunSummary, nodeName string, links bool) []list.Item {
	repo := common.FormatRunListRepo(run.RepoURL, run.SourceCommitSHA)
	if target := runCommitURL(run.RepoURL, run.SourceCommitSHA); target != "" && links {
		repo = lipgloss.NewStyle().Hyperlink(target).Render(repo)
	}
	return []list.Item{
		listItem{title: "ID", description: run.ID.String()},
		listItem{title: "STATUS", description: string(run.Status)},
		listItem{title: "SPEC", description: common.FormatRunListSpecDetail(run)},
		listItem{title: "REPO", description: repo},
		listItem{title: "NODE", description: normalizeLabel(nodeName)},
	}
}

func runCommitURL(repoURL, sha string) string {
	commit, ok := domaintypes.NormalizeFullCommitSHA(sha)
	raw := domaintypes.NormalizeRepoURL(repoURL)
	if !strings.Contains(raw, "://") {
		host, path, found := strings.Cut(raw, ":")
		if !found || strings.Contains(host, "/") {
			return ""
		}
		raw = "ssh://" + host + "/" + path
	}
	u, err := url.Parse(raw)
	if !ok || err != nil || u.Hostname() == "" || u.Path == "" {
		return ""
	}
	switch u.Scheme {
	case "https":
	case "ssh":
		u.Scheme = "https"
		u.Host = u.Hostname()
	default:
		return ""
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), ".git")
	if strings.EqualFold(u.Hostname(), "github.com") {
		return u.JoinPath("commit", commit.String()).String()
	}
	return u.JoinPath("-", "commit", commit.String()).String()
}
