package main

import (
	"errors"
	"runtime"
	"strings"

	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
)

func resolveIdentityPath(value stringValue) (string, error) {
	if value.set {
		trimmed := strings.TrimSpace(value.value)
		if trimmed == "" {
			return "", errors.New("identity path cannot be empty")
		}
		return deploycli.ExpandPath(trimmed), nil
	}
	path := deploycli.DefaultIdentityPath()
	if strings.TrimSpace(path) == "" {
		return "", errors.New("unable to resolve default SSH identity; provide --identity")
	}
	return path, nil
}

func resolvePloydBinaryPath(value stringValue) (string, error) {
	if value.set {
		trimmed := strings.TrimSpace(value.value)
		if trimmed == "" {
			return "", errors.New("ployd binary path cannot be empty")
		}
		return deploycli.ExpandPath(trimmed), nil
	}
	return deploycli.DefaultPloydBinaryPath(runtime.GOOS)
}
