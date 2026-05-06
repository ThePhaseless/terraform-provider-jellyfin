// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNoPathSeparatorsValidator(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value       string
		expectError bool
	}{
		"plain identifier": {value: "8c95c4d2e50c4fb0a4f36c06ff0f9a1a"},
		"forward slash":    {value: "plugin/id", expectError: true},
		"backslash":        {value: `plugin\id`, expectError: true},
		"empty":            {value: "", expectError: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resp := validator.StringResponse{}
			noPathSeparatorsValidator.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("id"),
				ConfigValue: types.StringValue(test.value),
			}, &resp)

			if resp.Diagnostics.HasError() != test.expectError {
				t.Fatalf("expected error %t, got diagnostics: %v", test.expectError, resp.Diagnostics)
			}
		})
	}
}
