// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
)

// NetworkConfiguration represents the network configuration.
// RawJSON stores the complete JSON since the configuration has many fields.
type NetworkConfiguration struct {
	RawJSON string `json:"-"`
}

// GetNetworkConfiguration retrieves the network configuration.
func (c *Client) GetNetworkConfiguration(ctx context.Context) (*NetworkConfiguration, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration/network")
	if err != nil {
		return nil, fmt.Errorf("getting network configuration: %w", err)
	}

	return &NetworkConfiguration{RawJSON: raw}, nil
}

// UpdateNetworkConfiguration updates the network configuration.
func (c *Client) UpdateNetworkConfiguration(ctx context.Context, config *NetworkConfiguration) error {
	if err := c.postRaw(ctx, "/System/Configuration/network", config.RawJSON); err != nil {
		return fmt.Errorf("updating network configuration: %w", err)
	}
	return nil
}
