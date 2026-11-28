package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/guard"
	"github.com/McReaper/rumblestrip-go/internal/spec"
	"github.com/spf13/cobra"
)

func specCommand(out io.Writer) *cobra.Command {
	var o specOptions
	cmd := leaf("spec", "Prove the spec: each scenario passes alone, fails when a value a Then step expects is altered, and runs every line the change adds to a slice or the domain", func(cmd *cobra.Command) error { return runSpec(cmd.Context(), out, o) })
	cmd.Flags().BoolVar(&o.asJSON, "json", false, "print the findings as JSON")
	cmd.Flags().StringVar(&o.base, "base", "origin/main", "the branch the change goes into")
	return cmd
}

type specOptions struct {
	base   string
	asJSON bool
}

func runSpec(ctx context.Context, out io.Writer, o specOptions) error {
	dir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("spec: %w", err)
	}
	d, err := guard.Read(ctx, dir, o.base)
	if err != nil {
		return fmt.Errorf("spec: %w", err)
	}
	fs, err := spec.Prove(ctx, dir, d.AddedLines(spec.Reachable))
	if err != nil {
		return fmt.Errorf("spec: %w", err)
	}
	if o.asJSON {
		err = writeSpecJSON(out, fs)
	} else {
		_, err = io.WriteString(out, specReport(fs))
	}
	if err != nil {
		return fmt.Errorf("print the spec's findings: %w", err)
	}
	if len(fs) > 0 {
		return errFindings
	}
	return nil
}

func specReport(fs []spec.Finding) string {
	var b strings.Builder
	for _, l := range spec.Lines(fs) {
		b.WriteString(l + "\n")
	}
	fmt.Fprintf(&b, "rumblestrip spec: %d findings\n", len(fs))
	return b.String()
}

func writeSpecJSON(out io.Writer, fs []spec.Finding) error {
	type row struct {
		Rule    string `json:"rule"`
		File    string `json:"file"`
		Line    int    `json:"line"`
		Message string `json:"message"`
	}
	rows := make([]row, 0, len(fs))
	for _, f := range fs {
		rows = append(rows, row(f))
	}
	return json.NewEncoder(out).Encode(rows)
}
