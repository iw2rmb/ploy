package httpx

import (
	"fmt"
	"net/http"
)

// NewHeaderTransport returns a transport that overlays headers without modifying the caller's request.
func NewHeaderTransport(base http.RoundTripper, headers http.Header) http.RoundTripper {
	return &headerTransport{
		base:    base,
		headers: headers.Clone(),
	}
}

type headerTransport struct {
	base    http.RoundTripper
	headers http.Header
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	// Copy only the request and headers because Request.Clone has been unstable
	// in the node runtime under load.
	reqCopy := new(http.Request)
	*reqCopy = *req
	if req.Header != nil {
		reqCopy.Header = req.Header.Clone()
	} else {
		reqCopy.Header = make(http.Header, len(t.headers))
	}
	for key, values := range t.headers {
		reqCopy.Header.Del(key)
		for _, value := range values {
			reqCopy.Header.Add(key, value)
		}
	}
	return base.RoundTrip(reqCopy)
}
