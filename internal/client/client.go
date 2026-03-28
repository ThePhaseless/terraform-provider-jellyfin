// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is an HTTP client for the Jellyfin API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a new Jellyfin API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
	}
}

// doRequest executes an HTTP request with authentication and returns the response.
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := c.BaseURL + path

	req, err := http.NewRequest(method, url, body)
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
func (c *Client) get(path string, target interface{}) error {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s returned status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decoding response from GET %s: %w", path, err)
	}

	return nil
}

// getRaw performs an authenticated GET request and returns the raw response body as a string.
func (c *Client) getRaw(path string) (string, error) {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GET %s returned status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body from GET %s: %w", path, err)
	}

	return string(bodyBytes), nil
}

// post performs an authenticated POST request with a JSON body.
func (c *Client) post(path string, body interface{}) error {
	var reader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body for POST %s: %w", path, err)
		}
		reader = bytes.NewReader(jsonBody)
	}

	resp, err := c.doRequest(http.MethodPost, path, reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s returned status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// postRaw performs an authenticated POST request with a raw JSON string body.
func (c *Client) postRaw(path string, rawJSON string) error {
	resp, err := c.doRequest(http.MethodPost, path, strings.NewReader(rawJSON))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s returned status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// postAndDecode performs an authenticated POST request with a JSON body and decodes the response.
func (c *Client) postAndDecode(path string, body interface{}, target interface{}) error {
	var reader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body for POST %s: %w", path, err)
		}
		reader = bytes.NewReader(jsonBody)
	}

	resp, err := c.doRequest(http.MethodPost, path, reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s returned status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decoding response from POST %s: %w", path, err)
	}

	return nil
}

// delete performs an authenticated DELETE request.
func (c *Client) delete(path string) error {
	resp, err := c.doRequest(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s returned status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	return nil
}
