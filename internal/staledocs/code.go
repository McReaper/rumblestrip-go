package staledocs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/platform/git"
)

// WHY: the share counts the code a doc can describe: tests, test data and
// generated files are left out. The lines of files git doesn't track yet
// are kept apart, since a diff from a commit never shows them.
type code struct {
	dir       string
	root      *os.Root
	lines     int
	untracked int
	since     map[string]int
}

var generated = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

func measure(ctx context.Context, dir string, root *os.Root) (*code, error) {
	c := &code{dir: dir, root: root, since: map[string]int{}}
	tracked, err := c.goFiles(ctx, "-c")
	if err != nil {
		return nil, err
	}
	untracked, err := c.goFiles(ctx, "-o", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, p := range tracked {
		c.lines += c.lineCount(p)
	}
	for _, p := range untracked {
		c.untracked += c.lineCount(p)
	}
	c.lines += c.untracked
	return c, nil
}

func (c *code) goFiles(ctx context.Context, which ...string) ([]string, error) {
	out, err := git.Run(ctx, c.dir, append(append([]string{"ls-files", "-z"}, which...), "--", "*.go")...)
	if err != nil {
		return nil, err
	}
	var files []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p != "" && c.counts(p) {
			files = append(files, p)
		}
	}
	return files, nil
}

// WHY: a file gone from the working tree still counts, as the code it was:
// removing it changed the code a doc may describe.
func (c *code) counts(p string) bool {
	if strings.HasSuffix(p, "_test.go") || strings.Contains("/"+p, "/testdata/") {
		return false
	}
	b, err := c.root.ReadFile(p)
	if err != nil {
		return true
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() && !strings.HasPrefix(sc.Text(), "package ") {
		if generated.MatchString(sc.Text()) {
			return false
		}
	}
	return true
}

func (c *code) lineCount(p string) int {
	b, err := c.root.ReadFile(p)
	if err != nil || len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

func (c *code) changedSince(ctx context.Context, commit string) (int, error) {
	changed, err := c.linesSince(ctx, commit)
	if err != nil || c.lines == 0 {
		return 0, err
	}
	return changed * 100 / c.lines, nil
}

// WHY: the lines added and removed both count, so rewriting a function
// counts twice its size: it changed that much of what a doc may say.
func (c *code) linesSince(ctx context.Context, commit string) (int, error) {
	if n, ok := c.since[commit]; ok {
		return n, nil
	}
	out, err := git.Run(ctx, c.dir, "diff", "--numstat", "-z", "--no-renames", commit, "--", "*.go")
	if err != nil {
		return 0, err
	}
	changed := c.untracked
	for rec := range strings.SplitSeq(out, "\x00") {
		n, err := c.numstatLines(rec)
		if err != nil {
			return 0, err
		}
		changed += n
	}
	c.since[commit] = changed
	return changed, nil
}

func (c *code) numstatLines(rec string) (int, error) {
	f := strings.SplitN(rec, "\t", 3)
	if len(f) < 3 || f[0] == "-" || !c.counts(f[2]) {
		return 0, nil
	}
	added, err := strconv.Atoi(f[0])
	if err != nil {
		return 0, fmt.Errorf("git diff --numstat: %q: %w", rec, err)
	}
	removed, err := strconv.Atoi(f[1])
	if err != nil {
		return 0, fmt.Errorf("git diff --numstat: %q: %w", rec, err)
	}
	return added + removed, nil
}
