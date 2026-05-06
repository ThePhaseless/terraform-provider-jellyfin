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
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// doRequest executes an HTTP request with authentication and returns the response.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
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

// get performs an authenticated GET request and decodes the JSON response into target.
func (c *Client) get(ctx context.Context, path string, decode func(io.Reader) error) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Method: http.MethodGet, Path: path, StatusCode: resp.StatusCode, Body: readResponseBody(resp.Body)}
	}

	if err := decode(resp.Body); err != nil {
		return fmt.Errorf("decoding response from GET %s: %w", path, err)
	}

	return nil
}

// getRaw performs an authenticated GET request and returns the raw response body as a string.
func (c *Client) getRaw(ctx context.Context, path string) (string, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &HTTPError{Method: http.MethodGet, Path: path, StatusCode: resp.StatusCode, Body: readResponseBody(resp.Body)}
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body from GET %s: %w", path, err)
	}

	return string(bodyBytes), nil
}

// post performs an authenticated POST request with an optional JSON body.
func (c *Client) post(ctx context.Context, path string, body []byte) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, path, reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Method: http.MethodPost, Path: path, StatusCode: resp.StatusCode, Body: readResponseBody(resp.Body)}
	}

	return nil
}

// postRaw performs an authenticated POST request with a raw JSON string body.
func (c *Client) postRaw(ctx context.Context, path string, rawJSON string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, path, strings.NewReader(rawJSON))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Method: http.MethodPost, Path: path, StatusCode: resp.StatusCode, Body: readResponseBody(resp.Body)}
	}

	return nil
}

// postAndDecode performs an authenticated POST request with a JSON body and decodes the response.
func (c *Client) postAndDecode(ctx context.Context, path string, body []byte, decode func(io.Reader) error) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, path, reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Method: http.MethodPost, Path: path, StatusCode: resp.StatusCode, Body: readResponseBody(resp.Body)}
	}

	if err := decode(resp.Body); err != nil {
		return fmt.Errorf("decoding response from POST %s: %w", path, err)
	}

	return nil
}

// delete performs an authenticated DELETE request.
func (c *Client) delete(ctx context.Context, path string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Method: http.MethodDelete, Path: path, StatusCode: resp.StatusCode, Body: readResponseBody(resp.Body)}
	}

	return nil
}

func decodeJSON(reader io.Reader, target func(*json.Decoder) error) error {
	return target(json.NewDecoder(reader))
}

func readResponseBody(body io.Reader) string {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return fmt.Sprintf("failed to read response body: %v", err)
	}
	return string(bodyBytes)
}
