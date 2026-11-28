// Package ghapp authenticates as the agent's GitHub App: a JWT signed with
// the app's private key, exchanged for an installation token that opens one
// repository and expires within the hour. The key is read only to sign; the
// token lives in the environment of the commands that act as the App, and
// in the user's cache, readable by the user alone.
package ghapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"
)

// ErrNoKey is returned when the key file holds no PEM block.
var ErrNoKey = errors.New("no PEM key in the file")

// Client talks to the GitHub API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a client of the public GitHub API with a timeout.
func NewClient() Client {
	return Client{BaseURL: "https://api.github.com", HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Installation is one account the app is installed on, and what it may
// do there: each permission's name and its level, read or write.
type Installation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
}

// Token is an installation token. It prints and logs as [redacted].
type Token struct {
	Value     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// String hides the token's value.
func (Token) String() string { return "[redacted]" }

// LogValue hides the token's value from slog.
func (Token) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// KeyFile reads the app's private key file as it is.
func KeyFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("open the key's folder: %w", err)
	}
	data, rerr := root.ReadFile(filepath.Base(path))
	if err := errors.Join(rerr, root.Close()); err != nil {
		return nil, fmt.Errorf("read the key: %w", err)
	}
	return data, nil
}

// LoadKey reads the app's private key, PKCS#1 or PKCS#8.
func LoadKey(path string) (*rsa.PrivateKey, error) {
	data, err := KeyFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrNoKey
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse the key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("the key is %T, not RSA", parsed)
	}
	return key, nil
}

// JWT returns the app's JSON Web Token, valid nine minutes from now.
// issuer is the app's Client ID (or App ID).
func JWT(issuer string, key *rsa.PrivateKey, now time.Time) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(struct {
		IssuedAt  int64  `json:"iat"`
		ExpiresAt int64  `json:"exp"`
		Issuer    string `json:"iss"`
	}{now.Add(-time.Minute).Unix(), now.Add(9 * time.Minute).Unix(), issuer})
	if err != nil {
		return "", fmt.Errorf("encode the JWT claims: %w", err)
	}
	signed := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign the JWT: %w", err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Installations lists where the app is installed.
func (c Client) Installations(ctx context.Context, jwt string) ([]Installation, error) {
	var out []Installation
	return out, c.do(ctx, call{http.MethodGet, "/app/installations", jwt, nil}, &out)
}

// RepoInstallation returns the app's installation that holds repo
// ("owner/name").
func (c Client) RepoInstallation(ctx context.Context, jwt, repo string) (Installation, error) {
	var out Installation
	return out, c.do(ctx, call{http.MethodGet, "/repos/" + repo + "/installation", jwt, nil}, &out)
}

// Slug returns the app's slug, which names its bot user: <slug>[bot].
func (c Client) Slug(ctx context.Context, jwt string) (string, error) {
	var app struct {
		Slug string `json:"slug"`
	}
	return app.Slug, c.do(ctx, call{http.MethodGet, "/app", jwt, nil}, &app)
}

// Scope is what a token opens: the repository Repo, owner/name, through
// the App's Installation that holds it, or 0 for Mint to look it up.
type Scope struct {
	Installation int64
	Repo         string
}

// Token mints an installation token that opens s.Repo alone.
//
// WHY: a token opens every repository of its installation unless it names
// one, and the agent's lands in its transcripts: read there, it opens the
// agent's repository and no other of the owner's.
func (c Client) Token(ctx context.Context, jwt string, s Scope) (Token, error) {
	var tok Token
	body := struct {
		Repositories []string `json:"repositories"`
	}{[]string{path.Base(s.Repo)}}
	return tok, c.do(ctx, call{http.MethodPost, "/app/installations/" + strconv.FormatInt(s.Installation, 10) + "/access_tokens", jwt, body}, &tok)
}

// Mint returns the App at work on s.Repo: a token that opens it alone, and
// the App's bot user, who commits and pushes.
func (c Client) Mint(ctx context.Context, jwt string, s Scope) (Bot, error) {
	if s.Installation == 0 {
		inst, err := c.RepoInstallation(ctx, jwt, s.Repo)
		if err != nil {
			return Bot{}, fmt.Errorf("the App isn't installed on %s, or can't be reached: install it there: %w", s.Repo, err)
		}
		s.Installation = inst.ID
	}
	tok, err := c.Token(ctx, jwt, s)
	if err != nil {
		return Bot{}, err
	}
	slug, err := c.Slug(ctx, jwt)
	if err != nil {
		return Bot{}, err
	}
	id := Identity{Login: slug + "[bot]"}
	if id.ID, err = c.UserID(ctx, id.Login); err != nil {
		return Bot{}, err
	}
	return Bot{Token: tok, Identity: id}, nil
}

// UserID returns the id of a user, the app's bot user included.
func (c Client) UserID(ctx context.Context, login string) (int64, error) {
	var u struct {
		ID int64 `json:"id"`
	}
	return u.ID, c.do(ctx, call{http.MethodGet, "/users/" + login, "", nil}, &u)
}

type call struct {
	method string
	path   string
	bearer string
	body   any
}

func (k call) request(ctx context.Context, base string) (*http.Request, error) {
	if k.body == nil {
		return http.NewRequestWithContext(ctx, k.method, base+k.path, http.NoBody)
	}
	data, err := json.Marshal(k.body)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, k.method, base+k.path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c Client) do(ctx context.Context, k call, out any) error {
	req, err := k.request(ctx, c.BaseURL)
	if err != nil {
		return fmt.Errorf("%s %s: %w", k.method, k.path, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if k.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+k.bearer)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", k.method, k.path, err)
	}
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := errors.Join(rerr, resp.Body.Close()); err != nil {
		return fmt.Errorf("%s %s: %w", k.method, k.path, err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %s: %s", k.method, k.path, resp.Status, bytes.TrimSpace(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s %s: decode: %w", k.method, k.path, err)
	}
	return nil
}
