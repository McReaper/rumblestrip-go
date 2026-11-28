package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
	"github.com/McReaper/rumblestrip-go/internal/platform/git"
	"github.com/spf13/cobra"
)

var errNoApp = errors.New("the agent acts on GitHub as the App its repository names in " + ghapp.RepoFile + ", and this repository names none: the owner runs rumblestrip init, which names one, or gives one with --client-id and --key, or RUMBLESTRIP_APP_CLIENT_ID and RUMBLESTRIP_APP_KEY")

type appOptions struct {
	clientID     string
	key          string
	installation int64
}

func appCommand(out io.Writer) *cobra.Command {
	o := &appOptions{clientID: os.Getenv("RUMBLESTRIP_APP_CLIENT_ID"), key: os.Getenv("RUMBLESTRIP_APP_KEY")}
	if id, err := strconv.ParseInt(os.Getenv("RUMBLESTRIP_APP_INSTALLATION"), 10, 64); err == nil {
		o.installation = id
	}
	app := &cobra.Command{Use: "app", Short: "Act as the agent's GitHub App"}
	app.PersistentFlags().StringVar(&o.clientID, "client-id", o.clientID, "the app's Client ID (RUMBLESTRIP_APP_CLIENT_ID)")
	app.PersistentFlags().StringVar(&o.key, "key", o.key, "the app's private key file (RUMBLESTRIP_APP_KEY)")
	app.AddCommand(installationsCommand(o, out), execCommand(o), envCommand(o, out))
	return app
}

func (o *appOptions) jwt(ctx context.Context) (string, error) {
	if err := o.resolve(ctx); err != nil {
		return "", err
	}
	key, err := ghapp.LoadKey(o.key)
	if err != nil {
		return "", err
	}
	return ghapp.JWT(o.clientID, key, time.Now())
}

// WHY: the agent's commands ask for the App before each one, with nothing
// given. The repository names its App, in a file the owner commits, and
// the machine keeps that App's key: a clone acts as the App its
// repository names, on any machine, and no key goes into git.
func (o *appOptions) resolve(ctx context.Context) error {
	if o.clientID == "" {
		id, err := namedApp(ctx)
		if err != nil {
			return err
		}
		o.clientID = id
	}
	if o.key != "" {
		return nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("the App's key: %w", err)
	}
	o.key = ghapp.MachineKey(config, o.clientID)
	if _, err := os.Stat(o.key); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the key of the App %s isn't on this machine: put it at %s, or give it with --key or RUMBLESTRIP_APP_KEY", o.clientID, o.key)
	}
	return nil
}

func namedApp(ctx context.Context) (string, error) {
	top, err := git.Toplevel(ctx, ".")
	if err != nil {
		return "", fmt.Errorf("which App: %w", err)
	}
	id, err := ghapp.RepoApp(top)
	if errors.Is(err, ghapp.ErrNoRepoApp) {
		return "", errNoApp
	}
	return id, err
}

func installationsCommand(o *appOptions, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "installations",
		Short: "List where the app is installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jwt, err := o.jwt(cmd.Context())
			if err != nil {
				return err
			}
			list, err := ghapp.NewClient().Installations(cmd.Context(), jwt)
			if err != nil {
				return err
			}
			for _, i := range list {
				if _, err := fmt.Fprintf(out, "%d\t%s\t%s\n", i.ID, i.Account.Login, i.RepositorySelection); err != nil {
					return fmt.Errorf("print installations: %w", err)
				}
			}
			return nil
		},
	}
}

func execCommand(o *appOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec -- command [args...]",
		Short: "Run a command as the app: gh and git push with its token, commits by its bot",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, argv []string) error {
			return runAsApp(cmd.Context(), o, argv)
		},
	}
	cmd.Flags().Int64Var(&o.installation, "installation", o.installation, "the installation's id (RUMBLESTRIP_APP_INSTALLATION)")
	return cmd
}

func runAsApp(ctx context.Context, o *appOptions, argv []string) error {
	b, err := repoBot(ctx, o)
	if err != nil {
		return err
	}
	return ghapp.Exec(ctx, argv, b)
}

func envCommand(o *appOptions, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Print, as shell, what has a command act as the App here: the plugin reads it before each of the agent's",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := repoBot(cmd.Context(), o)
			if err != nil {
				return err
			}
			return printEnv(out, ghapp.Env(nil, b.Token, b.Identity))
		},
	}
}

func printEnv(out io.Writer, env []string) error {
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if _, err := fmt.Fprintf(out, "export %s='%s'\n", k, strings.ReplaceAll(v, "'", `'\''`)); err != nil {
			return fmt.Errorf("print the environment: %w", err)
		}
	}
	return nil
}

// WHY: the App acts on the repository the command runs in, the agent's
// clone, and its token opens that one alone.
func repoBot(ctx context.Context, o *appOptions) (ghapp.Bot, error) {
	if err := o.resolve(ctx); err != nil {
		return ghapp.Bot{}, err
	}
	repo, err := git.OriginRepo(ctx, ".")
	if err != nil {
		return ghapp.Bot{}, fmt.Errorf("which repository: %w", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ghapp.Bot{}, fmt.Errorf("where to keep the token: %w", err)
	}
	g := ghapp.Grant{ClientID: o.clientID, Repo: repo}
	if b, ok := ghapp.CachedBot(cache, g, time.Now()); ok {
		return b, nil
	}
	jwt, err := o.jwt(ctx)
	if err != nil {
		return ghapp.Bot{}, err
	}
	b, err := ghapp.NewClient().Mint(ctx, jwt, ghapp.Scope{Installation: o.installation, Repo: repo})
	if err != nil {
		return ghapp.Bot{}, err
	}
	return b, ghapp.SaveBot(cache, g, b)
}
