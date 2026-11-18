package proof_test

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/rumblestrip-go/guard"
	"github.com/McReaper/rumblestrip-go/reread"
	"github.com/McReaper/rumblestrip-go/setup"
)

type ruleset struct {
	Target     string `json:"target"`
	Conditions struct {
		RefName struct {
			Include []string `json:"include"`
		} `json:"ref_name"`
	} `json:"conditions"`
	BypassActors []struct {
		ActorType  string `json:"actor_type"`
		BypassMode string `json:"bypass_mode"`
	} `json:"bypass_actors"`
	Rules []struct {
		Type       string          `json:"type"`
		Parameters json.RawMessage `json:"parameters"`
	} `json:"rules"`
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestTheKitsRulesetLetsOnlyTheOwnerMergeAnApprovedCheckedChange(t *testing.T) {
	var r ruleset
	readJSON(t, "../platform/ruleset-rumblestrip.json", &r)
	var types []string
	for _, rule := range r.Rules {
		types = append(types, rule.Type)
		if rule.Type == "pull_request" {
			// WHY: the merge needs an approval of the last push; earlier
			// approvals stay on record for the merge check, which keeps a
			// protected file approved while its content is the one approved.
			for _, p := range []string{`"require_code_owner_review": true`, `"dismiss_stale_reviews_on_push": false`, `"require_last_push_approval": true`} {
				if !strings.Contains(string(rule.Parameters), p) {
					t.Errorf("pull_request rule lacks %s", p)
				}
			}
		}
	}
	for _, want := range []string{"update", "pull_request", "required_status_checks", "non_fast_forward", "deletion"} {
		if !slices.Contains(types, want) {
			t.Errorf("the ruleset lacks a %s rule", want)
		}
	}
	if len(r.BypassActors) != 1 || r.BypassActors[0].BypassMode != "pull_request" {
		t.Errorf("only the admin role may bypass, through a pull request: %+v", r.BypassActors)
	}
}

// WHY: an app's change merges by itself once green. GitHub asks no review
// but the code owners' on the harness and keeps every approval on record;
// Rumblestrip's checks, which only GitHub Actions may report, holds the rest that
// is the owner's against the content they approved. No update rule, so
// Rumblestrip's merge job can land the change.
func TestTheAppsRulesetMergesAGreenChangeAndHoldsWhatIsTheOwners(t *testing.T) {
	var r ruleset
	readJSON(t, "../setup/files/ruleset.json", &r)
	var types []string
	for _, rule := range r.Rules {
		types = append(types, rule.Type)
		var want []string
		switch rule.Type {
		case "pull_request":
			want = []string{`"required_approving_review_count": 0`, `"require_code_owner_review": true`, `"dismiss_stale_reviews_on_push": false`, `"require_last_push_approval": false`}
		case "required_status_checks":
			want = []string{`{ "context": "` + appCheck(t) + `", "integration_id": 15368 }`, `{ "context": "` + approvalStatus + `", "integration_id": 15368 }`}
		}
		for _, p := range want {
			if !strings.Contains(string(rule.Parameters), p) {
				t.Errorf("%s rule lacks %s", rule.Type, p)
			}
		}
	}
	if slices.Contains(types, "update") {
		t.Error("an update rule stops Rumblestrip's merge job")
	}
	for _, want := range []string{"pull_request", "required_status_checks", "non_fast_forward", "deletion"} {
		if !slices.Contains(types, want) {
			t.Errorf("the ruleset lacks a %s rule", want)
		}
	}
}

// WHY: a job of a called workflow reports its check as "<caller's job's
// name> / <its name>": a ruleset requiring the merge check's job by its
// own name waits for a check that never comes, and no change merges.
func appCheck(t *testing.T) string {
	t.Helper()
	var caller []string
	for _, body := range jobs(t, "../setup/files/rumblestrip.yml") {
		if strings.Contains(body, "/rumblestrip-check.yml@") {
			caller = append(caller, jobName(t, body))
		}
	}
	check, ok := jobs(t, "../.github/workflows/rumblestrip-check.yml")["check"]
	if len(caller) != 1 || !ok {
		t.Fatalf("rumblestrip.yml calls the merge check from one job, its check job reports: %v", caller)
	}
	return caller[0] + " / " + jobName(t, check)
}

func jobName(t *testing.T, body string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^    name: (.+)$`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("a job has no name:\n%s", body)
	}
	return m[1]
}

// WHY: the ruleset requires the status by this context, and the review
// sheet's job posts it under the same one.
const approvalStatus = "Rumblestrip / Owner approval"

func jobs(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "\njobs:\n")
	if !ok {
		t.Fatalf("%s has no jobs", path)
	}
	job := regexp.MustCompile(`^  ([a-z][a-z0-9_-]*):\s*$`)
	out := map[string]string{}
	id := ""
	for line := range strings.Lines(body) {
		if m := job.FindStringSubmatch(strings.TrimRight(line, "\n")); m != nil {
			id = m[1]
			out[id] = ""
		} else if id != "" {
			out[id] += line
		}
	}
	return out
}

// WHY: the merge check runs on pull requests only, so every check it
// reports counts for the ruleset; the owner's approval re-runs the review
// sheet's job of that run, from a workflow of its own, whose jobs never
// show as skipped on the pull request.
func TestTheOwnersApprovalReRunsTheMergeCheckFromAWorkflowOfItsOwn(t *testing.T) {
	for path, want := range map[string][]string{
		"../setup/files/rumblestrip.yml":          {"/rumblestrip-check.yml@", "    name: Pull request\n"},
		"../setup/files/rumblestrip-approval.yml": {"/rumblestrip-approval.yml@", "actions: write", "statuses: read"},
	} {
		js := jobs(t, path)
		if len(js) != 1 {
			t.Errorf("%s has jobs %v, want one", path, slices.Sorted(maps.Keys(js)))
		}
		for _, body := range js {
			for _, w := range want {
				if !strings.Contains(body, w) {
					t.Errorf("%s's job lacks %q:\n%s", path, w, body)
				}
			}
		}
	}
	for path, trigger := range map[string]string{"../setup/files/rumblestrip.yml": "\n  pull_request:\n", "../setup/files/rumblestrip-approval.yml": "\n  pull_request_review:\n"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		on, _, _ := strings.Cut(string(data), "\njobs:\n")
		if strings.Count(on, "\n  pull_request") != 1 || !strings.Contains(on, trigger) {
			t.Errorf("%s runs on more or other than%s", path, trigger)
		}
	}
}

// WHY: GitHub counts a skipped check as passed. An edit of the description
// alone runs nothing at all, by the caller's condition, rather than a run
// whose Check skips; and Check runs whatever the jobs before it did, so it
// fails rather than skips.
func TestTheRequiredCheckNeverSkips(t *testing.T) {
	pr := jobs(t, "../setup/files/rumblestrip.yml")["pr"]
	if !strings.Contains(pr, "    if: github.event.action != 'edited' || github.event.changes.title || github.event.changes.base\n") {
		t.Errorf("rumblestrip.yml's job runs on every edit:\n%s", pr)
	}
	called := jobs(t, "../.github/workflows/rumblestrip-check.yml")
	if !strings.Contains(called["check"], "    if: ${{ !cancelled() }}\n") {
		t.Errorf("Check skips when a job before it failed:\n%s", called["check"])
	}
	for id, body := range called {
		if id != "check" && id != "merge" && strings.Contains(body, "\n    if:") {
			t.Errorf("rumblestrip-check.yml's %s job has a condition: a job it skips makes Check pass", id)
		}
	}
}

// WHY: the owner's approval status is required, and only a job that runs
// none of the change's code may post it: code under test could read the
// job's token. The review sheet's job alone writes statuses.
func TestOnlyTheReviewSheetsJobPostsTheOwnersApproval(t *testing.T) {
	for id, body := range jobs(t, "../.github/workflows/rumblestrip-check.yml") {
		writes := strings.Contains(body, "statuses: write")
		if writes != (id == "review") {
			t.Errorf("job %s writes statuses: %v", id, writes)
		}
	}
	review := jobs(t, "../.github/workflows/rumblestrip-check.yml")["review"]
	if !strings.Contains(review, `-f context="`+approvalStatus+`"`) || strings.Contains(review, "go test") || strings.Contains(review, "rumblestrip check") {
		t.Errorf("the review sheet's job doesn't post %q, or runs the change's code:\n%s", approvalStatus, review)
	}
}

func TestTheTagRulesetLetsOnlyTheOwnerTagARelease(t *testing.T) {
	var r ruleset
	readJSON(t, "../setup/files/ruleset-tags.json", &r)
	if r.Target != "tag" || !slices.Equal(r.Conditions.RefName.Include, []string{"~ALL"}) {
		t.Errorf("the ruleset must hold every tag: target %q, include %v", r.Target, r.Conditions.RefName.Include)
	}
	var types []string
	for _, rule := range r.Rules {
		types = append(types, rule.Type)
	}
	for _, want := range []string{"creation", "update", "deletion"} {
		if !slices.Contains(types, want) {
			t.Errorf("the tag ruleset lacks a %s rule", want)
		}
	}
	if len(r.BypassActors) != 1 || r.BypassActors[0].ActorType != "RepositoryRole" || r.BypassActors[0].BypassMode != "always" {
		t.Errorf("only the admin role may tag, from its own clone: %+v", r.BypassActors)
	}
}

type claudeSettings struct {
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
	Permissions    struct {
		Deny []string `json:"deny"`
	} `json:"permissions"`
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func initFiles(t *testing.T) map[string]string {
	t.Helper()
	fs, err := setup.Files(setup.Options{Version: setup.Version{Tag: "v0.2.0", Commit: strings.Repeat("ab", 20)}, Owner: "OWNER", OwnerEmail: "1+OWNER@users.noreply.github.com"})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range fs {
		out[f.Path] = string(f.Content)
	}
	return out
}

// WHY: deny rules a plugin can't carry; the repository's settings, written
// by rumblestrip init, hold them and turn Rumblestrip's plugin on, managed settings do
// both where the agent can't undo them, and the plugin's hooks refuse what
// a deny rule's text misses (git -C . push).
func TestTheAgentSettingsDenyBypassesAndThePluginRunsTheKitHooks(t *testing.T) {
	var project, managed, plugin claudeSettings
	if err := json.Unmarshal([]byte(initFiles(t)[".claude/settings.json"]), &project); err != nil {
		t.Fatal(err)
	}
	readJSON(t, "../platform/claude/managed-settings.json", &managed)
	readJSON(t, "../plugin/hooks/hooks.json", &plugin)
	for _, want := range []string{"Bash(git commit --no-verify *)", "Bash(git push --force *)", "Bash(git push --force-with-lease * main)", "Bash(gh pr merge *)", "Edit(./.github/**)"} {
		if !slices.Contains(project.Permissions.Deny, want) {
			t.Errorf("deny lacks %s", want)
		}
	}
	// WHY: the agent rebases its own pull request while the owner hasn't
	// approved it; main's ruleset refuses any force push there anyway.
	if slices.Contains(project.Permissions.Deny, "Bash(git push --force-with-lease *)") {
		t.Error("deny refuses --force-with-lease on the agent's own branches")
	}
	if !slices.Equal(project.Permissions.Deny, managed.Permissions.Deny) || !project.EnabledPlugins["rumblestrip@rumblestrip-go"] || !managed.EnabledPlugins["rumblestrip@rumblestrip-go"] {
		t.Errorf("the repository's and the managed settings differ: %+v, %+v", project, managed)
	}
	for event, command := range map[string]string{"PreToolUse": "hook pre-tool", "PostToolUse": "hook post-edit", "Stop": "hook stop"} {
		groups := plugin.Hooks[event]
		if len(groups) == 0 || len(groups[0].Hooks) == 0 || !strings.Contains(groups[0].Hooks[0].Command, `"${CLAUDE_PLUGIN_ROOT}/bin/rumblestrip" `+command) {
			t.Errorf("the plugin's %s hook does not run rumblestrip %s: %+v", event, command, groups)
		}
	}
}

// WHY: the commit on the default branch is Rumblestrip's merge job's; the
// agent's own commits and pull requests name Rumblestrip, not Claude, without
// a prompt saying so.
func TestTheSettingsNameTheKitInTheAgentsCommitsAndPullRequests(t *testing.T) {
	var settings struct {
		Attribution struct {
			Commit     string `json:"commit"`
			PR         string `json:"pr"`
			SessionURL *bool  `json:"sessionUrl"`
		} `json:"attribution"`
	}
	if err := json.Unmarshal([]byte(initFiles(t)[".claude/settings.json"]), &settings); err != nil {
		t.Fatal(err)
	}
	a := settings.Attribution
	if a.Commit != "Co-authored-by: OWNER <1+OWNER@users.noreply.github.com>\nBuilt-with: Rumblestrip <https://github.com/McReaper/rumblestrip-go>" ||
		!strings.HasSuffix(a.PR, "Built with [Rumblestrip](https://github.com/McReaper/rumblestrip-go)") ||
		a.SessionURL == nil || *a.SessionURL {
		t.Errorf("attribution = %+v", a)
	}
}

// WHY: plugin/bin/rumblestrip runs in a repository rumblestrip init set
// up, with a go, a docker and a uname on the PATH that say how they were
// called; docker keeps its one container's state in a file.
type launcher struct{ dir, fakes, tmp, log, commit string }

const fakeDocker = `#!/bin/sh
echo "docker $*" >> "$FAKE_DOCKER_LOG"
state=$FAKE_DOCKER_LOG.state
case $1 in
  image) [ "${FAKE_IMAGE-}" = present ] ;;
  inspect) cat "$state" 2>/dev/null ;;
  run) echo running > "$state" ;;
  rm) rm -f "$state" ;;
  exec)
    case ${FAKE_EXEC-} in
      gone) rm -f "$state"; exit 1 ;;
      refuse) echo "docker $*"; exit 2 ;;
      *) echo "docker $*" ;;
    esac ;;
esac
`

const fakeUname = `#!/bin/sh
case $1 in
  -m) echo "${FAKE_ARCH:-x86_64}" ;;
  -s) echo "${FAKE_OS:-Linux}" ;;
esac
`

func newLauncher(t *testing.T) launcher {
	t.Helper()
	l := launcher{dir: t.TempDir(), fakes: t.TempDir(), tmp: t.TempDir(), commit: strings.Repeat("ab", 20)}
	l.log = filepath.Join(l.tmp, "docker.log")
	fs, err := setup.Files(setup.Options{Version: setup.Version{Tag: "v0.3.0", Commit: l.commit}, Owner: "octo", OwnerEmail: "1+octo@users.noreply.github.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setup.Write(l.dir, fs); err != nil {
		t.Fatal(err)
	}
	gitIn(t, l.dir, "init", "-q")
	gitIn(t, l.dir, "add", "-A")
	gitIn(t, l.dir, "commit", "-q", "-m", "init")
	fakes := map[string]string{"go": "#!/bin/sh\necho \"go $*\"\n", "docker": fakeDocker, "uname": fakeUname}
	for name, script := range fakes {
		if err := os.WriteFile(filepath.Join(l.fakes, name), []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		runIn(t, place{dir: l.fakes}, "chmod", "+x", name)
	}
	l.calls(t)
	return l
}

// WHY: env sets the fakes: FAKE_IMAGE=present has the image pulled,
// FAKE_EXEC says how docker exec ends, FAKE_OS and FAKE_ARCH what uname
// says.
func (l launcher) run(t *testing.T, env string, args ...string) (string, int) {
	t.Helper()
	path, err := filepath.Abs("../plugin/bin/rumblestrip")
	if err != nil {
		t.Fatal(err)
	}
	script := `PATH="$(cd "$1" && pwd):$PATH"; l=$2; shift 2; exec "$l" "$@"`
	cmd := exec.CommandContext(t.Context(), "sh", append([]string{"-c", script, "sh", filepath.ToSlash(l.fakes), filepath.ToSlash(path)}, args...)...)
	cmd.Dir = l.dir
	cmd.Env = append(os.Environ(), "FAKE_DOCKER_LOG="+filepath.ToSlash(l.log), "TMPDIR="+filepath.ToSlash(l.tmp))
	cmd.Env = append(cmd.Env, strings.Fields(env)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if code := exitCode(err); code > 0 {
		return string(out), code
	}
	t.Fatalf("the launcher: %v\n%s", err, out)
	return "", 0
}

// WHY: what docker was called with since the last look.
func (l launcher) calls(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(l.log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// WHY: the image is pulled in the background, so the call that starts the
// pull returns first.
func (l launcher) logged(t *testing.T, want string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if data, err := os.ReadFile(l.log); err == nil && strings.Contains(string(data), want) {
			return
		}
	}
	data, err := os.ReadFile(l.log)
	t.Errorf("docker was never called with %q: %v\n%s", want, err, data)
}

// WHY: the plugin's hooks run rumblestrip at the commit the repository's
// workflow pins, so the agent's checks are the merge check's; a launcher
// that misreads the pin runs nothing, and every hook fails. With Docker, a
// check runs in the image of that commit, which holds every tool; until the
// image is pulled, and for a command that needs the host's credentials, go
// run builds rumblestrip.
func launcherRunsThePinnedVersion(t *testing.T) {
	t.Helper()
	l := newLauncher(t)
	goRun := "go run github.com/McReaper/rumblestrip-go/cmd/rumblestrip@" + l.commit + " "
	image := "ghcr.io/mcreaper/rumblestrip:sha-" + l.commit
	if out, _ := l.run(t, "", "check"); out != goRun+"check\n" {
		t.Errorf("a check before the image is pulled ran %q, want %q", out, goRun+"check\n")
	}
	l.logged(t, "docker pull --quiet "+image)
	l.calls(t)
	launcherRunsChecksInTheImage(t, l, image)
	worktree := l
	worktree.dir = filepath.Join(t.TempDir(), "worktree")
	gitIn(t, l.dir, "worktree", "add", "-q", worktree.dir)
	for _, c := range []struct {
		who  string
		l    launcher
		env  string
		args []string
	}{
		{"pr, which needs the host's credentials,", l, "FAKE_IMAGE=present", []string{"pr", "stack"}},
		{"RUMBLESTRIP_DOCKER=0", l, "FAKE_IMAGE=present RUMBLESTRIP_DOCKER=0", []string{"check"}},
		{"an arm64 machine, which the image doesn't run on,", l, "FAKE_IMAGE=present FAKE_ARCH=arm64", []string{"check"}},
		{"a linked worktree, whose git directory the container doesn't mount,", worktree, "FAKE_IMAGE=present", []string{"check"}},
	} {
		want := goRun + strings.Join(c.args, " ") + "\n"
		if out, _ := c.l.run(t, c.env, c.args...); out != want {
			t.Errorf("%s ran %q, want %q", c.who, out, want)
		}
	}
}

// WHY: the first check starts the repository's container, as the user but
// on Windows; the next ones reuse it. A refusal comes back with its code,
// and a check whose container stopped under it runs again on the host.
func launcherRunsChecksInTheImage(t *testing.T, l launcher, image string) {
	t.Helper()
	out, _ := l.run(t, "FAKE_IMAGE=present", "hook", "stop")
	container := regexp.MustCompile(` (rumblestrip-\d+-` + l.commit[:12] + `) rumblestrip hook stop\n$`).FindStringSubmatch(out)
	if !strings.HasPrefix(out, "docker exec --interactive --workdir /work/ ") || !strings.Contains(out, " --env RUMBLESTRIP_ROOT=/work ") || container == nil {
		t.Fatalf("a hook with the image pulled ran %q", out)
	}
	started := regexp.MustCompile(`docker run --detach --rm --init --name ` + container[1] + ` --mount type=bind,source=\S+,target=/work --volume rumblestrip-go:/go --user \d+:\d+ ` + regexp.QuoteMeta(image) + ` sleep 86400\n`)
	if calls := l.calls(t); !started.MatchString(calls) {
		t.Errorf("the first hook started no container as the user:\n%s", calls)
	}
	out, _ = l.run(t, "FAKE_IMAGE=present", "check")
	if calls := l.calls(t); !strings.HasPrefix(out, "docker exec ") || strings.Contains(calls, "docker run ") {
		t.Errorf("a check with the container up ran %q, and docker:\n%s", out, calls)
	}
	if out, code := l.run(t, "FAKE_IMAGE=present FAKE_EXEC=refuse", "hook", "pre-tool"); code != 2 || strings.Contains(out, "go run") {
		t.Errorf("a refusal in the image ended %d: %q", code, out)
	}
	goRun := "go run github.com/McReaper/rumblestrip-go/cmd/rumblestrip@" + l.commit + " hook stop\n"
	if out, code := l.run(t, "FAKE_IMAGE=present FAKE_EXEC=gone", "hook", "stop"); code != 0 || out != goRun {
		t.Errorf("a hook whose container stopped under it ran %q and ended %d, want %q", out, code, goRun)
	}
	l.calls(t)
	out, _ = l.run(t, "FAKE_IMAGE=present FAKE_OS=MINGW64_NT-10.0-26200", "check")
	if calls := l.calls(t); !strings.HasPrefix(out, "docker exec ") || !strings.Contains(calls, "docker run --detach ") || strings.Contains(calls, "--user") {
		t.Errorf("on Windows, a check ran %q, and docker:\n%s", out, calls)
	}
}

func TestTheAppCodeOwnersHoldTheHarnessAndLeaveTheSpecToTheCheck(t *testing.T) {
	var owned []string
	for l := range strings.Lines(initFiles(t)[".github/CODEOWNERS"]) {
		if p, ok := strings.CutSuffix(strings.TrimSpace(l), " @OWNER"); ok {
			owned = append(owned, p)
		}
	}
	var want []string
	for _, h := range guard.Harness() {
		if h == "GLOSSARY.md" {
			want = append(want, "**/"+h)
		} else {
			want = append(want, "/"+h)
		}
	}
	if !slices.Equal(owned, want) {
		t.Errorf("CODEOWNERS owns %q, the guard's harness is %q", owned, want)
	}
}

// WHY: what rumblestrip init writes is what every app's CI runs; it passes the
// workflow gates Rumblestrip holds an app's workflows to.
func TestTheCIKitInitWritesPassesTheWorkflowGates(t *testing.T) {
	withZizmor(t)
	dir := t.TempDir()
	fs, err := setup.Files(setup.Options{Version: setup.Version{Tag: "v0.2.0", Commit: strings.Repeat("ab", 20)}, Owner: "octo", OwnerEmail: "1+octo@users.noreply.github.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setup.Write(dir, fs); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "add", "-A")
	check(t, nil, actionlintFindings(t, dir))
	check(t, nil, zizmorFindings(t, dir))
}

// WHY: the plugin is how an app gets Rumblestrip's skills, rules and hooks;
// every file its manifests and hooks name must be there, the wrapper they
// call executable, and each skill named as Claude Code lists it.
func TestThePluginShipsWhatItsManifestsAndHooksName(t *testing.T) {
	var market struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &market)
	if len(market.Plugins) != 1 || market.Plugins[0].Name != "rumblestrip" || market.Plugins[0].Source != "./plugin" {
		t.Fatalf("the marketplace: %+v", market)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	readJSON(t, "../plugin/.claude-plugin/plugin.json", &manifest)
	if manifest.Name != "rumblestrip" {
		t.Errorf("the plugin is named %q", manifest.Name)
	}
	mode, err := exec.CommandContext(t.Context(), "git", "ls-files", "-s", "../plugin/bin/rumblestrip").Output()
	if err != nil || !strings.HasPrefix(string(mode), "100755 ") {
		t.Errorf("plugin/bin/rumblestrip is not executable in git: %q, %v", mode, err)
	}
	runIn(t, place{dir: ".."}, "shellcheck", "plugin/bin/rumblestrip")
	launcherRunsThePinnedVersion(t)
	rules, err := os.ReadFile("../plugin/rules.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile("`rumblestrip:([a-z-]+)`").FindAllStringSubmatch(string(rules), -1) {
		skill, err := os.ReadFile(filepath.Join("../plugin/skills", m[1], "SKILL.md"))
		if err != nil || !strings.Contains(string(skill), "\nname: "+m[1]+"\n") {
			t.Errorf("rules.md names rumblestrip:%s, which no skill of that name holds: %v", m[1], err)
		}
	}
}

func hookRun(t *testing.T, input string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), kitCommand(t, "rumblestrip"), append([]string{"hook"}, args...)...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	return string(out), exitCode(err)
}

func editOf(t *testing.T, file string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"tool_name": "Edit", "tool_input": map[string]string{"file_path": filepath.ToSlash(file)}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// WHY: a rule the agent meets only at the end is learned late; after each
// edit, the package's findings reach it through the hook's exit code 2.
func TestThePostEditHookTellsTheAgentWhatItsEditBroke(t *testing.T) {
	dir := overlay(t, caseNamed(t, "m1-simplicity"))
	out, code := hookRun(t, editOf(t, filepath.Join(dir, "internal/notes/internal/m1case/m1.go")), "post-edit")
	if code != 2 || !strings.Contains(out, "cognitive complexity 11") || !strings.Contains(out, "nested 3 deep") {
		t.Errorf("a broken package: exit %d\n%s", code, out)
	}
	clean, err := filepath.Abs(filepath.Join(cleanDir, "internal/notes/internal/add/add.go"))
	if err != nil {
		t.Fatal(err)
	}
	if out, code := hookRun(t, editOf(t, clean), "post-edit"); code != 0 || out != "" {
		t.Errorf("a clean package: exit %d, said %q", code, out)
	}
}

// WHY: in Rumblestrip's image, the launcher mounts the repository at
// RUMBLESTRIP_ROOT, and Claude Code still gives the hooks the host's paths,
// a Windows one included: a hook that took them as given would check a
// file that isn't there.
func TestTheHooksReadTheHostsPathsInTheImage(t *testing.T) {
	dir := overlay(t, caseNamed(t, "m1-simplicity"))
	t.Setenv("RUMBLESTRIP_ROOT", dir)
	for host, file := range map[string]string{
		`C:\Users\Dev\app`:  `c:\users\dev\app\internal\notes\internal\m1case\m1.go`,
		"C:/Users/Dev/app/": "/c/Users/Dev/app/internal/notes/internal/m1case/m1.go",
		"/home/dev/app":     "/home/dev/app/internal/notes/internal/m1case/m1.go",
	} {
		t.Setenv("RUMBLESTRIP_HOST_ROOT", host)
		if out, code := hookRun(t, editOf(t, file), "post-edit"); code != 2 || !strings.Contains(out, "cognitive complexity 11") {
			t.Errorf("an edit of %s, the repository at %s on the host: exit %d\n%s", file, host, code, out)
		}
	}
	t.Setenv("RUMBLESTRIP_HOST_ROOT", "/home/dev/app")
	if out, _ := hookRun(t, editOf(t, "/home/dev/application/x.go"), "post-edit"); !strings.Contains(out, filepath.FromSlash("/home/dev/application")) {
		t.Errorf("a file outside the repository is read elsewhere: %s", out)
	}
}

func TestTheStopHookLetsTheAgentStopOnItsSecondTry(t *testing.T) {
	if out, code := hookRun(t, `{"cwd":".","stop_hook_active":true}`, "stop"); code != 0 {
		t.Errorf("stop_hook_active: exit %d\n%s", code, out)
	}
}

// WHY: GitHub takes a file's owners from the last pattern that matches it,
// so the rereads, listed with no owner after the harness, need no one.
func TestTheAppCodeOwnersLeaveTheRereadsToTheAgent(t *testing.T) {
	lines := slices.Collect(strings.Lines(initFiles(t)[".github/CODEOWNERS"]))
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	harness := slices.Index(lines, "/.rumblestrip/ @OWNER")
	free := slices.Index(lines, "/"+reread.File)
	if harness < 0 || free < harness {
		t.Errorf("CODEOWNERS must list /%s with no owner after /.rumblestrip/:\n%s", reread.File, strings.Join(lines, "\n"))
	}
}
