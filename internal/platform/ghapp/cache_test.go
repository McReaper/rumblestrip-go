package ghapp_test

import (
	"testing"
	"time"

	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
)

func TestTheMachineKeepsATokenUntilTenMinutesAreLeft(t *testing.T) {
	dir := t.TempDir()
	minted := time.Date(2026, 11, 28, 15, 0, 0, 0, time.UTC)
	g := ghapp.Grant{ClientID: "Iv23abc", Repo: "octo/notes"}
	if _, ok := ghapp.CachedToken(dir, g, minted); ok {
		t.Fatal("a token before any was kept")
	}
	if err := ghapp.SaveToken(dir, g, ghapp.Token{Value: "ghs_one", ExpiresAt: minted.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		g    ghapp.Grant
		at   time.Duration
		want string
	}{
		{g, 49 * time.Minute, "ghs_one"},
		{g, 50 * time.Minute, ""},
		{ghapp.Grant{ClientID: "Iv23abc", Repo: "octo/other"}, 0, ""},
		{ghapp.Grant{ClientID: "Iv23xyz", Repo: "octo/notes"}, 0, ""},
	} {
		tok, ok := ghapp.CachedToken(dir, c.g, minted.Add(c.at))
		if ok != (c.want != "") || tok.Value != c.want {
			t.Errorf("CachedToken(%+v) %v after minting = %q, %v, want %q", c.g, c.at, tok.Value, ok, c.want)
		}
	}
}

func TestAGrantThatNamesAnotherFileIsRefused(t *testing.T) {
	tok := ghapp.Token{Value: "ghs_one", ExpiresAt: time.Now().Add(time.Hour)}
	for _, g := range []ghapp.Grant{
		{ClientID: "../x", Repo: "octo/notes"},
		{ClientID: "Iv23abc", Repo: "../notes"},
		{ClientID: "Iv23abc", Repo: "octo/.."},
		{ClientID: "Iv23abc", Repo: "octo/notes/x"},
		{ClientID: "Iv23abc", Repo: "notes"},
	} {
		if err := ghapp.SaveToken(t.TempDir(), g, tok); err == nil {
			t.Errorf("SaveToken(%+v) kept it", g)
		}
	}
}
