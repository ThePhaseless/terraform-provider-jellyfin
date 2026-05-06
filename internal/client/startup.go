// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// StartupConfiguration represents the initial setup configuration.
type StartupConfiguration struct {
	UICulture                 string `json:"UICulture"`
	MetadataCountryCode       string `json:"MetadataCountryCode"`
	PreferredMetadataLanguage string `json:"PreferredMetadataLanguage"`
}

// StartupUser represents a user during initial setup.
type StartupUser struct {
	Name     string `json:"Name"`
	Password string `json:"Password"`
}

// CompleteStartupWizard marks the startup wizard as complete.
func (c *Client) CompleteStartupWizard(ctx context.Context) error {
	if err := c.post(ctx, "/Startup/Complete", nil); err != nil {
		return fmt.Errorf("completing startup wizard: %w", err)
	}
	return nil
}

// GetStartupConfiguration retrieves the startup configuration.
func (c *Client) GetStartupConfiguration(ctx context.Context) (*StartupConfiguration, error) {
	var config StartupConfiguration
	if err := c.get(ctx, "/Startup/Configuration", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&config)
	}); err != nil {
		return nil, fmt.Errorf("getting startup configuration: %w", err)
	}
	return &config, nil
}

// UpdateStartupConfiguration updates the startup configuration.
func (c *Client) UpdateStartupConfiguration(ctx context.Context, config *StartupConfiguration) error {
	jsonBody, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling startup configuration: %w", err)
	}
	if err := c.post(ctx, "/Startup/Configuration", jsonBody); err != nil {
		return fmt.Errorf("updating startup configuration: %w", err)
	}
	return nil
}

// SetStartupUser sets the initial admin user during setup.
func (c *Client) SetStartupUser(ctx context.Context, name, password string) error {
	body := StartupUser{
		Name:     name,
		Password: password,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling startup user: %w", err)
	}
	if err := c.post(ctx, "/Startup/User", jsonBody); err != nil {
		return fmt.Errorf("setting startup user: %w", err)
	}
	return nil
}

// GetFirstUser retrieves the first user during initial setup.
func (c *Client) GetFirstUser(ctx context.Context) (*StartupUser, error) {
	var user StartupUser
	if err := c.get(ctx, "/Startup/User", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&user)
	}); err != nil {
		return nil, fmt.Errorf("getting first user: %w", err)
	}
	return &user, nil
}
