package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/McReaper/rumblestrip-go/internal/initrepo"
	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
)

// WHY: one App acts in every repository of its owner: init creates it
// only when neither the repository nor the machine has one.
func agentApp(ctx context.Context, out io.Writer, o initOptions) (*appOptions, error) {
	id, err := appToName(ctx, o.clientID)
	if err != nil {
		return nil, fmt.Errorf("rumblestrip init: %w", err)
	}
	if id == "" {
		if o.app.name == "" {
			o.app.name = o.owner + "-agent"
		}
		created, err := createApp(ctx, out, o.app)
		if err != nil {
			return nil, err
		}
		id = created.ClientID
	}
	app := &appOptions{clientID: id, key: os.Getenv("RUMBLESTRIP_APP_KEY")}
	if err := app.resolve(ctx); err != nil {
		return nil, fmt.Errorf("rumblestrip init: %w", err)
	}
	return app, nil
}

type installTarget struct {
	app  *appOptions
	repo string
}

// WHY: GitHub installs an App on a repository only from the owner's
// browser: init gives the page, and waits until the installation is there.
func awaitInstallation(ctx context.Context, out io.Writer, t installTarget) error {
	c := ghapp.NewClient()
	jwt, err := t.app.jwt(ctx)
	if err != nil {
		return fmt.Errorf("rumblestrip init: %w", err)
	}
	if _, err := c.RepoInstallation(ctx, jwt, t.repo); err == nil {
		return nil
	}
	slug, err := c.Slug(ctx, jwt)
	if err != nil {
		return fmt.Errorf("rumblestrip init: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Install the App %s on %s: https://github.com/apps/%s/installations/new\nWaiting for it.\n", slug, t.repo, slug); err != nil {
		return fmt.Errorf("rumblestrip init: %w", err)
	}
	return pollInstallation(ctx, c, t)
}

func pollInstallation(ctx context.Context, c ghapp.Client, t installTarget) error {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	deadline := time.After(15 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("rumblestrip init: %w", ctx.Err())
		case <-deadline:
			return fmt.Errorf("rumblestrip init: the App isn't installed on %s after a quarter of an hour: install it, then run rumblestrip init again", t.repo)
		case <-tick.C:
		}
		jwt, err := t.app.jwt(ctx)
		if err != nil {
			return fmt.Errorf("rumblestrip init: %w", err)
		}
		if _, err := c.RepoInstallation(ctx, jwt, t.repo); err == nil {
			return nil
		}
	}
}

// WHY: GitHub's rules take a change to the default branch only through a
// pull request: set before the owner pushes Rumblestrip's files, they
// would hold that push. init sets them once the workflow is there.
func protectOncePushed(ctx context.Context, out io.Writer, o initGitHubOptions) error {
	pushed, err := initrepo.Pushed(ctx, initrepo.GH{}, o.repo)
	if err != nil {
		return err
	}
	if !pushed {
		_, err := io.WriteString(out, "\nCommit and push these files to the default branch, then run rumblestrip init again: it sets the rules GitHub enforces once they are there.\n")
		return err
	}
	if err := runInitGitHub(ctx, out, o); err != nil {
		return err
	}
	_, err = io.WriteString(out, "\nRumblestrip is set up here. In Claude Code, run /rumblestrip:setup once, then ask for a feature.\n")
	return err
}
