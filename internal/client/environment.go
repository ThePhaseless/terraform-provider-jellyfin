// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// DirectoryExists reports whether path is a directory on the Jellyfin server.
func (c *Client) DirectoryExists(ctx context.Context, path string) (bool, error) {
	body, err := json.Marshal(struct {
		Path   string `json:"Path"`
		IsFile bool   `json:"IsFile"`
	}{Path: path})
	if err != nil {
		return false, fmt.Errorf("marshaling path validation request for %s: %w", path, err)
	}

	if err := c.post(ctx, "/Environment/ValidatePath", body); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("validating path %s: %w", path, err)
	}
	return true, nil
}
