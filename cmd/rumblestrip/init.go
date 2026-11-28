package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/initrepo"
	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
	"github.com/McReaper/rumblestrip-go/internal/platform/git"
	"github.com/McReaper/rumblestrip-go/internal/platform/gomod"
	"github.com/spf13/cobra"
)

const rumblestripModule = "github.com/McReaper/rumblestrip-go"

type initOptions struct {
	owner      string
	ownerEmail string
	version    string
	clientID   string
	app        initAppOptions
}

func initCommand(out io.Writer) *cobra.Command {
	var o initOptions
	cmd := leaf("init", "As the owner, in the repository's clone: the agent's GitHub App, its installation, the files the repository keeps, the rules GitHub enforces, each step once", func(cmd *cobra.Command) error {
		return runInit(cmd.Context(), out, o)
	})
	flags := cmd.Flags()
	flags.StringVar(&o.owner, "owner", "", "the owner's GitHub login, the code owner of the harness (default: the owner of the repository origin names)")
	flags.StringVar(&o.version, "version", "", "the Rumblestrip release to pin, a tag (default: this binary's own version)")
	flags.StringVar(&o.ownerEmail, "owner-email", "", "the owner's address the agent's commits name them co-author by (default: their GitHub no-reply address)")
	flags.StringVar(&o.clientID, "client-id", os.Getenv("RUMBLESTRIP_APP_CLIENT_ID"), "the Client ID of the App the agent acts as here (default: the one "+ghapp.RepoFile+" names, or whose key this machine keeps; RUMBLESTRIP_APP_CLIENT_ID)")
	flags.StringVar(&o.app.name, "app-name", "", "the name of the App init creates when there is none, unique on GitHub (default: <owner>-agent)")
	flags.StringVar(&o.app.org, "org", "", "create that App for this organization rather than your account")
	cmd.AddCommand(initGitHubCommand(out), initAppCommand(out))
	return cmd
}

// WHY: the owner runs one command, and again after each step it leaves
// them, installing the App and pushing the files: it skips what is done,
// the App the repository names or the machine keeps, an installation
// already there, a file the repository has.
func runInit(ctx context.Context, out io.Writer, o initOptions) error {
	repo, err := git.OriginRepo(ctx, ".")
	if err != nil {
		return fmt.Errorf("rumblestrip init: run it in the repository's clone: %w", err)
	}
	if o.owner == "" {
		o.owner, _, _ = strings.Cut(repo, "/")
	}
	app, err := agentApp(ctx, out, o)
	if err != nil {
		return err
	}
	if err := awaitInstallation(ctx, out, installTarget{app, repo}); err != nil {
		return err
	}
	o.clientID = app.clientID
	if err := writeFiles(ctx, out, o); err != nil {
		return err
	}
	return protectOncePushed(ctx, out, initGitHubOptions{repo: repo, clientID: app.clientID, key: app.key})
}

func writeFiles(ctx context.Context, out io.Writer, o initOptions) error {
	v, err := pinned(ctx, o.version)
	if err != nil {
		return err
	}
	if o.ownerEmail == "" {
		if o.ownerEmail, err = noReply(ctx, ghapp.NewClient(), o.owner); err != nil {
			return fmt.Errorf("rumblestrip init: %w", err)
		}
	}
	fs, err := initrepo.Files(initrepo.Options{Version: v, Owner: o.owner, OwnerEmail: o.ownerEmail, ClientID: o.clientID})
	if err != nil {
		return err
	}
	r, err := initrepo.Write(".", fs)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, initReport(r))
	return err
}

func pinned(ctx context.Context, tag string) (initrepo.Version, error) {
	if tag == "" {
		tag = ownVersion()
	}
	if tag == "" {
		return initrepo.Version{}, errors.New("rumblestrip init: this rumblestrip was built from a checkout: name the release to pin with --version")
	}
	commit, err := gomod.Commit(ctx, rumblestripModule, tag)
	if err != nil {
		return initrepo.Version{}, fmt.Errorf("rumblestrip init: %w", err)
	}
	return initrepo.Version{Tag: tag, Commit: commit}, nil
}

// WHY: one App acting in every repository of its owner is the usual setup:
// the App the repository already names stays, and otherwise the one whose
// key this machine keeps is the one. With several, the owner says which.
func appToName(ctx context.Context, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	if id, err := namedApp(ctx); err == nil || !errors.Is(err, errNoApp) {
		return id, err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("the App's key: %w", err)
	}
	ids, err := ghapp.MachineApps(config)
	if err != nil || len(ids) < 2 {
		return strings.Join(ids, ""), err
	}
	return "", fmt.Errorf("this machine keeps the keys of several Apps, %s: name the one the agent acts as here with --client-id", strings.Join(ids, ", "))
}

// WHY: GitHub credits a commit to the account its address belongs to; the
// no-reply one, id+login@users.noreply.github.com, always does, whatever
// the owner made public.
func noReply(ctx context.Context, c ghapp.Client, login string) (string, error) {
	id, err := c.UserID(ctx, login)
	if err != nil {
		return "", fmt.Errorf("the owner's GitHub id: %w", err)
	}
	return ghapp.Identity{Login: login, ID: id}.Email(), nil
}

// WHY: go install and go run record the version they built; a checkout's
// build says (devel), and names no release to pin.
func ownVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Path == rumblestripModule && strings.HasPrefix(info.Main.Version, "v") {
		return info.Main.Version
	}
	return ""
}

var pseudoVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+-(?:.*\.)?\d{14}-([0-9a-f]{7})[0-9a-f]{5}$`)

// WHY: a rumblestrip pinned at a commit no release tags installs as a pseudo-version
// (v0.2.8-0.20261108130046-03876d889681), which says no more to the owner
// than its commit does.
func shortVersion(v string) string {
	if m := pseudoVersion.FindStringSubmatch(v); m != nil {
		return m[1]
	}
	return v
}

func initReport(r initrepo.Report) string {
	var b strings.Builder
	for _, p := range r.Written {
		fmt.Fprintf(&b, "written: %s\n", p)
	}
	for _, p := range r.Same {
		fmt.Fprintf(&b, "already Rumblestrip's: %s\n", p)
	}
	for _, p := range r.Kept {
		fmt.Fprintf(&b, "kept, it differs from Rumblestrip's: %s (delete it and run rumblestrip init again to take Rumblestrip's)\n", p)
	}
	return b.String()
}

type initGitHubOptions struct {
	repo     string
	clientID string
	key      string
}

func initGitHubCommand(out io.Writer) *cobra.Command {
	o := initGitHubOptions{clientID: os.Getenv("RUMBLESTRIP_APP_CLIENT_ID"), key: os.Getenv("RUMBLESTRIP_APP_KEY")}
	cmd := leaf("github", "As the owner: the rulesets that make Rumblestrip's checks the arbiter, merged branches deleted, immutable releases, the App Rumblestrip merges as", func(cmd *cobra.Command) error {
		return runInitGitHub(cmd.Context(), out, o)
	})
	cmd.Flags().StringVar(&o.repo, "repo", "", "the repository, OWNER/REPO (default: the one origin names)")
	cmd.Flags().StringVar(&o.clientID, "client-id", o.clientID, "the Client ID of the GitHub App Rumblestrip merges as (default: the one "+ghapp.RepoFile+" names; RUMBLESTRIP_APP_CLIENT_ID)")
	cmd.Flags().StringVar(&o.key, "key", o.key, "that App's private key file (default: the one this machine keeps for it; RUMBLESTRIP_APP_KEY)")
	return cmd
}

// WHY: the App is checked before anything changes, so a repository is
// never left with Rumblestrip's rules and no App to merge as.
func runInitGitHub(ctx context.Context, out io.Writer, o initGitHubOptions) error {
	app, err := mergeApp(ctx, o)
	if err != nil {
		return err
	}
	lines, err := initrepo.Protect(ctx, initrepo.GH{}, o.repo)
	if err == nil {
		var more []string
		more, err = initrepo.GiveApp(ctx, initrepo.GH{}, app)
		lines = append(lines, more...)
	}
	for _, l := range lines {
		if _, werr := fmt.Fprintln(out, l); werr != nil {
			return errors.Join(err, werr)
		}
	}
	return err
}

func mergeApp(ctx context.Context, o initGitHubOptions) (initrepo.MergeApp, error) {
	var err error
	if o.repo == "" {
		if o.repo, err = git.OriginRepo(ctx, "."); err != nil {
			return initrepo.MergeApp{}, fmt.Errorf("rumblestrip init github: which repository: run it in its clone, or name it with --repo OWNER/REPO: %w", err)
		}
	}
	app := &appOptions{clientID: o.clientID, key: o.key}
	jwt, err := app.jwt(ctx)
	if err != nil {
		return initrepo.MergeApp{}, fmt.Errorf("rumblestrip init github: Rumblestrip merges as a GitHub App: %w", err)
	}
	inst, err := ghapp.NewClient().RepoInstallation(ctx, jwt, o.repo)
	if err != nil {
		return initrepo.MergeApp{}, fmt.Errorf("rumblestrip init github: is the App installed on %s? %w", o.repo, err)
	}
	key, err := ghapp.KeyFile(app.key)
	if err != nil {
		return initrepo.MergeApp{}, fmt.Errorf("rumblestrip init github: %w", err)
	}
	return initrepo.MergeApp{Repo: o.repo, ClientID: app.clientID, Key: key, Installation: inst}, nil
}
