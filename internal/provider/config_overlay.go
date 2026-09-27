// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
)

// parseJSONObject parses a raw JSON string into a map of raw JSON messages.
// An empty string or "{}" returns an empty map.
func parseJSONObject(raw string) (map[string]json.RawMessage, error) {
	if raw == "" || raw == "{}" {
		return map[string]json.RawMessage{}, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("failed to parse JSON object: %w", err)
	}

	return obj, nil
}
