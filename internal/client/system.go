// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"fmt"
)

// SystemInfo represents the full system information from /System/Info.
type SystemInfo struct {
	Id                string `json:"Id"`
	ServerName        string `json:"ServerName"`
	Version           string `json:"Version"`
	OperatingSystem   string `json:"OperatingSystem"`
	HasPendingRestart bool   `json:"HasPendingRestart"`
	LocalAddress      string `json:"LocalAddress"`
}

// PublicSystemInfo represents public system information from /System/Info/Public.
type PublicSystemInfo struct {
	Id                     string `json:"Id"`
	ServerName             string `json:"ServerName"`
	Version                string `json:"Version"`
	LocalAddress           string `json:"LocalAddress"`
	StartupWizardCompleted bool   `json:"StartupWizardCompleted"`
}

// SystemConfiguration represents the server configuration.
// RawJSON stores the complete JSON to preserve all fields during round-trips.
type SystemConfiguration struct {
	ServerName               string `json:"-"`
	IsStartupWizardCompleted bool   `json:"-"`
	RawJSON                  string `json:"-"`
}

// GetSystemInfo retrieves the full system information.
func (c *Client) GetSystemInfo() (*SystemInfo, error) {
	var info SystemInfo
	if err := c.get("/System/Info", &info); err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}
	return &info, nil
}

// GetPublicSystemInfo retrieves public system information (no auth required).
func (c *Client) GetPublicSystemInfo() (*PublicSystemInfo, error) {
	var info PublicSystemInfo
	if err := c.get("/System/Info/Public", &info); err != nil {
		return nil, fmt.Errorf("getting public system info: %w", err)
	}
	return &info, nil
}

// GetSystemConfiguration retrieves the server configuration.
func (c *Client) GetSystemConfiguration() (*SystemConfiguration, error) {
	raw, err := c.getRaw("/System/Configuration")
	if err != nil {
		return nil, fmt.Errorf("getting system configuration: %w", err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parsing system configuration: %w", err)
	}

	config := &SystemConfiguration{
		RawJSON: raw,
	}

	if v, ok := parsed["ServerName"]; ok {
		if err := json.Unmarshal(v, &config.ServerName); err != nil {
			return nil, fmt.Errorf("parsing ServerName from system configuration: %w", err)
		}
	}
	if v, ok := parsed["IsStartupWizardCompleted"]; ok {
		if err := json.Unmarshal(v, &config.IsStartupWizardCompleted); err != nil {
			return nil, fmt.Errorf("parsing IsStartupWizardCompleted from system configuration: %w", err)
		}
	}

	return config, nil
}

// UpdateSystemConfiguration updates the server configuration.
func (c *Client) UpdateSystemConfiguration(config *SystemConfiguration) error {
	if err := c.postRaw("/System/Configuration", config.RawJSON); err != nil {
		return fmt.Errorf("updating system configuration: %w", err)
	}
	return nil
}

// EncodingOptions represents the encoding configuration.
// RawJSON stores the complete JSON since the configuration is very complex.
type EncodingOptions struct {
	RawJSON string `json:"-"`
}

// GetEncodingOptions retrieves the encoding configuration.
func (c *Client) GetEncodingOptions() (*EncodingOptions, error) {
	raw, err := c.getRaw("/System/Configuration/encoding")
	if err != nil {
		return nil, fmt.Errorf("getting encoding options: %w", err)
	}

	return &EncodingOptions{RawJSON: raw}, nil
}

// UpdateEncodingOptions updates the encoding configuration.
func (c *Client) UpdateEncodingOptions(config *EncodingOptions) error {
	if err := c.postRaw("/System/Configuration/encoding", config.RawJSON); err != nil {
		return fmt.Errorf("updating encoding options: %w", err)
	}
	return nil
}
