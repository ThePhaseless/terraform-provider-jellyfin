// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create without a policy block and Read.
			{
				Config: testAccUserResourceConfig("testuser1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jellyfin_user.test", "id"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "testuser1"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "is_administrator", "false"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.enable_media_playback", "true"),
					resource.TestCheckNoResourceAttr("jellyfin_user.test", "policy.max_parental_rating"),
				),
			},
			// ImportState.
			{
				ResourceName:            "jellyfin_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			// Update.
			{
				Config: testAccUserResourceConfig("testuser1_updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "testuser1_updated"),
				),
			},
		},
	})
}

// Jellyfin 12 accepts a rename that changes only letter case; 10.11 rejects
// it with "The new and old names must be different".
func TestAccUserResourceRenameLetterCaseOnly(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceConfig("caseuser"),
				Check:  resource.TestCheckResourceAttr("jellyfin_user.test", "name", "caseuser"),
			},
			{
				Config: testAccUserResourceConfig("CaseUser"),
				Check:  resource.TestCheckResourceAttr("jellyfin_user.test", "name", "CaseUser"),
			},
		},
	})
}

func TestAccUserResourceParentalRating(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with a rating and an explicitly null sub-rating.
			{
				Config: testAccUserResourceParentalRatingConfig("10", "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.max_parental_rating", "10"),
					resource.TestCheckNoResourceAttr("jellyfin_user.test", "policy.max_parental_sub_rating"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.enable_media_playback", "true"),
				),
			},
			// Update both.
			{
				Config: testAccUserResourceParentalRatingConfig("13", "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.max_parental_rating", "13"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.max_parental_sub_rating", "1"),
				),
			},
			// Back to null.
			{
				Config: testAccUserResourceParentalRatingConfig("null", "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("jellyfin_user.test", "policy.max_parental_rating"),
					resource.TestCheckNoResourceAttr("jellyfin_user.test", "policy.max_parental_sub_rating"),
				),
			},
			// ImportState.
			{
				ResourceName:            "jellyfin_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

func testAccUserResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "jellyfin_user" "test" {
  name     = %[1]q
  password = "testpass123"
}
`, name)
}

func testAccUserResourceParentalRatingConfig(rating, subRating string) string {
	return fmt.Sprintf(`
resource "jellyfin_user" "test" {
  name     = "ratinguser"
  password = "testpass123"

  policy = {
    max_parental_rating     = %[1]s
    max_parental_sub_rating = %[2]s
  }
}
`, rating, subRating)
}
