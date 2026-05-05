// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pretty

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type Code int

const (
	Bold     Code = 1
	FgGreen  Code = 32
	FgYellow Code = 33
	FgCyan   Code = 36
	BgBlack  Code = 40
)

var colorsEnabled atomic.Bool

func init() {
	colorsEnabled.Store(true)
}

func EnableColors() {
	colorsEnabled.Store(true)
}

func DisableColors() {
	colorsEnabled.Store(false)
}

type Style struct {
	codes []Code
}

func Color(code Code) Style {
	return Style{codes: []Code{code}}
}

func (c Code) Sprint(args ...any) string {
	return Apply(fmt.Sprint(args...), c)
}

func (c Code) Sprintf(format string, args ...any) string {
	return Apply(fmt.Sprintf(format, args...), c)
}

func (s Style) Sprint(args ...any) string {
	return Apply(fmt.Sprint(args...), s.codes...)
}

func (s Style) Sprintf(format string, args ...any) string {
	return Apply(fmt.Sprintf(format, args...), s.codes...)
}

func Apply(text string, codes ...Code) string {
	if !colorsEnabled.Load() || len(codes) == 0 {
		return text
	}

	parts := make([]string, 0, len(codes))
	for _, code := range codes {
		parts = append(parts, fmt.Sprint(int(code)))
	}

	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", strings.Join(parts, ";"), text)
}
