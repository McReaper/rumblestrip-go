package ghapp_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
)

func writeRepoApp(t *testing.T, dir string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".rumblestrip"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(ghapp.RepoFile)), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestARepositoryNamesTheAppItsAgentActsAs(t *testing.T) {
	dir := t.TempDir()
	if _, err := ghapp.RepoApp(dir); !errors.Is(err, ghapp.ErrNoRepoApp) {
		t.Fatalf("no App named: %v, want ErrNoRepoApp", err)
	}
	data, err := ghapp.RepoAppFile("Iv23abc")
	if err != nil {
		t.Fatal(err)
	}
	writeRepoApp(t, dir, data)
	if got, err := ghapp.RepoApp(dir); err != nil || got != "Iv23abc" {
		t.Errorf("RepoApp = %q, %v, want Iv23abc", got, err)
	}
}

func TestAClientIDThatNamesAnotherFileIsRefused(t *testing.T) {
	for _, id := range []string{"", "../key", "a/b", ".hidden", "Iv1..x"} {
		if _, err := ghapp.RepoAppFile(id); err == nil {
			t.Errorf("RepoAppFile(%q): no error", id)
		}
		if _, err := ghapp.SaveKey(t.TempDir(), id, []byte("k")); err == nil {
			t.Errorf("SaveKey(%q): no error", id)
		}
	}
	dir := t.TempDir()
	writeRepoApp(t, dir, []byte(`{"client_id": "../../key"}`))
	if _, err := ghapp.RepoApp(dir); err == nil || errors.Is(err, ghapp.ErrNoRepoApp) {
		t.Errorf("a repository naming ../../key: %v, want an error that says it is no Client ID", err)
	}
}

func TestTheMachineKeepsEachAppsKeyUnderItsClientID(t *testing.T) {
	dir := t.TempDir()
	if ids, err := ghapp.MachineApps(dir); err != nil || len(ids) != 0 {
		t.Fatalf("no key saved: %v, %v, want none", ids, err)
	}
	for _, id := range []string{"Iv2b", "Iv1.a"} {
		p, err := ghapp.SaveKey(dir, id, []byte("key of "+id))
		if err != nil || p != ghapp.MachineKey(dir, id) {
			t.Fatalf("SaveKey(%s) = %q, %v, want %q", id, p, err, ghapp.MachineKey(dir, id))
		}
		got, err := ghapp.KeyFile(p)
		if err != nil || string(got) != "key of "+id {
			t.Errorf("the key of %s reads %q, %v", id, got, err)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
			t.Errorf("the key of %s is %v, want readable by its user alone", id, info.Mode().Perm())
		}
	}
	if ids, err := ghapp.MachineApps(dir); err != nil || !slices.Equal(ids, []string{"Iv1.a", "Iv2b"}) {
		t.Errorf("MachineApps = %v, %v, want [Iv1.a Iv2b]", ids, err)
	}
}
