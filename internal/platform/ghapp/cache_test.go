package ghapp_test

import (
	"testing"
	"time"

	"github.com/McReaper/rumblestrip-go/internal/platform/ghapp"
)

func TestTheMachineKeepsTheBotUntilTenMinutesOfItsTokenAreLeft(t *testing.T) {
	dir := t.TempDir()
	minted := time.Date(2026, 11, 28, 15, 0, 0, 0, time.UTC)
	g := ghapp.Grant{ClientID: "Iv23abc", Repo: "octo/notes"}
	if _, ok := ghapp.CachedBot(dir, g, minted); ok {
		t.Fatal("a bot before any was kept")
	}
	bot := ghapp.Bot{Token: ghapp.Token{Value: "ghs_one", ExpiresAt: minted.Add(time.Hour)}, Identity: ghapp.Identity{Login: "app[bot]", ID: 7}}
	if err := ghapp.SaveBot(dir, g, bot); err != nil {
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
		got, ok := ghapp.CachedBot(dir, c.g, minted.Add(c.at))
		if ok != (c.want != "") || got.Token.Value != c.want {
			t.Errorf("CachedBot(%+v) %v after minting = %q, %v, want %q", c.g, c.at, got.Token.Value, ok, c.want)
		}
		if ok && got.Identity != bot.Identity {
			t.Errorf("CachedBot(%+v) acts as %+v, want %+v", c.g, got.Identity, bot.Identity)
		}
	}
}

func TestAKeptTokenWithNoBotIsMintedAgain(t *testing.T) {
	dir := t.TempDir()
	g := ghapp.Grant{ClientID: "Iv23abc", Repo: "octo/notes"}
	if err := ghapp.SaveBot(dir, g, ghapp.Bot{Token: ghapp.Token{Value: "ghs_one", ExpiresAt: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ghapp.CachedBot(dir, g, time.Now()); ok {
		t.Error("a token kept with no bot to act as was reused")
	}
}

func TestAGrantThatNamesAnotherFileIsRefused(t *testing.T) {
	bot := ghapp.Bot{Token: ghapp.Token{Value: "ghs_one", ExpiresAt: time.Now().Add(time.Hour)}, Identity: ghapp.Identity{Login: "app[bot]", ID: 7}}
	for _, g := range []ghapp.Grant{
		{ClientID: "../x", Repo: "octo/notes"},
		{ClientID: "Iv23abc", Repo: "../notes"},
		{ClientID: "Iv23abc", Repo: "octo/.."},
		{ClientID: "Iv23abc", Repo: "octo/notes/x"},
		{ClientID: "Iv23abc", Repo: "notes"},
	} {
		if err := ghapp.SaveBot(t.TempDir(), g, bot); err == nil {
			t.Errorf("SaveBot(%+v) kept it", g)
		}
	}
}
