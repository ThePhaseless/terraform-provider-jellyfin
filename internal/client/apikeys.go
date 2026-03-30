// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"fmt"
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
func (c *Client) GetAPIKeys() ([]APIKey, error) {
	var keyList APIKeyList
	if err := c.get("/Auth/Keys", &keyList); err != nil {
		return nil, fmt.Errorf("getting API keys: %w", err)
	}
	return keyList.Items, nil
}

// CreateAPIKey creates a new API key with the given app name.
func (c *Client) CreateAPIKey(appName string) error {
	if err := c.post(fmt.Sprintf("/Auth/Keys?app=%s", url.QueryEscape(appName)), nil); err != nil {
		return fmt.Errorf("creating API key for %s: %w", appName, err)
	}
	return nil
}

// DeleteAPIKey deletes an API key by its access token.
func (c *Client) DeleteAPIKey(accessToken string) error {
	if err := c.delete(fmt.Sprintf("/Auth/Keys/%s", url.PathEscape(accessToken))); err != nil {
		return fmt.Errorf("deleting API key: %w", err)
	}
	return nil
}

// GetAPIKeyByAppName finds an API key by app name.
func (c *Client) GetAPIKeyByAppName(appName string) (*APIKey, error) {
	keys, err := c.GetAPIKeys()
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		if key.AppName == appName {
			return &key, nil
		}
	}

	return nil, fmt.Errorf("API key with app name %q not found", appName)
}
