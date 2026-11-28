package proof_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
)

// WHY: the agent's git acts as the App whatever the user set up: github.com
// hears the App's token, not the user's helper, and no other host hears
// it; its commits are the bot's, unsigned, since the user's key can't sign
// for the bot. The user here signs every commit, through a gpg that fails.
func TestTheAgentsGitCommitsAndPushesAsTheApp(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	global := filepath.Join(home, ".gitconfig")
	user := "[credential]\n\thelper = \"!f() { echo username=owner; echo password=the-owners; }; f\"\n[user]\n\tname = Owner\n\temail = owner@example.com\n[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = false\n"
	if err := os.WriteFile(global, []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	base := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "GH_") })
	base = append(base, "HOME="+home, "GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	env := ghapp.Env(base, ghapp.Token{Value: "ghs_app"}, ghapp.Identity{Login: "app[bot]", ID: 7})
	git := func(stdin string, args ...string) string {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir, cmd.Env, cmd.Stdin = repo, env, strings.NewReader(stdin)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("", "init", "-q")
	for host, want := range map[string]string{"github.com": "password=ghs_app\n", "example.com": "password=the-owners\n"} {
		if got := git("protocol=https\nhost="+host+"\n\n", "credential", "fill"); !strings.Contains(got, want) {
			t.Errorf("git's credential for %s:\n%s\nwant %s", host, got, want)
		}
	}
	git("", "commit", "-q", "--allow-empty", "-m", "a change")
	if got := git("", "log", "-1", "--format=%an <%ae>, %cn, %G?"); got != "app[bot] <7+app[bot]@users.noreply.github.com>, app[bot], N\n" {
		t.Errorf("the agent's commit: %q, want the bot's, unsigned", got)
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
