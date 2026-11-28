package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
	"github.com/McReaper/rumblestrip-go/internal/platform/git"
	"github.com/spf13/cobra"
)

var errNoApp = errors.New("the agent acts on GitHub as your App, and none is set up on this machine: run rumblestrip init app, or give its Client ID and key, --client-id and --key, or RUMBLESTRIP_APP_CLIENT_ID and RUMBLESTRIP_APP_KEY")

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
	app.AddCommand(installationsCommand(o, out), execCommand(o))
	return app
}

func (o *appOptions) jwt() (string, error) {
	if o.clientID == "" && o.key == "" {
		if err := o.fromConfig(); err != nil && !errors.Is(err, ghapp.ErrNoConfig) {
			return "", err
		}
	}
	if o.clientID == "" || o.key == "" {
		return "", errNoApp
	}
	key, err := ghapp.LoadKey(o.key)
	if err != nil {
		return "", err
	}
	return ghapp.JWT(o.clientID, key, time.Now())
}

// WHY: the plugin's gh runs each call through app exec with nothing
// given: the App rumblestrip init app saved is the one.
func (o *appOptions) fromConfig() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("the App: %w", err)
	}
	c, err := ghapp.LoadConfig(dir)
	if err != nil {
		return err
	}
	o.clientID, o.key = c.ClientID, c.Key
	return nil
}

func installationsCommand(o *appOptions, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "installations",
		Short: "List where the app is installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jwt, err := o.jwt()
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
	jwt, err := o.jwt()
	if err != nil {
		return err
	}
	c := ghapp.NewClient()
	if o.installation == 0 {
		if o.installation, err = repoInstallation(ctx, c, jwt); err != nil {
			return err
		}
	}
	tok, err := c.Token(ctx, jwt, o.installation)
	if err != nil {
		return err
	}
	slug, err := c.Slug(ctx, jwt)
	if err != nil {
		return err
	}
	id := ghapp.Identity{Login: slug + "[bot]"}
	if id.ID, err = c.UserID(ctx, id.Login); err != nil {
		return err
	}
	return ghapp.Exec(ctx, argv, ghapp.Bot{Token: tok, Identity: id})
}

// WHY: given no installation, the one that holds the repository the
// command runs in is the one: the plugin's gh runs in the agent's clone.
func repoInstallation(ctx context.Context, c ghapp.Client, jwt string) (int64, error) {
	repo, err := git.OriginRepo(ctx, ".")
	if err != nil {
		return 0, fmt.Errorf("which installation of the App: %w", err)
	}
	inst, err := c.RepoInstallation(ctx, jwt, repo)
	if err != nil {
		return 0, fmt.Errorf("the App isn't installed on %s, or can't be reached: install it there: %w", repo, err)
	}
	return inst.ID, nil
}
