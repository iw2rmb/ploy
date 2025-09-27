package helper

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/grid/workflowrpc"
)

const (
	defaultRetries = 3
	defaultBackoff = 50 * time.Millisecond
	authorization  = "Authorization"
	bearerPrefix   = "Bearer "
)

// Options controls helper-backed Workflow RPC client construction.
type Options struct {
	Endpoint    string
	HTTPClient  *http.Client
	BearerToken string
	Retries     int
	Backoff     time.Duration
}

// Helper wraps the Workflow RPC client with helper semantics (auth headers, retries).
type Helper struct {
	client  workflowRPCClient
	retries int
	backoff time.Duration
}

type workflowRPCClient interface {
	Submit(ctx context.Context, req workflowrpc.SubmitRequest) (workflowrpc.SubmitResponse, error)
}

// New constructs a helper-backed Workflow RPC submitter.
func New(opts Options) (*Helper, error) {
	httpClient := cloneHTTPClient(opts.HTTPClient)
	token := strings.TrimSpace(opts.BearerToken)
	if token != "" {
		transport := httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		httpClient.Transport = &authRoundTripper{token: token, next: transport}
	}

	client, err := workflowrpc.NewClient(workflowrpc.Options{Endpoint: opts.Endpoint, HTTPClient: httpClient})
	if err != nil {
		return nil, err
	}

	retries := opts.Retries
	if retries <= 0 {
		retries = defaultRetries
	}
	backoff := opts.Backoff
	if backoff <= 0 {
		backoff = defaultBackoff
	}

	return &Helper{client: client, retries: retries, backoff: backoff}, nil
}

// Submit dispatches the workflow submission request, retrying retryable failures.
func (h *Helper) Submit(ctx context.Context, req workflowrpc.SubmitRequest) (workflowrpc.SubmitResponse, error) {
	var lastErr error
	for attempt := 0; attempt < h.retries; attempt++ {
		resp, err := h.client.Submit(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable(err) || attempt == h.retries-1 {
			break
		}
		select {
		case <-time.After(h.backoff * time.Duration(attempt+1)):
			continue
		case <-ctx.Done():
			return workflowrpc.SubmitResponse{}, ctx.Err()
		}
	}
	return workflowrpc.SubmitResponse{}, lastErr
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *workflowrpc.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Retryable()
	}
	return false
}

type authRoundTripper struct {
	token string
	next  http.RoundTripper
}

func (rt *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set(authorization, bearerPrefix+rt.token)
	return rt.next.RoundTrip(cloned)
}

func cloneHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		return &http.Client{}
	}
	return &http.Client{
		Transport:     base.Transport,
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
		Timeout:       base.Timeout,
	}
}
