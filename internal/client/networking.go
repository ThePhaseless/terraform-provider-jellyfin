// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
)

// GetNetworkConfiguration returns the network configuration document as the
// server serves it.
func (c *Client) GetNetworkConfiguration(ctx context.Context) (string, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration/network")
	if err != nil {
		return "", fmt.Errorf("getting network configuration: %w", err)
	}
	return raw, nil
}

// UpdateNetworkConfiguration replaces the network configuration document with
// raw.
func (c *Client) UpdateNetworkConfiguration(ctx context.Context, raw string) error {
	if err := c.postRaw(ctx, "/System/Configuration/network", raw); err != nil {
		return fmt.Errorf("updating network configuration: %w", err)
	}
	return nil
}
