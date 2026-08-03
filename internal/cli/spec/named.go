package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/common"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("list named specs: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list named specs: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, common.ControlPlaneHTTPError(resp)
	}
	defer func() { _ = resp.Body.Close() }()
	var list domainapi.NamedSpecListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("list named specs: decode response: %w", err)
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
