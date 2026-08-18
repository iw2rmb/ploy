package httpx

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	MaxErrorBodyBytes    int64 = 2048
	MaxJSONBodyBytes     int64 = 1 << 20
	MaxDownloadBodyBytes int64 = 64 << 20
	MaxGunzipOutputBytes int64 = 256 << 20
)

// DoJSON executes one bounded JSON request and requires exactly one expected status.
func DoJSON[T any](ctx context.Context, client *http.Client, method, endpoint string, body any, expectedStatus int, action string) (T, error) {
	var zero T
	if client == nil {
		return zero, fmt.Errorf("%s: http client required", action)
	}

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return zero, fmt.Errorf("%s: encode request: %w", action, err)
		}
		requestBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return zero, fmt.Errorf("%s: build request: %w", action, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return zero, fmt.Errorf("%s: http request: %w", action, err)
	}
	defer DrainAndClose(resp)

	if resp.StatusCode != expectedStatus {
		msg := ReadErrorMessage(resp.Body, resp.Status, MaxErrorBodyBytes)
		return zero, fmt.Errorf("%s: unexpected status %d: %s", action, resp.StatusCode, msg)
	}

	if err := DecodeResponseJSON(resp.Body, &zero, MaxJSONBodyBytes); err != nil {
		return zero, fmt.Errorf("%s: decode response: %w", action, err)
	}
	return zero, nil
}

func DecodeResponseJSON(r io.Reader, out any, limit int64) error {
	if limit <= 0 {
		limit = MaxJSONBodyBytes
	}
	lr := &io.LimitedReader{R: r, N: limit + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("response exceeds %d bytes", limit)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errMultipleJSONValues
		}
		return err
	}
	return nil
}

var errMultipleJSONValues = errors.New("response contains multiple JSON values")

func ReadErrorMessage(r io.Reader, status string, limit int64) string {
	if limit <= 0 {
		limit = MaxErrorBodyBytes
	}
	data, _ := io.ReadAll(io.LimitReader(r, limit))
	body := strings.TrimSpace(string(data))
	if body == "" {
		return status
	}

	var apiErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &apiErr); err == nil {
		msg := strings.TrimSpace(apiErr.Error)
		if msg != "" {
			return msg
		}
	}

	return body
}

func WrapError(prefix string, status string, r io.Reader) error {
	msg := ReadErrorMessage(r, status, MaxErrorBodyBytes)
	return fmt.Errorf("%s: %s", prefix, msg)
}

func GunzipToBytes(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = MaxGunzipOutputBytes
	}

	br := bufio.NewReader(r)
	if _, err := br.Peek(1); err != nil {
		if err == io.EOF {
			return []byte{}, nil
		}
		return nil, err
	}

	gr, err := gzip.NewReader(br)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gr.Close() }()

	lr := &io.LimitedReader{R: gr, N: maxBytes + 1}
	out, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > maxBytes {
		return nil, fmt.Errorf("gunzip: output exceeds %d bytes", maxBytes)
	}
	return out, nil
}

func DrainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func CheckStatus(resp *http.Response, expected int, action string) error {
	if resp.StatusCode == expected {
		return nil
	}
	msg := ReadErrorMessage(resp.Body, resp.Status, MaxErrorBodyBytes)
	return fmt.Errorf("%s failed: status %d: %s", action, resp.StatusCode, msg)
}

func RequireClientAndURL(client *http.Client, base *url.URL) error {
	if client == nil {
		return fmt.Errorf("http client required")
	}
	if base == nil {
		return fmt.Errorf("base url required")
	}
	return nil
}
