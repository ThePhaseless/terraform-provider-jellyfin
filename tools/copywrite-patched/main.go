// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/hashicorp/copywrite/cmd"
	"github.com/hashicorp/copywrite/internal/logging"
)

func main() {
	appLogger := logging.New(&logging.LoggerOptions{
		Name:  "hc-copywrite",
		Level: logging.LevelFromString("DEBUG"),
		Color: logging.AutoColor,
	})
	logging.SetDefault(appLogger)
	cmd.Execute()
}
