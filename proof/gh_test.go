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
}
