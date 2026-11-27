package ghapp_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
)

func TestTheSavedAppIsTheOneLoaded(t *testing.T) {
	dir := t.TempDir()
	if _, err := ghapp.LoadConfig(dir); !errors.Is(err, ghapp.ErrNoConfig) {
		t.Fatalf("no App saved: %v, want ErrNoConfig", err)
	}
	want := ghapp.Config{ClientID: "Iv1", Key: filepath.Join(dir, "notes-agent.private-key.pem")}
	if err := ghapp.SaveConfig(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ghapp.LoadConfig(dir)
	if err != nil || got != want {
		t.Fatalf("loaded %+v, %v, want %+v", got, err, want)
	}
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(ghapp.ConfigFile)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("the App's file is %v, want readable by its user alone", info.Mode().Perm())
	}
}

func TestAnAppWithNoKeyIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := ghapp.SaveConfig(dir, ghapp.Config{ClientID: "Iv1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ghapp.LoadConfig(dir); err == nil || errors.Is(err, ghapp.ErrNoConfig) {
		t.Errorf("an App with no key: %v, want an error that says so", err)
	}
}
