package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/McReaper/rumblestrip-go/internal/initrepo"
	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
	"github.com/spf13/cobra"
)

type initAppOptions struct {
	name string
	org  string
}

func initAppCommand(out io.Writer) *cobra.Command {
	var o initAppOptions
	cmd := leaf("app", "As the owner: create the agent's GitHub App from Rumblestrip's manifest, through your browser", func(cmd *cobra.Command) error {
		return runInitApp(cmd.Context(), out, o)
	})
	flags := cmd.Flags()
	flags.StringVar(&o.name, "name", "", "the App's name, unique on GitHub")
	flags.StringVar(&o.org, "org", "", "create it for this organization rather than your account")
	return cmd
}

func runInitApp(ctx context.Context, out io.Writer, o initAppOptions) error {
	if o.name == "" {
		return errors.New("rumblestrip init app: name the App with --name")
	}
	app, err := createApp(ctx, out, o)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, `
Install it on the repositories the agent works in:
  https://github.com/apps/%s/installations/new

Then run rumblestrip init in each of them: it names the App in %s, and there
the agent's gh acts as it.
`, app.Slug, ghapp.RepoFile)
	return err
}

// WHY: GitHub's manifest flow ends in a redirect to a page of the owner's
// machine carrying a one-time code; a local server serves the form that
// starts it and takes the code back, so the private key goes from GitHub
// to the owner's disk and nowhere else.
func createApp(ctx context.Context, out io.Writer, o initAppOptions) (initrepo.App, error) {
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return initrepo.App{}, fmt.Errorf("rumblestrip init app: %w", err)
	}
	base := "http://" + l.Addr().String()
	manifest, err := initrepo.Manifest(o.name, base+"/created")
	if err != nil {
		return initrepo.App{}, errors.Join(err, l.Close())
	}
	if _, err := fmt.Fprintf(out, "Open %s in your browser, and create the agent's App on GitHub.\n", base); err != nil {
		return initrepo.App{}, errors.Join(err, l.Close())
	}
	app, err := serveUntilCreated(ctx, l, appHandler(manifest, o.org))
	if app.Slug == "" {
		return app, errors.Join(errors.New("rumblestrip init app: no App was created"), err)
	}
	return app, errors.Join(err, saveApp(out, app))
}

// WHY: the server lives until GitHub's redirect brings the App back, the
// owner gives up (the context), or a quarter of an hour passes.
func serveUntilCreated(ctx context.Context, l net.Listener, h func(chan<- initrepo.App) http.Handler) (initrepo.App, error) {
	created := make(chan initrepo.App, 1)
	srv := &http.Server{Handler: h(created), ReadHeaderTimeout: 10 * time.Second}
	var serveErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := srv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	})
	var app initrepo.App
	select {
	case app = <-created:
	case <-ctx.Done():
	case <-time.After(15 * time.Minute):
	}
	err := srv.Shutdown(context.WithoutCancel(ctx))
	wg.Wait()
	return app, errors.Join(err, serveErr)
}

func appHandler(manifest []byte, org string) func(chan<- initrepo.App) http.Handler {
	return func(created chan<- initrepo.App) http.Handler {
		return appMux(manifest, org, created)
	}
}

func appMux(manifest []byte, org string, created chan<- initrepo.App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		if err := initrepo.Form(w, initrepo.NewAppURL(org), manifest); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /created", func(w http.ResponseWriter, r *http.Request) {
		ex := initrepo.Exchanger{Client: &http.Client{Timeout: 30 * time.Second}, BaseURL: "https://api.github.com"}
		app, err := ex.Exchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		select {
		case created <- app:
		default:
		}
		if _, err := io.WriteString(w, "The App is created: go back to your terminal.\n"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	return mux
}

// WHY: the key goes beside the user's other configuration, readable by them
// alone, never into a repository: a repository names the App by its
// Client ID, and the machine finds the key by it.
func saveApp(out io.Writer, app initrepo.App) error {
	config, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("rumblestrip init app: %w", err)
	}
	key, err := ghapp.SaveKey(config, app.ClientID, []byte(app.PEM))
	if err != nil {
		return fmt.Errorf("rumblestrip init app: %w", err)
	}
	_, err = fmt.Fprintf(out, "The App %s is created (id %d, Client ID %s). Its private key: %s\n", app.Slug, app.ID, app.ClientID, key)
	return err
}
