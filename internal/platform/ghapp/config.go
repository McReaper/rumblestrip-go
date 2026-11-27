package ghapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
)

// Config is the App the agent acts as on this machine: its Client ID and
// the path of its private key. rumblestrip init app writes it beside the
// key, so the plugin's gh finds the App without being told.
type Config struct {
	ClientID string `json:"client_id"`
	Key      string `json:"key"`
}

// ConfigFile is where Config lives, under the user's configuration
// directory.
const ConfigFile = "rumblestrip-go/app.json"

// ErrNoConfig is returned when this machine has no App set up.
var ErrNoConfig = errors.New("no App is set up on this machine")

// SaveConfig writes c under dir, the user's configuration directory,
// readable by them alone.
func SaveConfig(dir string, c Config) (err error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("save the App: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("save the App: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := root.MkdirAll(path.Dir(ConfigFile), 0o700); err != nil {
		return fmt.Errorf("save the App: %w", err)
	}
	if err := root.WriteFile(ConfigFile, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("save the App: %w", err)
	}
	return nil
}

// LoadConfig reads the App saved under dir, the user's configuration
// directory: ErrNoConfig when there is none.
func LoadConfig(dir string) (c Config, err error) {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, ErrNoConfig
	}
	if err != nil {
		return Config{}, fmt.Errorf("read the App: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	data, err := root.ReadFile(ConfigFile)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, ErrNoConfig
	}
	if err != nil {
		return Config{}, fmt.Errorf("read the App: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("read the App, %s: %w", ConfigFile, err)
	}
	if c.ClientID == "" || c.Key == "" {
		return Config{}, fmt.Errorf("read the App, %s: it names no Client ID or no key", ConfigFile)
	}
	return c, nil
}
