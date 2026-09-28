// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
)

// GetLiveTVConfiguration returns the Live TV configuration document as the
// server serves it.
func (c *Client) GetLiveTVConfiguration(ctx context.Context) (string, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration/livetv")
	if err != nil {
		return "", fmt.Errorf("getting Live TV configuration: %w", err)
	}
	return raw, nil
}

// UpdateLiveTVConfiguration replaces the Live TV configuration document with
// raw.
func (c *Client) UpdateLiveTVConfiguration(ctx context.Context, raw string) error {
	if err := c.postRaw(ctx, "/System/Configuration/livetv", raw); err != nil {
		return fmt.Errorf("updating Live TV configuration: %w", err)
	}
	return nil
}
