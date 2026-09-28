// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The optional attributes below are also computed, so one the configuration
// leaves unset reads the server's value, and each keeps its prior value while
// unknown.

func optionalString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// optionalEnum is an optional string that takes one of values, which its
// description lists.
func optionalEnum(desc string, values ...string) schema.StringAttribute {
	a := optionalString(desc + " One of `" + strings.Join(values, "`, `") + "`.")
	a.Validators = []validator.String{stringvalidator.OneOf(values...)}
	return a
}

func optionalBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
	}
}

func optionalInt(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
	}
}

func optionalFloat(desc string) schema.Float64Attribute {
	return schema.Float64Attribute{
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
	}
}

func optionalStringList(desc string) schema.ListAttribute {
	return schema.ListAttribute{
		ElementType:         types.StringType,
		Description:         desc,
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
	}
}

func optionalIntList(desc string) schema.ListAttribute {
	a := optionalStringList(desc)
	a.ElementType = types.Int64Type
	return a
}

// The element attributes below belong to the objects of a list, and take no
// UseStateForUnknown: it pairs the elements by index, so an element inserted
// or removed would take another's values. Their list fills them by key with
// useStateForUnknownByKey instead.

func elementString(desc string) schema.StringAttribute {
	a := optionalString(desc)
	a.PlanModifiers = nil
	return a
}

func elementBool(desc string) schema.BoolAttribute {
	a := optionalBool(desc)
	a.PlanModifiers = nil
	return a
}

func elementInt(desc string) schema.Int64Attribute {
	a := optionalInt(desc)
	a.PlanModifiers = nil
	return a
}

func elementFloat(desc string) schema.Float64Attribute {
	a := optionalFloat(desc)
	a.PlanModifiers = nil
	return a
}

func elementStringList(desc string) schema.ListAttribute {
	a := optionalStringList(desc)
	a.PlanModifiers = nil
	return a
}
