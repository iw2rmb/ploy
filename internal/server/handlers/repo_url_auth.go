package handlers

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func gitAuthOptionsFromSpec(specJSON []byte) gitauth.Options {
	spec, err := contracts.ParseMigSpecJSON(specJSON)
	if err != nil || spec == nil {
		return gitauth.Options{}
	}
	return gitauth.Options{
		GitLabPAT:    strings.TrimSpace(spec.GitLabPAT),
		GitLabDomain: strings.TrimSpace(spec.GitLabDomain),
	}
}
