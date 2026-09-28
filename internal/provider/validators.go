// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var noPathSeparatorsValidator = stringvalidator.RegexMatches(
	regexp.MustCompile(`^[^/\\]+$`),
	"must not contain path separators",
)

func requiredIdentifierValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtLeast(1),
		noPathSeparatorsValidator,
	}
}

// Jellyfin keeps a library as a directory named after it and replaces these
// characters with spaces, so a name containing one would come back as a
// different library.
var libraryNameCharactersValidator = stringvalidator.RegexMatches(
	regexp.MustCompile(`^[^"<>|:*?\\/\x00-\x1f]*$`),
	`must not contain " < > | : * ? \ / or control characters`,
)

// Jellyfin checks names against .NET's `^\S(?:.*\S)?$`, and .NET's \s also
// matches \v, U+0085 and the Unicode separators, which RE2's \s does not.
var noSurroundingWhitespaceValidator = stringvalidator.RegexMatches(
	regexp.MustCompile(`^[^\s\v\x{85}\p{Z}](?:.*[^\s\v\x{85}\p{Z}])?$`),
	"must not start or end with whitespace",
)

func libraryNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtLeast(1),
		noSurroundingWhitespaceValidator,
		libraryNameCharactersValidator,
	}
}

// unsetValidator rejects at plan time a value for an attribute that no
// Jellyfin setting is behind. The server ignores the key and reads it back as
// null, which Terraform reports only as an inconsistent result after apply.
type unsetValidator struct {
	// summary heads the error, and reason says why the value cannot be set.
	summary, reason string
}

// unsupportedLibraryOptionValidator rejects the library options Jellyfin
// does not have.
var unsupportedLibraryOptionValidator = unsetValidator{
	summary: "Unsupported library option",
	reason:  "Jellyfin has no such library option, so the server would ignore this value",
}

func (v unsetValidator) Description(context.Context) string {
	return "must not be set: " + v.reason
}

func (v unsetValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v unsetValidator) check(p path.Path, value attr.Value, diags *diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	diags.AddAttributeError(p, v.summary, v.reason+". Remove it from the configuration.")
}

func (v unsetValidator) ValidateBool(_ context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	v.check(req.Path, req.ConfigValue, &resp.Diagnostics)
}

func (v unsetValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	v.check(req.Path, req.ConfigValue, &resp.Diagnostics)
}

func (v unsetValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	v.check(req.Path, req.ConfigValue, &resp.Diagnostics)
}

func (v unsetValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	v.check(req.Path, req.ConfigValue, &resp.Diagnostics)
}
