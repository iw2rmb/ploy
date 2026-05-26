package handlers

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

func gitAuthOptionsFromConfig(cfg config.GitLabConfig) gitauth.Options {
	return gitauth.Options{
		GitLabPAT:    strings.TrimSpace(cfg.Token),
		GitLabDomain: strings.TrimSpace(cfg.Domain),
	}
}
