# Setup in detail

What each step of the README's setup sets, and why the agent can't undo it. You run these; the agent can't.

## `rumblestrip init`: the files the repository keeps

- `.github/workflows/rumblestrip.yml` and `rumblestrip-approval.yml`: they call Rumblestrip's checks at the version they pin. Dependabot moves that pin to each new release.
- `.github/CODEOWNERS`: you own the agent's instructions and the CI, so GitHub waits for your approval on them.
- `.github/dependabot.yml`: the pin, and the Go dependencies, kept up to date.
- `.claude/settings.json`: turns Rumblestrip's plugin on for Claude Code, refuses the gestures that reach past it (merging, a force push to `main`, skipping hooks, editing `.github/`), and makes the agent's commits name you as co-author.

`rumblestrip check` fails once the two workflows differ from what the version they pin writes: delete them and run `rumblestrip init` again.

## `rumblestrip init app`: the agent's identity

A GitHub App the agent acts as, created from Rumblestrip's manifest in your browser. Its private key goes to your configuration directory, never into a repository. Install it on the repositories it works on.

It may write code and pull requests. It may not touch the workflows or the repository's settings, so it can't change the rules that judge it, even through a pull request. In Claude Code, the agent's `gh` acts as it, so you can approve what it opens. An App of yours will do: set `RUMBLESTRIP_APP_CLIENT_ID` and `RUMBLESTRIP_APP_KEY`.

## `rumblestrip init github`: the rules GitHub enforces

With the App's `RUMBLESTRIP_APP_CLIENT_ID` and `RUMBLESTRIP_APP_KEY`, it checks the App is installed on the repository before it changes anything. Then:

- **The default branch** takes changes only through a pull request, squash-merged. It needs two checks, both from GitHub Actions: `Pull request / Check`, every check passed, and `Rumblestrip / Owner approval`, your approval of what is yours.
- **Tags and releases**: only you create or move a tag, and a published release can't change.
- **The App's secrets**: its Client ID and key, so Rumblestrip merges as the App. A merge by the workflow's own token would leave a stack's next pull request behind.
- Merged branches are deleted, and a pull request behind its base shows GitHub's Update branch button.

## The agent's machine

The plugin runs the agent's checks in Rumblestrip's image, through Docker: every tool they need, at the versions CI runs. Run `/rumblestrip:setup` in Claude Code once, and again when the pin moves: it pulls the image, and installs `rumblestrip` for `pr`, `app` and `init`, which run on the machine with your credentials, through Go. Until then, the hooks refuse every command and edit. The image runs on x86-64 only, and not in a git submodule.

## Harder to undo: managed settings

The agent could edit `.claude/settings.json`. To stop that, copy [`managed-settings.json`](managed-settings.json) to the machine's managed settings: `C:\Program Files\ClaudeCode\managed-settings.json` on Windows, `/Library/Application Support/ClaudeCode/` on macOS, `/etc/claude-code/` on Linux. No other settings override it.

Settings and hooks only slow the agent down; GitHub's rules are what hold.
