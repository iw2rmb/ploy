package main

import (
	"io"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/runs"
)

// followRunRenderOptions returns shared render options for follow-mode output.
func followRunRenderOptions(baseURL *url.URL, output io.Writer) runs.TextRenderOptions {
	token, _ := resolveControlPlaneToken()
	return runs.TextRenderOptions{
		EnableOSC8: runStatusSupportsOSC8(output),
		AuthToken:  token,
		BaseURL:    baseURL,
	}
}
