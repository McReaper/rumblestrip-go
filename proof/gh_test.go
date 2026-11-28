package proof_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// WHY: chmod, not os.Chmod: a fake must run, and the lint holds Go's own
// file modes to 0600.
func writeScript(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), "chmod", "+x", path).CombinedOutput(); err != nil {
		t.Fatalf("chmod: %v\n%s", err, out)
	}
}

// WHY: with the owner's account, the agent could merge past every rule, and
// the owner can't approve a pull request their own account opened: the
// plugin's gh runs the real one as the App, through rumblestrip app exec,
// and never runs itself again.
func TestThePluginsGhRunsTheRealOneAsTheApp(t *testing.T) {
	plugin, other := t.TempDir(), t.TempDir()
	shim, err := os.ReadFile("../plugin/bin/gh")
	if err != nil {
		t.Fatal(err)
	}
	writeScript(t, filepath.Join(plugin, "gh"), shim)
	writeScript(t, filepath.Join(plugin, "rumblestrip"), []byte("#!/bin/sh\necho \"rumblestrip $*\"\n"))
	writeScript(t, filepath.Join(other, "gh"), []byte("#!/bin/sh\necho real\n"))
	cmd := exec.CommandContext(t.Context(), "sh", filepath.Join(plugin, "gh"), "pr", "view", "7")
	cmd.Env = append(os.Environ(), "PATH="+plugin+string(os.PathListSeparator)+other+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gh: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if !strings.HasPrefix(got, "rumblestrip app exec -- ") || !strings.HasSuffix(got, "/gh pr view 7") || strings.Contains(got, filepath.Base(plugin)+"/gh") {
		t.Errorf("the plugin's gh ran %q, want rumblestrip app exec -- <the real gh> pr view 7", got)
	}
	cmd = exec.CommandContext(t.Context(), "sh", filepath.Join(plugin, "gh"), "extension", "list")
	cmd.Env = append(os.Environ(), "PATH="+plugin+string(os.PathListSeparator)+other+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "real" {
		t.Errorf("gh extension list, which touches no repository: %q, %v, want the real gh run as the user", out, err)
	}
}

func runAsRepoApp(t *testing.T, repo, config string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), built(t, "rumblestrip"), "app", "installations")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "APPDATA="+config, "XDG_CONFIG_HOME="+config, "RUMBLESTRIP_APP_CLIENT_ID=", "RUMBLESTRIP_APP_KEY=")
	out, err := cmd.CombinedOutput()
	if exitCode(err) != 1 {
		t.Fatalf("rumblestrip app installations: %v, want exit 1\n%s", err, out)
	}
	return string(out)
}

// WHY: the repository names the App its agent acts as, a file the owner
// commits, and the machine keeps its key: a clone with no App named, or a
// machine without the key, hears what to do, and no key is in git.
func TestTheAgentActsAsTheAppItsRepositoryNames(t *testing.T) {
	repo, config := t.TempDir(), t.TempDir()
	gitIn(t, repo, "init", "-q")
	if out := runAsRepoApp(t, repo, config); !strings.Contains(out, "this repository names none: the owner runs rumblestrip init") {
		t.Errorf("a repository naming no App:\n%s", out)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".rumblestrip"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".rumblestrip", "app.json"), []byte(`{"client_id": "Iv1test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(config, "rumblestrip-go", "apps", "Iv1test.pem")
	if out := runAsRepoApp(t, repo, config); !strings.Contains(out, "the key of the App Iv1test isn't on this machine: put it at "+key) {
		t.Errorf("a machine without the App's key:\n%s\nwant it to name %s", out, key)
	}
}
