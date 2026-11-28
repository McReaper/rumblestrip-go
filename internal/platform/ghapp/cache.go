package ghapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"time"
)

// WHY: the agent's commands ask for the App before each one, and a token
// lasts an hour: the machine keeps it, with the bot it acts as, until ten
// minutes are left, so a command pays a file's read, not calls to GitHub.
const (
	tokens = "rumblestrip-go/tokens"
	reuse  = 10 * time.Minute
)

var repoShape = regexp.MustCompile(`^[0-9A-Za-z-]+/[0-9A-Za-z._-]+$`)

// Grant names what a token is for: the App, by its Client ID, acting on
// the repository Repo, owner/name.
type Grant struct {
	ClientID string
	Repo     string
}

// WHY: the Client ID and the repository name a file on the machine; any
// other shape could name another.
func (g Grant) file() (string, error) {
	if err := checkClientID(g.ClientID); err != nil {
		return "", err
	}
	if !repoShape.MatchString(g.Repo) || path.Base(g.Repo) == ".." || path.Base(g.Repo) == "." {
		return "", fmt.Errorf("%q is not a repository's owner/name", g.Repo)
	}
	return path.Join(tokens, g.ClientID, g.Repo+".json"), nil
}

// CachedBot returns the App at work kept under dir, the user's cache
// directory, for g, if its token lasts ten more minutes at now.
func CachedBot(dir string, g Grant, now time.Time) (Bot, bool) {
	name, err := g.file()
	if err != nil {
		return Bot{}, false
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Bot{}, false
	}
	data, err := root.ReadFile(name)
	if errors.Join(err, root.Close()) != nil {
		return Bot{}, false
	}
	var b Bot
	if json.Unmarshal(data, &b) != nil || b.Token.Value == "" || b.Identity.Login == "" || !now.Add(reuse).Before(b.Token.ExpiresAt) {
		return Bot{}, false
	}
	return b, true
}

// SaveBot keeps b for g under dir, the user's cache directory, readable by
// the user alone.
func SaveBot(dir string, g Grant, b Bot) (err error) {
	name, err := g.file()
	if err != nil {
		return err
	}
	data, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("keep the token: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("keep the token: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
		return fmt.Errorf("keep the token: %w", err)
	}
	if err := root.WriteFile(name, data, 0o600); err != nil {
		return fmt.Errorf("keep the token: %w", err)
	}
	return nil
}
