// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import "fmt"

// MetadataConfiguration represents the metadata configuration.
// RawJSON stores the complete JSON for the metadata settings.
type MetadataConfiguration struct {
	RawJSON string `json:"-"`
}

// GetMetadataConfiguration retrieves the metadata configuration.
func (c *Client) GetMetadataConfiguration() (*MetadataConfiguration, error) {
	raw, err := c.getRaw("/System/Configuration/metadata")
	if err != nil {
		return nil, fmt.Errorf("getting metadata configuration: %w", err)
	}

	return &MetadataConfiguration{RawJSON: raw}, nil
}

// UpdateMetadataConfiguration updates the metadata configuration.
func (c *Client) UpdateMetadataConfiguration(config *MetadataConfiguration) error {
	if err := c.postRaw("/System/Configuration/metadata", config.RawJSON); err != nil {
		return fmt.Errorf("updating metadata configuration: %w", err)
	}
	return nil
}
