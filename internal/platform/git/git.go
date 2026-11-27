// Package git runs git commands in a repository.
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ErrOptionLikeRef is returned for a ref git would read as an option.
var ErrOptionLikeRef = errors.New("a ref may not start with '-'")

// Run runs git with args in dir and returns its standard output.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exit.Stderr)))
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([^/]+/[^/]+?)(?:\.git)?/?$`)

// OriginRepo returns the "owner/name" of the GitHub repository the origin
// remote of the repository at dir names.
func OriginRepo(ctx context.Context, dir string) (string, error) {
	url, err := Run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", fmt.Errorf("origin is %s, not a GitHub repository", strings.TrimSpace(url))
	}
	return m[1], nil
}

// CheckRef refuses a ref that git would take for an option.
func CheckRef(ref string) error {
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("%q: %w", ref, ErrOptionLikeRef)
	}
	return nil
}
