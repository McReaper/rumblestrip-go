package proof_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
