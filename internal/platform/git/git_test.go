package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/rumblestrip-go/internal/platform/git"
)

func TestOriginRepoReadsEachFormOfAGitHubURL(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/octo/notes.git":    "octo/notes",
		"https://github.com/octo/notes":        "octo/notes",
		"git@github.com:octo/notes.git":        "octo/notes",
		"ssh://git@github.com/octo/notes.git/": "octo/notes",
	} {
		dir := t.TempDir()
		for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", url}} {
			cmd := exec.CommandContext(t.Context(), "git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		if got, err := git.OriginRepo(t.Context(), dir); err != nil || got != want {
			t.Errorf("origin %s: %q, %v, want %q", url, got, err, want)
		}
	}
}

func TestOriginRepoRefusesAnotherHost(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://gitlab.com/octo/notes.git"}} {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if got, err := git.OriginRepo(t.Context(), dir); err == nil {
		t.Errorf("a GitLab origin: %q, want an error", got)
	}
}

func TestToplevelIsTheRootOfTheWorkingTree(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	got, err := git.Toplevel(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath, err := filepath.EvalSymlinks(got); err != nil || !strings.EqualFold(gotPath, want) {
		t.Errorf("Toplevel from %s = %q, want %q", sub, got, want)
	}
}
