# CLAUDE.md

Rumblestrip keeps AI-written Go on its rails. Read `docs/rules.md` before changing a rule, and `docs/adr/` before reopening a decision.

## Layout

- `cmd/rumblestrip`: the CLI.
- `cmd/rumblestrip-vet`: the analyzers' multichecker.
- `internal/<command>/`: each command's logic, in the package named after it. The exceptions are `pretool` (`hook pre-tool`), `staledocs` (`docs`) and `initrepo` (`init`).
- `internal/analyzers/`: Rumblestrip's analyzers. `layout` says which part of an app a package is.
- `internal/lint/`: the golangci-lint and gitleaks configs apps run. An app never uses its own.
- `internal/initrepo/files/`: the files `rumblestrip init` writes into an app.
- `internal/platform/`: git, GitHub and the tools, each started in one place.
- `.github/workflows/rumblestrip-*.yml`: the checks an app's CI calls. Their header comments say how they run.
- `plugin/`: the Claude Code plugin, with its skills, rules and hooks. `plugin/bin/rumblestrip` runs the checks in the image the app pins, and `pr`, `app` and `init` on the host. Its header says how.
- `template/`: the app template, its own module. Every proof starts from it, and it passes every gate.
- `proof/`: runs every tool on the template and on each case. It also checks `docs/rules.md` against the cases and `tools.txt`.
- `proof/cases/<case>/`: one violation, and the findings it must produce, in `want.txt` (`proof/doc.go` gives the format).
- `tools.txt` and `Dockerfile`: every tool's pinned version, and the image that carries them. A push to main publishes the image as `sha-<commit>`.

## Rules

- Rumblestrip is for one owner who reads the spec, never the code (ADR 9). When a check misses something, add a check, never more for the owner to read.
- A rule enters `docs/rules.md` only once a case proves it, with the exact finding.
- Mark a case `!isolated` if it replaces a file others build on, like go.mod.
- Never run go-mutesting on `template`: it rewrites the source while tests run.
- Rumblestrip passes its own rules: `golangci-lint run --config internal/lint/golangci.yml ./...` and rumblestrip-vet, for GOOS linux and windows.
- `go get x@latest` can move the `go` line: check go.mod after it, and follow ADR 1.
- Everything in English. Checks are Go, not scripts in another language. Conventional commits.
- This repository's ruleset is `.github/ruleset.json`. Apply it with `gh api repos/McReaper/rumblestrip-go/rulesets/26266513 --method PUT --input .github/ruleset.json`. It requires the CI job `rumblestrip`, so renaming one means renaming both.

## Writing

- Short sentences, one idea each. No chains of colons or semicolons.
- The owner reads `README.md`, `docs/guide.md` and `docs/setup.md`. Use plain words and no criterion IDs there.
- `TestTheHumanDocsStayShort` sets each page's word budget. Before adding words to a page, cut some.
- A doc states the rule and why. How it was found or measured goes in the commit.

## Commands

- Everything, as CI runs it: `docker build -t rumblestrip .`, then `docker run --rm -v "$PWD:/work" rumblestrip go test ./...`.
- Outside the image, `go test ./...` needs the tools in `tools.txt` on the PATH. Set `ZIZMOR=<path>` if pip put zizmor elsewhere. The race proof runs on Linux only.
- One proof: `go test ./proof/ -run TestGuardReportsEveryCase`.
