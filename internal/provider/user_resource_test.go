// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ThePhaseless/terraform-provider-jellyfin/internal/client"
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
			{
				ResourceName:            "jellyfin_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				Config: testAccUserResourceConfig("testuser1_updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jellyfin_user.test", "name", "testuser1_updated"),
				),
			},
		},
	})
}

// Jellyfin accepts a rename that changes only letter case.
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

func TestAccUserResourcePasswordChange(t *testing.T) {
	ctx := t.Context()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourcePasswordConfig("firstpass123"),
				Check:  testAccCheckUserSignIn(ctx, "pwuser", "firstpass123", true),
			},
			{
				Config: testAccUserResourcePasswordConfig("secondpass123"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserSignIn(ctx, "pwuser", "secondpass123", true),
					testAccCheckUserSignIn(ctx, "pwuser", "firstpass123", false),
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

func TestAccUserResourceRetryAfterFailedCreate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Without refresh, the retry plans from the state the failed create
		// saved rather than from one Read rebuilt from the server.
		AdditionalCLIOptions: &resource.AdditionalCLIOptions{
			Plan: resource.PlanOptions{NoRefresh: true},
		},
		Steps: []resource.TestStep{
			{
				Config: `
resource "jellyfin_user" "test" {
  name     = "retryuser"
  password = "testpass123"

  policy = {
    sync_play_access = "Bogus"
  }
}
`,
				ExpectError: regexp.MustCompile(`Failed to update user policy`),
			},
			{
				Config: testAccUserResourceConfig("retryuser"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						expectPriorStateAttributesSet{
							address:    "jellyfin_user.test",
							attributes: []string{"name", "policy"},
						},
					},
				},
				Check: resource.TestCheckResourceAttr("jellyfin_user.test", "name", "retryuser"),
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

func testAccUserResourcePasswordConfig(password string) string {
	return fmt.Sprintf(`
resource "jellyfin_user" "test" {
  name     = "pwuser"
  password = %[1]q
}
`, password)
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

type expectPriorStateAttributesSet struct {
	address    string
	attributes []string
}

func (e expectPriorStateAttributesSet) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	if req.Plan.PriorState == nil || req.Plan.PriorState.Values == nil {
		resp.Error = errors.New("plan has no prior state")
		return
	}
	for _, r := range req.Plan.PriorState.Values.RootModule.Resources {
		if r.Address != e.address {
			continue
		}
		for _, a := range e.attributes {
			if r.AttributeValues[a] == nil {
				resp.Error = fmt.Errorf("%s.%s is null in the prior state", e.address, a)
				return
			}
		}
		return
	}
	resp.Error = fmt.Errorf("%s is not in the prior state", e.address)
}

// testAccSetUserSubtitleLanguage changes a per-user setting of
// jellyfin_user.test outside Terraform.
func testAccSetUserSubtitleLanguage(t *testing.T, language string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()
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
		raw, err := testAccClient(t).GetUserRaw(t.Context(), id)
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

func testAccCheckUserSignIn(ctx context.Context, name, password string, wantAccepted bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		_, err := client.NewClient(os.Getenv("JELLYFIN_ENDPOINT"), "").AuthenticateByName(ctx, name, password)
		var httpErr *client.HTTPError
		switch {
		case wantAccepted && err != nil:
			return fmt.Errorf("signing in as %s with password %q: %w", name, password, err)
		case !wantAccepted && err == nil:
			return fmt.Errorf("signing in as %s with password %q succeeded, want it rejected", name, password)
		case !wantAccepted && (!errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized):
			return fmt.Errorf("signing in as %s with password %q: %w, want it rejected with status 401", name, password, err)
		}
		return nil
	}
}
