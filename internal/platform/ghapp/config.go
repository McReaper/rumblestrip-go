package ghapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// RepoFile names, in a repository, the App the agent acts as there, by its
// Client ID. The owner commits it: it holds no secret.
const RepoFile = ".rumblestrip/app.json"

// ErrNoRepoApp is returned when a repository names no App.
var ErrNoRepoApp = errors.New("the repository names no App")

// WHY: each key is named by its App's Client ID, which is all a repository
// names: the machine finds it with nothing else.
const keys = "rumblestrip-go/apps"

var clientIDShape = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.]*$`)

type repoApp struct {
	ClientID string `json:"client_id"`
}

// RepoAppFile is the content of RepoFile naming the App clientID.
func RepoAppFile(clientID string) ([]byte, error) {
	if err := checkClientID(clientID); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(repoApp{clientID}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("name the App: %w", err)
	}
	return append(data, '\n'), nil
}

// RepoApp returns the Client ID of the App the repository at dir names:
// ErrNoRepoApp when it names none.
func RepoApp(dir string) (id string, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("read the repository's App: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	data, err := root.ReadFile(RepoFile)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoRepoApp
	}
	if err != nil {
		return "", fmt.Errorf("read the repository's App: %w", err)
	}
	var a repoApp
	if err := json.Unmarshal(data, &a); err != nil {
		return "", fmt.Errorf("read the repository's App, %s: %w", RepoFile, err)
	}
	if err := checkClientID(a.ClientID); err != nil {
		return "", fmt.Errorf("read the repository's App, %s: %w", RepoFile, err)
	}
	return a.ClientID, nil
}

// MachineKey is the path of the private key of the App clientID on this
// machine, under dir, the user's configuration directory.
func MachineKey(dir, clientID string) string {
	return filepath.Join(dir, filepath.FromSlash(keys), clientID+".pem")
}

// SaveKey writes the App's private key where MachineKey says, readable by
// the user alone, and returns its path.
func SaveKey(dir, clientID string, pem []byte) (p string, err error) {
	if err := checkClientID(clientID); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("save the App's key: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := root.MkdirAll(keys, 0o700); err != nil {
		return "", fmt.Errorf("save the App's key: %w", err)
	}
	if err := root.WriteFile(path.Join(keys, clientID+".pem"), pem, 0o600); err != nil {
		return "", fmt.Errorf("save the App's key: %w", err)
	}
	return MachineKey(dir, clientID), nil
}

// MachineApps returns the Client IDs of the Apps whose key this machine
// keeps under dir, the user's configuration directory, sorted.
func MachineApps(dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(keys)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list the Apps' keys: %w", err)
	}
	var ids []string
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), ".pem"); ok && !e.IsDir() && checkClientID(id) == nil {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// WHY: a Client ID names a file on the machine; one that is anything else
// could name another.
func checkClientID(id string) error {
	if !clientIDShape.MatchString(id) || strings.Contains(id, "..") {
		return fmt.Errorf("%q is not an App's Client ID", id)
	}
	return nil
}
