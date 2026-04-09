// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import "fmt"

// NetworkConfiguration represents the network configuration.
// RawJSON stores the complete JSON since the configuration has many fields.
type NetworkConfiguration struct {
	RawJSON string `json:"-"`
}

// GetNetworkConfiguration retrieves the network configuration.
func (c *Client) GetNetworkConfiguration() (*NetworkConfiguration, error) {
	raw, err := c.getRaw("/System/Configuration/network")
	if err != nil {
		return nil, fmt.Errorf("getting network configuration: %w", err)
	}

	return &NetworkConfiguration{RawJSON: raw}, nil
}

// UpdateNetworkConfiguration updates the network configuration.
func (c *Client) UpdateNetworkConfiguration(config *NetworkConfiguration) error {
	if err := c.postRaw("/System/Configuration/network", config.RawJSON); err != nil {
		return fmt.Errorf("updating network configuration: %w", err)
	}
	return nil
}
