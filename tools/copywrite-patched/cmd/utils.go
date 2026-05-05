// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	enccsv "encoding/csv"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/hashicorp/copywrite/internal/pretty"
)

type tableRow []any

type tableWriter struct {
	out    io.Writer
	header tableRow
	rows   []tableRow
}

func newTableWriter(out io.Writer) *tableWriter {
	return &tableWriter{out: out}
}

func (t *tableWriter) SetOutputMirror(out io.Writer) {
	t.out = out
}

func (t *tableWriter) AppendHeader(row tableRow) {
	t.header = append(tableRow{}, row...)
}

func (t *tableWriter) AppendRow(row tableRow) {
	t.rows = append(t.rows, append(tableRow{}, row...))
}

func (t *tableWriter) Render() {
	w := tabwriter.NewWriter(t.out, 0, 0, 2, ' ', 0)
	if len(t.header) > 0 {
		fmt.Fprintln(w, renderTabbedRow(t.header, pretty.FgGreen))
	}
	for _, row := range t.rows {
		fmt.Fprintln(w, renderTabbedRow(row))
	}
	_ = w.Flush()
}

func (t *tableWriter) RenderCSV() {
	w := enccsv.NewWriter(t.out)
	if len(t.header) > 0 {
		_ = w.Write(rowToStrings(t.header))
	}
	for _, row := range t.rows {
		_ = w.Write(rowToStrings(row))
	}
	w.Flush()
}

func stringArrayToRow(m []string) tableRow {
	row := make(tableRow, 0, len(m))
	for _, v := range m {
		row = append(row, v)
	}
	return row
}

func colorize(s string, colors ...pretty.Code) string {
	return pretty.Apply(s, colors...)
}

func renderTabbedRow(row tableRow, colors ...pretty.Code) string {
	values := rowToStrings(row)
	if len(colors) > 0 {
		for i, value := range values {
			values[i] = colorize(value, colors...)
		}
	}
	return strings.Join(values, "\t")
}

func rowToStrings(row tableRow) []string {
	values := make([]string, 0, len(row))
	for _, value := range row {
		values = append(values, fmt.Sprint(value))
	}
	return values
}
