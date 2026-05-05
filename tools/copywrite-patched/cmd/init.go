// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/hashicorp/copywrite/addlicense"
	"github.com/hashicorp/copywrite/config"
	"github.com/hashicorp/copywrite/github"
	"github.com/hashicorp/copywrite/internal/pretty"
	"github.com/mattn/go-isatty"
	"github.com/samber/lo"

	"github.com/spf13/cobra"
)

var (
	force bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generates a .copywrite.hcl config for a new project",
	Long: `Generates a .copywrite.hcl config for a new project with helpful comments.

License type and copyright year are inferred from GitHub, and prompts are made
for any unknown values. If you are running this command in CI, please use the
--year and --spdx flags, as prompts are disabled when no TTY is present.`,
	GroupID: "common", // Let's put this command in the common section of the help
	PreRun: func(cmd *cobra.Command, args []string) {
		// Validate we aren't going to write over an existing config
		_, err := os.Stat(".copywrite.hcl")
		if !errors.Is(err, os.ErrNotExist) && !force {
			cobra.CheckErr(fmt.Errorf(".copywrite.hcl config already exists. If you wish to override it, use the `--force` flag"))
		}

		// Input Validation
		spdx, err := cmd.Flags().GetString("spdx")
		cobra.CheckErr(err)
		// SPDX flag must either be an empty string _or_ a valid SPDX list option
		if spdx != "" && !addlicense.ValidSPDX(spdx) {
			err := fmt.Errorf("invalid SPDX license identifier: %s", spdx)
			cobra.CheckErr(err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// We create a new config object here to ensure any existing
		// .copywrite.hcl does not influence the new configuration file
		newConfig, err := config.New()
		cobra.CheckErr(err)

		// Map command flags to config keys
		mapping := map[string]string{
			`spdx`: `project.license`,
			`year`: `project.copyright_year`,
		}

		// update the running config with any command-line flags
		clobberWithDefaults := false
		err = newConfig.LoadCommandFlags(cmd.Flags(), mapping, clobberWithDefaults)
		cobra.CheckErr(err)

		// Try to autodiscover license and year
		if repo, err := github.DiscoverRepo(); err == nil {
			client := github.NewGHClient().Raw()
			data, _, err := client.Repositories.Get(context.Background(), repo.Owner, repo.Name)
			if err == nil {
				cobra.CheckErr(err)
				// fall back to GitHub repo creation year if --year wasn't set
				if !cmd.Flags().Changed("year") {
					newConfig.Project.CopyrightYear = data.CreatedAt.Year()
				}

				// fall back to GitHub's reported SPDX identifier if --spdx wasn't set
				if !cmd.Flags().Changed("spdx") {
					license := data.GetLicense()
					newConfig.Project.License = license.GetSPDXID()
				}
			}
		}

		// Let's prompt the user to validate the current values
		if cmd.OutOrStdout() == os.Stdout && isatty.IsTerminal(os.Stdout.Fd()) {
			err = promptForConfigValues(newConfig)
			cobra.CheckErr(err)
		} else {
			cmd.Println("No TTY detected: if running in CI, use `--year` and `--spdx` flags to set values as needed")
		}

		// Render it out!
		f, err := os.Create(".copywrite.hcl")
		cobra.CheckErr(err)
		defer f.Close()

		err = configToHCL(*newConfig, f)
		cobra.CheckErr(err)

		successText := pretty.Color(pretty.FgGreen).Sprintf("✔️ A config has been successfully generated at: ./%s", f.Name())
		cmd.Println(successText)
		cmd.Println("Please commit this file to your repo")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite an existing .copywrite.hcl file, if one exists")

	// These flags will get mapped to keys in the the global Config
	initCmd.Flags().IntP("year", "y", 0, "Year that the copyright statement should include")
	initCmd.Flags().StringP("spdx", "s", "", "SPDX License Identifier indicating what the project should be licensed under")
}

// configToHCL takes in a Config object and writes an example HCL configuration,
// filling in the `project.license` and `project.copyright_year` keys, along
// with helpful comments. Any io.Writer interface is accepted, be it stdout
// or a file writer.
//
// Config keys other than license and copyright year are currently unsupported.
func configToHCL(c config.Config, wr io.Writer) error {
	tmpl, err := template.New(".copywrite.hcl").Parse(`schema_version = {{.SchemaVersion}}

project {
  license        = "{{.Project.License}}"
  copyright_year = {{.Project.CopyrightYear}}

  # (OPTIONAL) A list of globs that should not have copyright/license headers.
  # Supports doublestar glob patterns for more flexibility in defining which
  # files or folders should be ignored
  header_ignore = [
    # "vendor/**",
    # "**autogen**",
  ]
}
`)
	if err != nil {
		return err
	}

	err = tmpl.Execute(wr, c)
	if err != nil {
		return err
	}

	return nil
}

// promptForConfigValues takes in a pointer to a Config object and prompts the
// user to select or confirm selections for project license type (SPDX ID) and
// copyright year, which then get written back to the config object.
func promptForConfigValues(c *config.Config) error {
	noLicenseText := "" // Copywrite uses an empty string to represent no license

	currentLicense := strings.ToUpper(c.Project.License)
	licenseOptions := lo.Uniq([]string{noLicenseText, currentLicense, "MPL-2.0", "MIT", "Apache-2.0"})
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Choose a license:")
	defaultIndex := 1
	for i, option := range licenseOptions {
		label := option
		if label == noLicenseText {
			label = "(none)"
		}
		switch option {
		case noLicenseText:
			label += " - Proceed without a license"
		case c.Project.License:
			label += " - Current Repo License"
		case "MPL-2.0":
			label += " - HashiCorp default for public repos"
		}
		if option == currentLicense {
			defaultIndex = i + 1
		}
		fmt.Printf("  %d) %s\n", i+1, label)
	}

	licenseChoice, err := promptWithDefault(reader, "Select license number", strconv.Itoa(defaultIndex))
	if err != nil {
		return err
	}
	selectedIndex, err := strconv.Atoi(licenseChoice)
	if err != nil || selectedIndex < 1 || selectedIndex > len(licenseOptions) {
		return fmt.Errorf("license selection must be a number between 1 and %d", len(licenseOptions))
	}

	yearInput, err := promptWithDefault(reader, "Choose a copyright year", strconv.Itoa(c.Project.CopyrightYear))
	if err != nil {
		return err
	}
	year, err := strconv.Atoi(yearInput)
	if err != nil {
		return fmt.Errorf("year must be a number")
	}

	minYear := 1970
	maxYear := time.Now().Year() + 1
	if year < minYear || year > maxYear {
		return fmt.Errorf("copyright year is expected to be between %v and %v", minYear, maxYear)
	}

	c.Project.License = licenseOptions[selectedIndex-1]
	c.Project.CopyrightYear = year
	return nil
}

func promptWithDefault(reader *bufio.Reader, label, defaultValue string) (string, error) {
	fmt.Printf("%s [%s]: ", label, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}
