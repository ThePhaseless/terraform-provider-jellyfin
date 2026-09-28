// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
)

// GetMetadataConfiguration returns the metadata configuration document as the
// server serves it.
func (c *Client) GetMetadataConfiguration(ctx context.Context) (string, error) {
	raw, err := c.getRaw(ctx, "/System/Configuration/metadata")
	if err != nil {
		return "", fmt.Errorf("getting metadata configuration: %w", err)
	}
	return raw, nil
}

// UpdateMetadataConfiguration replaces the metadata configuration document with
// raw.
func (c *Client) UpdateMetadataConfiguration(ctx context.Context, raw string) error {
	if err := c.postRaw(ctx, "/System/Configuration/metadata", raw); err != nil {
		return fmt.Errorf("updating metadata configuration: %w", err)
	}
	return nil
}
