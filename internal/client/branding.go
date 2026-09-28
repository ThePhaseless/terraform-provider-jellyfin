// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
)

// GetBrandingConfiguration returns the branding configuration document as the
// server serves it.
func (c *Client) GetBrandingConfiguration(ctx context.Context) (string, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration/branding")
	if err != nil {
		return "", fmt.Errorf("getting branding configuration: %w", err)
	}
	return raw, nil
}

// UpdateBrandingConfiguration replaces the branding configuration document with
// raw.
func (c *Client) UpdateBrandingConfiguration(ctx context.Context, raw string) error {
	if err := c.postRaw(ctx, "/System/Configuration/branding", raw); err != nil {
		return fmt.Errorf("updating branding configuration: %w", err)
	}
	return nil
}
