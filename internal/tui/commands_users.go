package tui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
)

type usersLoadedMsg struct {
	users   []string
	enabled bool
	err     error
}

func loadUsersCmd(ctx context.Context, client *http.Client, baseURL *url.URL) tea.Cmd {
	return func() tea.Msg {
		result, err := (sharedclient.ListTokensCommand{Client: client, BaseURL: baseURL}).Run(ctx)
		if errors.Is(err, sharedclient.ErrTokenListForbidden) {
			return usersLoadedMsg{}
		}
		if err != nil {
			return usersLoadedMsg{err: err}
		}
		var users []string
		for _, token := range result.Tokens {
			if token.Username != nil {
				if username := strings.TrimSpace(*token.Username); username != "" {
					users = append(users, username)
				}
			}
		}
		slices.Sort(users)
		return usersLoadedMsg{users: slices.Compact(users), enabled: true}
	}
}
