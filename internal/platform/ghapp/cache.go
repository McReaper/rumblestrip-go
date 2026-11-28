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

// WHY: the agent's gh asks for a token before each of its commands, and a
// token lasts an hour: the machine keeps it until ten minutes are left, so
// a command pays a file's read, not two calls to GitHub.
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

// CachedToken returns the token kept under dir, the user's cache
// directory, for g, if it lasts ten more minutes at now.
func CachedToken(dir string, g Grant, now time.Time) (Token, bool) {
	name, err := g.file()
	if err != nil {
		return Token{}, false
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Token{}, false
	}
	data, err := root.ReadFile(name)
	if errors.Join(err, root.Close()) != nil {
		return Token{}, false
	}
	var tok Token
	if json.Unmarshal(data, &tok) != nil || tok.Value == "" || !now.Add(reuse).Before(tok.ExpiresAt) {
		return Token{}, false
	}
	return tok, true
}

// SaveToken keeps tok for g under dir, the user's cache directory,
// readable by the user alone.
func SaveToken(dir string, g Grant, tok Token) (err error) {
	name, err := g.file()
	if err != nil {
		return err
	}
	data, err := json.Marshal(tok)
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
