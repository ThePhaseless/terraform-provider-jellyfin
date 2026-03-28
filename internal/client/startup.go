// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import "fmt"

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
func (c *Client) CompleteStartupWizard() error {
	if err := c.post("/Startup/Complete", nil); err != nil {
		return fmt.Errorf("completing startup wizard: %w", err)
	}
	return nil
}

// GetStartupConfiguration retrieves the startup configuration.
func (c *Client) GetStartupConfiguration() (*StartupConfiguration, error) {
	var config StartupConfiguration
	if err := c.get("/Startup/Configuration", &config); err != nil {
		return nil, fmt.Errorf("getting startup configuration: %w", err)
	}
	return &config, nil
}

// UpdateStartupConfiguration updates the startup configuration.
func (c *Client) UpdateStartupConfiguration(config *StartupConfiguration) error {
	if err := c.post("/Startup/Configuration", config); err != nil {
		return fmt.Errorf("updating startup configuration: %w", err)
	}
	return nil
}

// SetStartupUser sets the initial admin user during setup.
func (c *Client) SetStartupUser(name, password string) error {
	body := StartupUser{
		Name:     name,
		Password: password,
	}
	if err := c.post("/Startup/User", &body); err != nil {
		return fmt.Errorf("setting startup user: %w", err)
	}
	return nil
}

// GetFirstUser retrieves the first user during initial setup.
func (c *Client) GetFirstUser() (*StartupUser, error) {
	var user StartupUser
	if err := c.get("/Startup/User", &user); err != nil {
		return nil, fmt.Errorf("getting first user: %w", err)
	}
	return &user, nil
}
