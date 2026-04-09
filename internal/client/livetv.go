// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import "fmt"

// LiveTVConfiguration represents the Live TV configuration.
// RawJSON stores the complete JSON for the Live TV settings.
type LiveTVConfiguration struct {
	RawJSON string `json:"-"`
}

// GetLiveTVConfiguration retrieves the Live TV configuration.
func (c *Client) GetLiveTVConfiguration() (*LiveTVConfiguration, error) {
	raw, err := c.getRaw("/System/Configuration/livetv")
	if err != nil {
		return nil, fmt.Errorf("getting Live TV configuration: %w", err)
	}

	return &LiveTVConfiguration{RawJSON: raw}, nil
}

// UpdateLiveTVConfiguration updates the Live TV configuration.
func (c *Client) UpdateLiveTVConfiguration(config *LiveTVConfiguration) error {
	if err := c.postRaw("/System/Configuration/livetv", config.RawJSON); err != nil {
		return fmt.Errorf("updating Live TV configuration: %w", err)
	}
	return nil
}
