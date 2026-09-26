// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckJellyfinVersionAtLeast(t, "12")
		},
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

func TestAccUserResourceUpdateKeepsUserConfiguration(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceConfig("cfguser"),
				Check: resource.ComposeTestCheckFunc(
					testAccSetUserSubtitleLanguage(t, "fre"),
					testAccCheckUserSubtitleLanguage(t, "fre"),
				),
			},
			{
				Config: testAccUserResourceConfig("cfguser_renamed"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "cfguser_renamed"),
					testAccCheckUserSubtitleLanguage(t, "fre"),
				),
			},
			{
				Config: `
resource "jellyfin_user" "test" {
  name     = "cfguser_renamed"
  password = "testpass123"

  policy = {
    max_active_sessions = 3
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.max_active_sessions", "3"),
					testAccCheckUserSubtitleLanguage(t, "fre"),
				),
			},
		},
	})
}

func TestAccUserResourceParentalRating(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceParentalRatingConfig("10", "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.max_parental_rating", "10"),
					resource.TestCheckNoResourceAttr("jellyfin_user.test", "policy.max_parental_sub_rating"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.enable_media_playback", "true"),
				),
			},
			{
				Config: testAccUserResourceParentalRatingConfig("13", "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.max_parental_rating", "13"),
					resource.TestCheckResourceAttr("jellyfin_user.test", "policy.max_parental_sub_rating", "1"),
				),
			},
			{
				ResourceName:            "jellyfin_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				Config: testAccUserResourceParentalRatingConfig("null", "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("jellyfin_user.test", "policy.max_parental_rating"),
					resource.TestCheckNoResourceAttr("jellyfin_user.test", "policy.max_parental_sub_rating"),
				),
			},
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

func testAccPreCheckJellyfinVersionAtLeast(t *testing.T, minVersion string) {
	t.Helper()

	info, err := testAccClient(t).GetSystemInfo(context.Background())
	if err != nil {
		t.Fatalf("reading Jellyfin version: %v", err)
	}
	if compareDottedVersions(info.Version, minVersion) < 0 {
		t.Skipf("requires Jellyfin %s or newer, server is %s", minVersion, info.Version)
	}
}

// testAccSetUserSubtitleLanguage changes a per-user setting of
// jellyfin_user.test outside Terraform, so a later step can check that the
// provider leaves it alone.
func testAccSetUserSubtitleLanguage(t *testing.T, language string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		c := testAccClient(t)
		id := s.RootModule().Resources["jellyfin_user.test"].Primary.ID

		raw, err := c.GetUserRaw(ctx, id)
		if err != nil {
			return err
		}
		user, err := parseJSONObject(raw)
		if err != nil {
			return err
		}
		configuration, err := parseJSONObject(string(user["Configuration"]))
		if err != nil {
			return err
		}
		configuration["SubtitleLanguagePreference"], _ = json.Marshal(language)
		if user["Configuration"], err = json.Marshal(configuration); err != nil {
			return err
		}
		body, err := json.Marshal(user)
		if err != nil {
			return err
		}
		return c.UpdateUserRaw(ctx, id, string(body))
	}
}

func testAccCheckUserSubtitleLanguage(t *testing.T, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id := s.RootModule().Resources["jellyfin_user.test"].Primary.ID
		raw, err := testAccClient(t).GetUserRaw(context.Background(), id)
		if err != nil {
			return err
		}

		var user struct {
			Configuration struct {
				SubtitleLanguagePreference string `json:"SubtitleLanguagePreference"`
			} `json:"Configuration"`
		}
		if err := json.Unmarshal([]byte(raw), &user); err != nil {
			return fmt.Errorf("parsing user %s: %w", id, err)
		}
		if got := user.Configuration.SubtitleLanguagePreference; got != want {
			return fmt.Errorf("SubtitleLanguagePreference = %q, want %q (user configuration was reset)", got, want)
		}
		return nil
	}
}
