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

func TestLibraryNameValidators(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value       string
		expectError bool
	}{
		"plain name":          {value: "Movies"},
		"inner spaces":        {value: "Kids Movies 4K"},
		"punctuation kept":    {value: "Rock & Roll's #1 (Live).mp"},
		"empty":               {value: "", expectError: true},
		"leading space":       {value: " Movies", expectError: true},
		"trailing space":      {value: "Movies ", expectError: true},
		"colon":               {value: "Kids: Movies", expectError: true},
		"forward slash":       {value: "Movies/4K", expectError: true},
		"backslash":           {value: `Movies\4K`, expectError: true},
		"question mark":       {value: "Movies?", expectError: true},
		"asterisk":            {value: "Movies*", expectError: true},
		"double quote":        {value: `"Movies"`, expectError: true},
		"angle brackets":      {value: "<Movies>", expectError: true},
		"pipe":                {value: "Movies|4K", expectError: true},
		"tab":                 {value: "Movies\t4K", expectError: true},
		"other control chars": {value: "Movies\x014K", expectError: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resp := validator.StringResponse{}
			for _, v := range libraryNameValidators() {
				v.ValidateString(context.Background(), validator.StringRequest{
					Path:        path.Root("name"),
					ConfigValue: types.StringValue(test.value),
				}, &resp)
			}

			if resp.Diagnostics.HasError() != test.expectError {
				t.Fatalf("expected error %t, got diagnostics: %v", test.expectError, resp.Diagnostics)
			}
		})
	}
}
