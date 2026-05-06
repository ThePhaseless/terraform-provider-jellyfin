// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

// APIKey represents a Jellyfin API key.
type APIKey struct {
	AccessToken string `json:"AccessToken"`
	AppName     string `json:"AppName"`
	DateCreated string `json:"DateCreated"`
}

// APIKeyList represents the response from listing API keys.
type APIKeyList struct {
	Items []APIKey `json:"Items"`
}

// GetAPIKeys retrieves all API keys.
func (c *Client) GetAPIKeys(ctx context.Context) ([]APIKey, error) {
	var keyList APIKeyList
	if err := c.get(ctx, "/Auth/Keys", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&keyList)
	}); err != nil {
		return nil, fmt.Errorf("getting API keys: %w", err)
	}
	return keyList.Items, nil
}

// CreateAPIKey creates a new API key with the given app name.
func (c *Client) CreateAPIKey(ctx context.Context, appName string) error {
	if err := c.post(ctx, fmt.Sprintf("/Auth/Keys?app=%s", url.QueryEscape(appName)), nil); err != nil {
		return fmt.Errorf("creating API key for %s: %w", appName, err)
	}
	return nil
}

// DeleteAPIKey deletes an API key by its access token.
func (c *Client) DeleteAPIKey(ctx context.Context, accessToken string) error {
	if err := c.delete(ctx, fmt.Sprintf("/Auth/Keys/%s", url.PathEscape(accessToken))); err != nil {
		return fmt.Errorf("deleting API key: %w", err)
	}
	return nil
}

// GetAPIKeyByAppName finds an API key by app name. Returns an error if multiple keys share the same name.
func (c *Client) GetAPIKeyByAppName(ctx context.Context, appName string) (*APIKey, error) {
	keys, err := c.GetAPIKeys(ctx)
	if err != nil {
		return nil, err
	}

	var matches []APIKey
	for _, key := range keys {
		if key.AppName == appName {
			matches = append(matches, key)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("API key with app name %q not found", appName)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d API keys with app name %q; use access_token to identify the key", len(matches), appName)
	}
}
