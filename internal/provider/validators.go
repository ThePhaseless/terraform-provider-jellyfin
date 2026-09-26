// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

var noSurroundingWhitespaceValidator = stringvalidator.RegexMatches(
	regexp.MustCompile(`^\S(?:.*\S)?$`),
	"must not start or end with whitespace",
)

func libraryNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtLeast(1),
		noSurroundingWhitespaceValidator,
		libraryNameCharactersValidator,
	}
}
