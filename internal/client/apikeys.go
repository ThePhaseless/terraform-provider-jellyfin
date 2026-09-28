// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/url"
)

// APIKey represents a Jellyfin API key.
type APIKey struct {
	AccessToken string `json:"AccessToken"`
	AppName     string `json:"AppName"`
}

// GetAPIKeys retrieves all API keys.
func (c *Client) GetAPIKeys(ctx context.Context) ([]APIKey, error) {
	var keyList struct {
		Items []APIKey `json:"Items"`
	}
	if err := c.getJSON(ctx, "/Auth/Keys", &keyList); err != nil {
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
