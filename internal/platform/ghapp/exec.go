package ghapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// Identity is the app's bot user, who commits and pushes.
type Identity struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

// Email is the bot's no-reply address, which GitHub ties to its account.
func (i Identity) Email() string {
	return strconv.FormatInt(i.ID, 10) + "+" + i.Login + "@users.noreply.github.com"
}

// Env returns base with the token for gh and git, and the bot as author
// and committer.
//
// WHY: git reads the credential through a helper that echoes $GH_TOKEN,
// so the token sits in the environment only, never on a command line or in
// a git config file. The helper replaces the user's for github.com alone,
// so git pushes there as the App whatever the user set up, and no other
// host hears the token. The bot's commits can't carry the user's signature.
func Env(base []string, tok Token, id Identity) []string {
	helper := `!f() { echo username=x-access-token; echo "password=$GH_TOKEN"; }; f`
	github := "credential.https://github.com.helper"
	return append(base,
		"GH_TOKEN="+tok.Value,
		"GITHUB_TOKEN="+tok.Value,
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0="+github, "GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1="+github, "GIT_CONFIG_VALUE_1="+helper,
		"GIT_CONFIG_KEY_2=commit.gpgsign", "GIT_CONFIG_VALUE_2=false",
		"GIT_AUTHOR_NAME="+id.Login, "GIT_AUTHOR_EMAIL="+id.Email(),
		"GIT_COMMITTER_NAME="+id.Login, "GIT_COMMITTER_EMAIL="+id.Email(),
	)
}

// Bot is the app at work: its installation token and its bot user.
type Bot struct {
	Token    Token    `json:"token"`
	Identity Identity `json:"identity"`
}

// Exec runs argv as the app, its standard streams the caller's.
func Exec(ctx context.Context, argv []string, b Bot) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = Env(os.Environ(), b.Token, b.Identity)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", argv[0], err)
	}
	return nil
}
