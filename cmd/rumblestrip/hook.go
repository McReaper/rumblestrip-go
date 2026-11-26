package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/check"
	"github.com/McReaper/rumblestrip-go/internal/guard"
	"github.com/McReaper/rumblestrip-go/internal/platform/gitleaks"
	"github.com/McReaper/rumblestrip-go/internal/pretool"
	"github.com/McReaper/rumblestrip-go/internal/staledocs"
	"github.com/spf13/cobra"
)

// WHY: the process exits with this code, not 1: Claude Code reads a
// hook's code 2 as "show my stderr to Claude", or for Stop "don't stop".
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit %d", int(c)) }

type hookInput struct {
	Cwd            string `json:"cwd"`
	StopHookActive bool   `json:"stop_hook_active"`
	ToolName       string `json:"tool_name"`
	ToolInput      struct {
		FilePath string `json:"file_path"`
		Command  string `json:"command"`
	} `json:"tool_input"`
}

// WHY: in Rumblestrip's image, plugin/bin/rumblestrip mounts the repository
// at RUMBLESTRIP_ROOT, while Claude Code still gives the hook the host's
// paths, under RUMBLESTRIP_HOST_ROOT.
func readHookInput(in io.Reader) (hookInput, error) {
	var h hookInput
	if err := json.NewDecoder(in).Decode(&h); err != nil {
		return h, fmt.Errorf("read the hook's input: %w", err)
	}
	if host, root := os.Getenv("RUMBLESTRIP_HOST_ROOT"), os.Getenv("RUMBLESTRIP_ROOT"); host != "" && root != "" {
		m := hostPaths{host: host, root: root}
		h.Cwd = m.inImage(h.Cwd)
		h.ToolInput.FilePath = m.inImage(h.ToolInput.FilePath)
	}
	return h, nil
}

type hostPaths struct{ host, root string }

var gitBashDrive = regexp.MustCompile(`^/([a-zA-Z])(/|$)`)

// WHY: Windows spells a path with backslashes, either case of its drive
// letter, or Git Bash's /c/; a path outside the repository stays as given.
func (m hostPaths) inImage(p string) string {
	host := strings.TrimSuffix(strings.ReplaceAll(m.host, `\`, "/"), "/")
	q := strings.ReplaceAll(p, `\`, "/")
	windows := len(host) >= 2 && host[1] == ':'
	if g := gitBashDrive.FindStringSubmatch(q); windows && g != nil {
		q = g[1] + ":/" + q[len(g[0]):]
	}
	if len(q) < len(host) || len(q) > len(host) && q[len(host)] != '/' {
		return p
	}
	if prefix := q[:len(host)]; prefix == host || windows && strings.EqualFold(prefix, host) {
		return m.root + q[len(host):]
	}
	return p
}

func hookCommand() *cobra.Command {
	hook := &cobra.Command{Use: "hook", Short: "Claude Code hooks: a gesture refused before it runs, feedback after each edit, the full check before stopping"}
	var base string
	stop := leaf("stop", "Run rumblestrip check before the agent stops; a failing gate keeps it working, once", func(cmd *cobra.Command) error { return stopHook(cmd, base) })
	stop.Flags().StringVar(&base, "base", "origin/main", "the branch the change goes into")
	hook.AddCommand(&cobra.Command{
		Use:   "session-start",
		Short: "Tell the agent which docs its change must reread; quiet when none",
		Args:  cobra.NoArgs,
		RunE:  sessionStartHook,
	}, &cobra.Command{
		Use:   "post-edit",
		Short: "Lint the package of the Go file just edited; findings go back to the agent",
		Args:  cobra.NoArgs,
		RunE:  postEditHook,
	}, stop, &cobra.Command{
		Use:   "pre-tool",
		Short: "Refuse a command or an edit that reaches past Rumblestrip: merging, pushing to main, a force push, skipping hooks, editing CI or settings",
		Args:  cobra.NoArgs,
		RunE:  preToolHook,
	})
	return hook
}

// WHY: what a SessionStart hook prints goes into the agent's context.
func sessionStartHook(cmd *cobra.Command, _ []string) error {
	h, err := readHookInput(cmd.InOrStdin())
	if err != nil || h.Cwd == "" {
		h.Cwd = "."
	}
	docs := docsOwed(cmd.Context(), h.Cwd)
	if len(docs) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("Rumblestrip: a change to the code here rereads these docs, the oldest first:\n")
	for _, d := range docs {
		b.WriteString("- " + d.Path + " " + d.Since() + "\n")
	}
	b.WriteString("Once your change touches the code, reread each against it, fix what no longer holds, then run rumblestrip docs reread <doc>. The Docs check holds such a change until then; a change with no code owes nothing.\n")
	if _, err := io.WriteString(cmd.OutOrStdout(), b.String()); err != nil {
		return fmt.Errorf("write the docs to reread: %w", err)
	}
	return nil
}

// WHY: a session must start whatever git says, so a failure owes nothing
// here: the Docs gate tells the agent again before it stops. At the start
// nothing has changed yet, so the docs come as what a change to the code
// will owe.
func docsOwed(ctx context.Context, dir string) []staledocs.Doc {
	r, err := staledocs.Status(ctx, dir, "origin/main")
	if err != nil {
		return nil
	}
	owed := r.Owed()
	return owed[:min(len(owed), staledocs.PerChange)]
}

// WHY: Claude Code refuses the tool call on exit code 2 and shows the agent
// the reason written to stderr.
func preToolHook(cmd *cobra.Command, _ []string) error {
	h, err := readHookInput(cmd.InOrStdin())
	if err != nil {
		return err
	}
	r := string(pretool.Check(pretool.Tool{Name: h.ToolName, Command: h.ToolInput.Command, FilePath: h.ToolInput.FilePath, Dir: h.Cwd}))
	if r == "" && h.ToolName == "Bash" && pretool.Commits(h.ToolInput.Command) {
		r = beforeCommit(cmd.Context(), h.Cwd)
	}
	if r == "" {
		return nil
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "rumblestrip: "+r); err != nil {
		return fmt.Errorf("write the hook's refusal: %w", err)
	}
	return exitCode(2)
}

func postEditHook(cmd *cobra.Command, _ []string) error {
	h, err := readHookInput(cmd.InOrStdin())
	if err != nil || !strings.HasSuffix(h.ToolInput.FilePath, ".go") {
		return err
	}
	root, err := moduleRoot(filepath.Dir(h.ToolInput.FilePath))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, filepath.Dir(h.ToolInput.FilePath))
	if err != nil {
		return fmt.Errorf("post-edit: %w", err)
	}
	o := check.Options{Dir: root, GOOS: []string{"linux", "windows"}}
	return feedback(cmd.ErrOrStderr(), check.Package(cmd.Context(), o, "./"+filepath.ToSlash(rel)))
}

func stopHook(cmd *cobra.Command, base string) error {
	h, err := readHookInput(cmd.InOrStdin())
	if err != nil || h.StopHookActive {
		return err
	}
	o := check.Options{Dir: h.Cwd, Base: base, GOOS: []string{"linux", "windows"}}
	if root, err := moduleRoot(h.Cwd); err == nil {
		o.Dir = root
	}
	results, err := check.Run(cmd.Context(), o)
	if err != nil {
		return err
	}
	return feedback(cmd.ErrOrStderr(), results)
}

// WHY: only failures are written, to stderr with code 2, the one channel
// Claude Code shows the agent; a clean run says nothing.
func feedback(stderr io.Writer, results []check.Result) error {
	var b strings.Builder
	failing := 0
	for _, r := range results {
		if r.Failed() {
			failing++
			writeResult(&b, r)
		}
	}
	if failing == 0 {
		return nil
	}
	fmt.Fprintf(&b, "rumblestrip: %d gates fail; fix them before you finish. Each line says the rule and how to fix it.\n", failing)
	if _, err := io.WriteString(stderr, b.String()); err != nil {
		return fmt.Errorf("write the hook's feedback: %w", err)
	}
	return exitCode(2)
}

func moduleRoot(dir string) (string, error) {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
	}
}

func bypassLines(ctx context.Context, dir string) []string {
	d, err := guard.Read(ctx, dir, "origin/main")
	if err != nil {
		return nil
	}
	var lines []string
	for _, f := range guard.Check(d) {
		if !f.NeedsApproval() {
			lines = append(lines, fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.Rule, f.Message))
		}
	}
	return lines
}

// WHY: what the merge check refuses outright, stopped before it enters the
// history: a bypass in the change, a secret, a key or an env file staged.
// A repository with no origin/main yet has no change to read: the bypasses
// wait for the merge check.
func beforeCommit(ctx context.Context, dir string) string {
	lines := bypassLines(ctx, dir)
	leaks, err := gitleaks.Scan(ctx, dir, "--pre-commit", "--staged")
	if err != nil {
		lines = append(lines, "the staged lines couldn't be scanned for secrets: "+err.Error())
	}
	for _, l := range leaks {
		lines = append(lines, fmt.Sprintf("%s:%d: %s: %s", l.File, l.Line, l.Rule, l.Description))
	}
	if len(lines) == 0 {
		return ""
	}
	return "fix these before you commit:\n" + strings.Join(lines, "\n")
}
