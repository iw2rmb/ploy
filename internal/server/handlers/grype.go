package handlers

import (
	"context"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

func grypeDatabaseHandler(transport http.RoundTripper) http.HandlerFunc {
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			out := r.Out
			out.URL.Scheme = "https"
			out.URL.Host = "grype.anchore.io"
			out.Host = out.URL.Host
			out.URL.Path = "/databases/" + r.In.PathValue("path")
			out.URL.RawPath = ""
			query := out.URL.Query()
			query.Del("auth_token")
			out.URL.RawQuery = query.Encode()

			// Only forward download headers because Ploy credentials belong to this server.
			headers := make(http.Header)
			for _, key := range []string{"Accept", "Accept-Encoding", "Range", "If-Range", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since"} {
				if values := out.Header.Values(key); len(values) > 0 {
					headers[key] = values
				}
			}
			out.Header = headers
			out.Body = nil
			out.ContentLength = 0
			out.TransferEncoding = nil
			out.Trailer = nil
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		for segment := range strings.SplitSeq(r.PathValue("path"), "/") {
			if segment == "." || segment == ".." {
				writeHTTPError(w, http.StatusBadRequest, "invalid database path")
				return
			}
		}
		// Database archives can exceed the server's default 15-second write deadline.
		deadline := time.Now().Add(15 * time.Minute)
		ctx, cancel := context.WithDeadline(r.Context(), deadline)
		defer cancel()
		if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to set response deadline")
			return
		}
		proxy.ServeHTTP(w, r.WithContext(ctx))
	}
}
