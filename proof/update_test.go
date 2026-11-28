package proof_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const checkWorkflow = "../.github/workflows/rumblestrip-check.yml"

func stepOutput(t *testing.T, script string, env ...string) (string, int) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "output")
	_, code, _ := runStep(t, script, append(env, "GITHUB_OUTPUT="+out)...)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data)), code
}

// WHY: a bot's dependency update has no agent's work to judge, so only what
// shows the update works runs on it: the tests, which build the code, the
// spec and the known vulnerabilities.
func TestABotsDependencyUpdateRunsOnlyWhatShowsItWorks(t *testing.T) {
	author := stepRun(t, checkWorkflow, "Is it a bot's dependency update?")
	for login, want := range map[string]string{
		"dependabot[bot]": "update=true", "renovate[bot]": "update=true",
		"t7-companion-maintainer[bot]": "update=false", "octo": "update=false",
	} {
		if got, code := stepOutput(t, author, "AUTHOR="+login); code != 0 || got != want {
			t.Errorf("a pull request by %s: exit %d, %q, want %q", login, code, got, want)
		}
	}
	gate := stepRun(t, checkWorkflow, "Does this gate run on this change?")
	for _, c := range []struct{ update, gate, want string }{
		{"true", "go-test", "runs=true"},
		{"true", "spec", "runs=true"},
		{"true", "govulncheck", "runs=true"},
		{"true", "docs", "runs=false"},
		{"true", "title", "runs=false"},
		{"true", "guard", "runs=false"},
		{"true", "mutate", "runs=false"},
		{"true", "golangci-lint-linux", "runs=false"},
		{"false", "docs", "runs=true"},
	} {
		if got, code := stepOutput(t, gate, "UPDATE="+c.update, "GATE="+c.gate); code != 0 || got != c.want {
			t.Errorf("gate %s, update %s: exit %d, %q, want %q", c.gate, c.update, code, got, c.want)
		}
	}
}

// WHY: Rumblestrip posts no comment and no label on a bot's dependency
// update, yet what it changes of the owner's, a pin in .github/ say, still
// waits for their approval.
func TestABotsDependencyUpdateGetsNoCommentButStillWaitsForTheOwner(t *testing.T) {
	sheet := stepRun(t, checkWorkflow, "Post the review sheet and the owner's approval status")
	fake := `gh() {
  echo "gh $*" >> "$LOG"
  case "$*" in *comments*--jq*) echo "https://x/c" ;; esac
}
rumblestrip() {
  case "$*" in
  *--approval*) echo "$STATE" ;;
  *--labels*) echo "+human-review" ;;
  *) echo "the sheet" ;;
  esac
}
`
	for _, c := range []struct {
		update, state, status string
		comments              bool
	}{
		{"true", "none", "state=success", false},
		{"true", "waiting", "state=pending", false},
		{"false", "waiting", "state=pending", true},
	} {
		out := filepath.Join(t.TempDir(), "output")
		_, code, log := runStep(t, fake+sheet, "UPDATE="+c.update, "STATE="+c.state, "PR=7", "HEAD=abc", "BASE=main", "OWNER=octo", "APPROVED=false", "GITHUB_OUTPUT="+out)
		commented := strings.Contains(log, "issues/7/comments") || strings.Contains(log, "label")
		if code != 0 || commented != c.comments || !strings.Contains(log, c.status) {
			t.Errorf("update %s, approval %s: exit %d, commented %v, want %v, and %s:\n%s", c.update, c.state, code, commented, c.comments, c.status, log)
		}
	}
}
