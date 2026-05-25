package common

import (
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/runs"
)

// FollowRunRenderOptions returns shared render options for follow-mode output.
func FollowRunRenderOptions(baseURL *url.URL, output io.Writer) runs.TextRenderOptions {
	token, _ := ResolveControlPlaneToken()
	return runs.TextRenderOptions{
		EnableOSC8: SupportsOSC8(output),
		AuthToken:  token,
		BaseURL:    baseURL,
	}
}

func SupportsOSC8(w io.Writer) bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" || strings.EqualFold(term, "dumb") {
		return false
	}

	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
