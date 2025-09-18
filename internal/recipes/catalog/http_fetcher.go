package catalog

import (
	"fmt"
	"io"
	"net/http"
)

// HTTPFetcher downloads OpenRewrite packs (JARs) from a base registry URL.
// Expected layout: {base}/{pack}/{version}.jar
type HTTPFetcher struct {
	BaseURL string
	Client  *http.Client
}

func (f HTTPFetcher) Fetch(pack, version string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s.jar", f.BaseURL, pack, version)
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
