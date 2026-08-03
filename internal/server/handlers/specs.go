package handlers

import (
	"context"
	"net/http"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/server/speccatalog"
)

func listNamedSpecsHandler(catalog specCatalogLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			writeHTTPError(w, http.StatusBadRequest, "spec listing does not accept query parameters")
			return
		}
		entries, err := catalog.List(r.Context())
		if err != nil {
			writeHTTPError(w, http.StatusServiceUnavailable, "%s", err)
			return
		}
		summaries := make([]domainapi.NamedSpecCatalogEntry, 0, len(entries))
		for _, entry := range entries {
			summaries = append(summaries, domainapi.NamedSpecCatalogEntry{
				Name: entry.Name, Description: entry.Description, Source: entry.Source, Path: entry.Path, SHA: entry.SHA,
			})
		}
		writeJSON(w, http.StatusOK, domainapi.NamedSpecListResponse{Specs: summaries})
	}
}

type specCatalogLister interface {
	List(context.Context) ([]speccatalog.Entry, error)
}
