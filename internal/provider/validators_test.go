// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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

func TestUnsupportedLibraryOptionValidatorRejectsOnlySetValues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	v := unsupportedLibraryOptionValidator{}
	p := path.Root("library_options").AtName("disabled_metadata_savers")
	validateBool := func(value types.Bool) diag.Diagnostics {
		resp := validator.BoolResponse{}
		v.ValidateBool(ctx, validator.BoolRequest{Path: p, ConfigValue: value}, &resp)
		return resp.Diagnostics
	}
	validateInt64 := func(value types.Int64) diag.Diagnostics {
		resp := validator.Int64Response{}
		v.ValidateInt64(ctx, validator.Int64Request{Path: p, ConfigValue: value}, &resp)
		return resp.Diagnostics
	}
	validateString := func(value types.String) diag.Diagnostics {
		resp := validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{Path: p, ConfigValue: value}, &resp)
		return resp.Diagnostics
	}
	validateList := func(value types.List) diag.Diagnostics {
		resp := validator.ListResponse{}
		v.ValidateList(ctx, validator.ListRequest{Path: p, ConfigValue: value}, &resp)
		return resp.Diagnostics
	}
	emptyList, _ := types.ListValue(types.StringType, nil)

	tests := map[string]struct {
		diags       diag.Diagnostics
		expectError bool
	}{
		"bool set":       {validateBool(types.BoolValue(false)), true},
		"bool null":      {validateBool(types.BoolNull()), false},
		"bool unknown":   {validateBool(types.BoolUnknown()), false},
		"int64 set":      {validateInt64(types.Int64Value(0)), true},
		"int64 null":     {validateInt64(types.Int64Null()), false},
		"string set":     {validateString(types.StringValue("")), true},
		"string null":    {validateString(types.StringNull()), false},
		"string unknown": {validateString(types.StringUnknown()), false},
		"list set":       {validateList(emptyList), true},
		"list null":      {validateList(types.ListNull(types.StringType)), false},
		"list unknown":   {validateList(types.ListUnknown(types.StringType)), false},
	}
	for name, test := range tests {
		if test.diags.HasError() != test.expectError {
			t.Errorf("%s: expected error %t, got diagnostics: %v", name, test.expectError, test.diags)
		}
	}
}

func TestLibraryNameValidators(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value       string
		expectError bool
	}{
		"plain name":                 {value: "Movies"},
		"inner spaces":               {value: "Kids Movies 4K"},
		"punctuation kept":           {value: "Rock & Roll's #1 (Live).mp"},
		"empty":                      {value: "", expectError: true},
		"leading space":              {value: " Movies", expectError: true},
		"trailing space":             {value: "Movies ", expectError: true},
		"leading nbsp":               {value: "\u00a0Movies", expectError: true},
		"trailing ideographic space": {value: "Movies\u3000", expectError: true},
		"trailing line separator":    {value: "Movies\u2028", expectError: true},
		"trailing next line":         {value: "Movies\u0085", expectError: true},
		"inner nbsp":                 {value: "Kids\u00a0Movies"},
		"colon":                      {value: "Kids: Movies", expectError: true},
		"forward slash":              {value: "Movies/4K", expectError: true},
		"backslash":                  {value: `Movies\4K`, expectError: true},
		"question mark":              {value: "Movies?", expectError: true},
		"asterisk":                   {value: "Movies*", expectError: true},
		"double quote":               {value: `"Movies"`, expectError: true},
		"angle brackets":             {value: "<Movies>", expectError: true},
		"pipe":                       {value: "Movies|4K", expectError: true},
		"tab":                        {value: "Movies\t4K", expectError: true},
		"other control chars":        {value: "Movies\x014K", expectError: true},
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
