// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import "fmt"

// BrandingConfiguration represents the branding configuration.
// RawJSON stores the complete JSON for the branding settings.
type BrandingConfiguration struct {
	RawJSON string `json:"-"`
}

// GetBrandingConfiguration retrieves the branding configuration.
func (c *Client) GetBrandingConfiguration() (*BrandingConfiguration, error) {
	raw, err := c.getRaw("/System/Configuration/branding")
	if err != nil {
		return nil, fmt.Errorf("getting branding configuration: %w", err)
	}

	return &BrandingConfiguration{RawJSON: raw}, nil
}

// UpdateBrandingConfiguration updates the branding configuration.
func (c *Client) UpdateBrandingConfiguration(config *BrandingConfiguration) error {
	if err := c.postRaw("/System/Configuration/branding", config.RawJSON); err != nil {
		return fmt.Errorf("updating branding configuration: %w", err)
	}
	return nil
}
