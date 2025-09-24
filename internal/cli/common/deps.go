package common

import (
	"io"
	"net/http"
	"os"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// HTTPDoer describes the minimal interface required for HTTP clients used by SharedPush.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// TarBuilderFunc streams the working directory into the provided writer using the supplied ignore rules.
type TarBuilderFunc func(dir string, w io.Writer, ign utils.Ignore, opts utils.TarOptions) error

// GitignoreReader loads ignore patterns for a working directory.
type GitignoreReader func(dir string) (utils.Ignore, error)

// AutogenDockerfileFunc produces a Dockerfile for simple stacks when no Dockerfile exists.
type AutogenDockerfileFunc func(dir string) error

// SharedPushDeps captures overridable dependencies for SharedPush. Callers may provide a partial struct;
// any nil field falls back to the default implementation.
type SharedPushDeps struct {
	HTTPClient        HTTPDoer
	TarBuilder        TarBuilderFunc
	ReadGitignore     GitignoreReader
	AutogenDockerfile AutogenDockerfileFunc
	Stdout            io.Writer
}

type resolvedDeps struct {
	httpClient        HTTPDoer
	tarBuilder        TarBuilderFunc
	readGitignore     GitignoreReader
	autogenDockerfile AutogenDockerfileFunc
	stdout            io.Writer
}

func resolveSharedPushDeps(cfg *SharedPushDeps) resolvedDeps {
	if cfg == nil {
		return resolvedDeps{
			httpClient:        &http.Client{},
			tarBuilder:        utils.TarDirWithOptions,
			readGitignore:     utils.ReadGitignore,
			autogenDockerfile: tryAutogenDockerfile,
			stdout:            os.Stdout,
		}
	}

	deps := resolvedDeps{
		httpClient:        cfg.HTTPClient,
		tarBuilder:        cfg.TarBuilder,
		readGitignore:     cfg.ReadGitignore,
		autogenDockerfile: cfg.AutogenDockerfile,
		stdout:            cfg.Stdout,
	}

	if deps.httpClient == nil {
		deps.httpClient = &http.Client{}
	}
	if deps.tarBuilder == nil {
		deps.tarBuilder = utils.TarDirWithOptions
	}
	if deps.readGitignore == nil {
		deps.readGitignore = utils.ReadGitignore
	}
	if deps.autogenDockerfile == nil {
		deps.autogenDockerfile = tryAutogenDockerfile
	}
	if deps.stdout == nil {
		deps.stdout = os.Stdout
	}
	return deps
}
