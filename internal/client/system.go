// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// SystemInfo represents the full system information from /System/Info.
type SystemInfo struct {
	ID                string `json:"Id"`
	ServerName        string `json:"ServerName"`
	Version           string `json:"Version"`
	OperatingSystem   string `json:"OperatingSystem"`
	HasPendingRestart bool   `json:"HasPendingRestart"`
	LocalAddress      string `json:"LocalAddress"`
}

// PublicSystemInfo represents public system information from /System/Info/Public.
type PublicSystemInfo struct {
	Version                string `json:"Version"`
	StartupWizardCompleted bool   `json:"StartupWizardCompleted"`
}

// SystemConfiguration represents the server configuration.
// RawJSON stores the complete JSON to preserve all fields during round-trips.
type SystemConfiguration struct {
	RawJSON string `json:"-"`
}

// GetSystemInfo retrieves the full system information.
func (c *Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	var info SystemInfo
	if err := c.get(ctx, "/System/Info", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&info)
	}); err != nil {
		return nil, fmt.Errorf("getting system info: %w", err)
	}
	return &info, nil
}

// GetPublicSystemInfo retrieves public system information (no auth required).
func (c *Client) GetPublicSystemInfo(ctx context.Context) (*PublicSystemInfo, error) {
	var info PublicSystemInfo
	if err := c.get(ctx, "/System/Info/Public", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&info)
	}); err != nil {
		return nil, fmt.Errorf("getting public system info: %w", err)
	}
	return &info, nil
}

// GetSystemConfiguration retrieves the server configuration.
func (c *Client) GetSystemConfiguration(ctx context.Context) (*SystemConfiguration, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration")
	if err != nil {
		return nil, fmt.Errorf("getting system configuration: %w", err)
	}

	return &SystemConfiguration{RawJSON: raw}, nil
}

// UpdateSystemConfiguration updates the server configuration.
func (c *Client) UpdateSystemConfiguration(ctx context.Context, config *SystemConfiguration) error {
	if err := c.postRaw(ctx, "/System/Configuration", config.RawJSON); err != nil {
		return fmt.Errorf("updating system configuration: %w", err)
	}
	return nil
}

// RestartServer asks the Jellyfin server to restart and returns without
// waiting. The server may keep answering for a while, and plugin updates can
// set HasPendingRestart again soon after, so neither shows that the restart
// is over; jellyfin_restart waits for several healthy answers in a row.
func (c *Client) RestartServer(ctx context.Context) error {
	if err := c.post(ctx, "/System/Restart", nil); err != nil {
		return fmt.Errorf("restarting server: %w", err)
	}
	return nil
}

// EncodingOptions represents the encoding configuration.
// RawJSON stores the complete JSON since the configuration is very complex.
type EncodingOptions struct {
	RawJSON string `json:"-"`
}

// GetEncodingOptions retrieves the encoding configuration.
func (c *Client) GetEncodingOptions(ctx context.Context) (*EncodingOptions, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration/encoding")
	if err != nil {
		return nil, fmt.Errorf("getting encoding options: %w", err)
	}

	return &EncodingOptions{RawJSON: raw}, nil
}

// UpdateEncodingOptions updates the encoding configuration.
func (c *Client) UpdateEncodingOptions(ctx context.Context, config *EncodingOptions) error {
	if err := c.postRaw(ctx, "/System/Configuration/encoding", config.RawJSON); err != nil {
		return fmt.Errorf("updating encoding options: %w", err)
	}
	return nil
}

// GetOpenAPISpec retrieves the Jellyfin OpenAPI specification.
func (c *Client) GetOpenAPISpec(ctx context.Context) (string, error) {
	raw, err := c.getRaw(ctx, "/api-docs/openapi.json")
	if err != nil {
		return "", fmt.Errorf("getting OpenAPI spec: %w", err)
	}
	return raw, nil
}
