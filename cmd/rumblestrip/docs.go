package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/McReaper/rumblestrip-go/internal/staledocs"
	"github.com/spf13/cobra"
)

type docsOptions struct {
	base   string
	asJSON bool
}

func docsCommand(out io.Writer) *cobra.Command {
	var o docsOptions
	cmd := leaf("docs", "List the docs this change must reread: never reread, or since a quarter of the code changed, two at most", func(cmd *cobra.Command) error {
		return runDocs(cmd.Context(), out, o)
	})
	cmd.Flags().StringVar(&o.base, "base", "origin/main", "the branch the change goes into")
	cmd.Flags().BoolVar(&o.asJSON, "json", false, "print the findings as JSON")
	cmd.AddCommand(leafOn("reread <doc>...", "Record docs as reread today, once each says what the code does", func(cmd *cobra.Command, args []string) error {
		return markReread(cmd.Context(), out, args)
	}))
	return cmd
}

func runDocs(ctx context.Context, out io.Writer, o docsOptions) error {
	findings, err := staledocs.Check(ctx, ".", o.base)
	if err != nil {
		return fmt.Errorf("docs: %w", err)
	}
	if o.asJSON {
		err = writeDocsJSON(out, findings)
	} else {
		_, err = io.WriteString(out, docsText(findings))
	}
	if err != nil {
		return fmt.Errorf("print the docs to reread: %w", err)
	}
	if len(findings) > 0 {
		return errFindings
	}
	return nil
}

func docsText(findings []staledocs.Finding) string {
	if len(findings) == 0 {
		return "No doc to reread in this change.\n"
	}
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "%s:%d: %s: %s\n", f.File, f.Line, f.Rule, f.Message)
	}
	return b.String()
}

func writeDocsJSON(out io.Writer, findings []staledocs.Finding) error {
	type row struct {
		Rule    string `json:"rule"`
		File    string `json:"file"`
		Line    int    `json:"line"`
		Message string `json:"message"`
	}
	rows := make([]row, 0, len(findings))
	for _, f := range findings {
		rows = append(rows, row{f.Rule, f.File, f.Line, f.Message})
	}
	if err := json.NewEncoder(out).Encode(rows); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// WHY: a doc is named as git lists it, from the repository's root, so
// ./README.md and docs\guide.md name the same lines as README.md and
// docs/guide.md.
func markReread(ctx context.Context, out io.Writer, args []string) error {
	watched, err := staledocs.Watched(ctx, ".")
	if err != nil {
		return fmt.Errorf("docs reread: %w", err)
	}
	docs := make([]string, 0, len(args))
	for _, a := range args {
		d := filepath.ToSlash(filepath.Clean(a))
		if !slices.Contains(watched, d) {
			return fmt.Errorf("docs reread: %s is no doc to reread: a markdown file of the repository, named from its root, that %s doesn't leave out", a, staledocs.File)
		}
		docs = append(docs, d)
	}
	if err := staledocs.Mark(".", docs, time.Now().Format(time.DateOnly)); err != nil {
		return fmt.Errorf("docs reread: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Reread: %s. Commit %s with the change.\n", strings.Join(docs, ", "), staledocs.File); err != nil {
		return fmt.Errorf("docs reread: %w", err)
	}
	return nil
}
