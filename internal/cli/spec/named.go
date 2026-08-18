package spec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/common"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

func handleList(args []string, stdout, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printListUsage(stderr)
		return nil
	}
	if len(args) > 0 {
		printListUsage(stderr)
		return errors.New("spec ls takes no arguments")
	}

	ctx := context.Background()
	base, client, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	specs, err := listNamedSpecs(ctx, base, client)
	if err != nil {
		return err
	}
	renderListResults(stdout, specs)
	return nil
}

func printListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy spec ls")
}

func listNamedSpecs(ctx context.Context, base *url.URL, client *http.Client) ([]domainapi.NamedSpecCatalogEntry, error) {
	endpoint := base.JoinPath("v1", "specs")
	list, err := httpx.DoJSON[domainapi.NamedSpecListResponse](ctx, client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list named specs")
	if err != nil {
		return nil, err
	}
	return list.Specs, nil
}

func renderListResults(out io.Writer, specs []domainapi.NamedSpecCatalogEntry) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSOURCE\tPATH\tSHA")
	for _, spec := range specs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", spec.Name, domaintypes.NormalizeRepoURLSchemless(spec.Source), spec.Path, shortSHA(spec.SHA))
	}
	_ = w.Flush()
}

func shortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}
