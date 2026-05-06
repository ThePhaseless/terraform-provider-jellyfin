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
