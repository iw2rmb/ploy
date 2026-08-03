package handlers

import (
	"context"
	"net/http"
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/server/speccatalog"
)

func TestNamedSpecs_List(t *testing.T) {
	tests := []struct {
		name       string
		catalog    *specCatalogStub
		query      string
		wantStatus int
		wantBody   string
		verify     func(t *testing.T, catalog *specCatalogStub, rrBody domainapi.NamedSpecListResponse)
	}{
		{
			name: "catalog response",
			catalog: &specCatalogStub{entries: []speccatalog.Entry{{
				Name: "upgrade-java", Description: "Upgrade Java", Source: "https://github.com/acme/service",
				Path: "migs/upgrade.yaml", SHA: "0123456789abcdef0123456789abcdef01234567",
			}}},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, catalog *specCatalogStub, body domainapi.NamedSpecListResponse) {
				t.Helper()
				if catalog.calls != 1 {
					t.Fatalf("catalog calls = %d, want 1", catalog.calls)
				}
				if len(body.Specs) != 1 || body.Specs[0].Path != "migs/upgrade.yaml" || body.Specs[0].Source != "https://github.com/acme/service" {
					t.Fatalf("response specs = %+v", body.Specs)
				}
			},
		},
		{name: "empty configuration", catalog: &specCatalogStub{err: speccatalog.ErrNoRepositories}, wantStatus: http.StatusServiceUnavailable, wantBody: "no spec repositories configured"},
		{name: "query rejected", catalog: &specCatalogStub{}, query: "archived=true", wantStatus: http.StatusBadRequest, wantBody: "does not accept query parameters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/v1/specs"
			if tt.query != "" {
				path += "?" + tt.query
			}
			rr := doRequest(t, listNamedSpecsHandler(tt.catalog), http.MethodGet, path, nil)
			assertStatus(t, rr, tt.wantStatus)
			if tt.wantBody != "" {
				assertBodyContains(t, rr, tt.wantBody)
			}
			if tt.verify != nil {
				tt.verify(t, tt.catalog, decodeBody[domainapi.NamedSpecListResponse](t, rr))
			}
		})
	}
}

type specCatalogStub struct {
	entries []speccatalog.Entry
	err     error
	calls   int
}

func (s *specCatalogStub) List(context.Context) ([]speccatalog.Entry, error) {
	s.calls++
	return append([]speccatalog.Entry(nil), s.entries...), s.err
}
