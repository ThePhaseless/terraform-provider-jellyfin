// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the Jellyfin API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// HTTPError represents a non-success Jellyfin API response.
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s returned status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// IsNotFound reports whether err wraps a Jellyfin API 404 response.
func IsNotFound(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

// NewClient creates a new Jellyfin API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 30 * time.Second, Transport: newTransport()},
	}
}

const maxIdleConnsPerHost = 10

func newTransport() http.RoundTripper {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	t = t.Clone()
	t.MaxIdleConnsPerHost = maxIdleConnsPerHost
	return t
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating %s request for %s: %w", method, path, err)
	}

	if c.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, c.APIKey))
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing %s request for %s: %w", method, path, err)
	}

	return resp, nil
}

// send fails with an HTTPError on a non-2xx answer, else hands the body to
// read (if non-nil).
func (c *Client) send(ctx context.Context, method, path string, body io.Reader, read func(io.Reader) error) error {
	resp, err := c.doRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatus(method, path, resp); err != nil {
		return err
	}
	if read == nil {
		return nil
	}
	if err := read(resp.Body); err != nil {
		return fmt.Errorf("reading the response to %s %s: %w", method, path, err)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	return c.send(ctx, http.MethodGet, path, nil, decodeInto(target))
}

func (c *Client) getRaw(ctx context.Context, path string) (string, error) {
	var raw []byte
	err := c.send(ctx, http.MethodGet, path, nil, func(r io.Reader) error {
		var err error
		raw, err = io.ReadAll(r)
		return err
	})
	return string(raw), err
}

func (c *Client) post(ctx context.Context, path string, body []byte) error {
	return c.send(ctx, http.MethodPost, path, bodyReader(body), nil)
}

func (c *Client) postRaw(ctx context.Context, path, rawJSON string) error {
	return c.send(ctx, http.MethodPost, path, strings.NewReader(rawJSON), nil)
}

func (c *Client) postJSON(ctx context.Context, path string, body []byte, target any) error {
	return c.send(ctx, http.MethodPost, path, bodyReader(body), decodeInto(target))
}

func (c *Client) delete(ctx context.Context, path string) error {
	return c.send(ctx, http.MethodDelete, path, nil, nil)
}

func bodyReader(body []byte) io.Reader {
	if body == nil {
		return nil
	}
	return bytes.NewReader(body)
}

func decodeInto(target any) func(io.Reader) error {
	return func(r io.Reader) error { return json.NewDecoder(r).Decode(target) }
}

func checkStatus(method, path string, resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		body = fmt.Appendf(nil, "failed to read response body: %v", err)
	}
	return &HTTPError{Method: method, Path: path, StatusCode: resp.StatusCode, Body: string(body)}
}
