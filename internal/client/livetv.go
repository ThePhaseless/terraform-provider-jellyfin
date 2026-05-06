// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
)

// LiveTVConfiguration represents the Live TV configuration.
// RawJSON stores the complete JSON for the Live TV settings.
type LiveTVConfiguration struct {
	RawJSON string `json:"-"`
}

// GetLiveTVConfiguration retrieves the Live TV configuration.
func (c *Client) GetLiveTVConfiguration(ctx context.Context) (*LiveTVConfiguration, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration/livetv")
	if err != nil {
		return nil, fmt.Errorf("getting Live TV configuration: %w", err)
	}

	return &LiveTVConfiguration{RawJSON: raw}, nil
}

// UpdateLiveTVConfiguration updates the Live TV configuration.
func (c *Client) UpdateLiveTVConfiguration(ctx context.Context, config *LiveTVConfiguration) error {
	if err := c.postRaw(ctx, "/System/Configuration/livetv", config.RawJSON); err != nil {
		return fmt.Errorf("updating Live TV configuration: %w", err)
	}
	return nil
}
