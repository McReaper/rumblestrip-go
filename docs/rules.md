# Rumblestrip's rules

Rumblestrip holds AI-written Go to the criteria below. Each one lists what drifts when an agent writes the code, what checks it, and the proof that the check fires. A proof is either a case or a test under `proof/`. A case is a folder under `proof/cases/` with one violation and the exact findings it must produce. `go test ./...` runs every proof, so a tool upgrade that drops a rule fails.

There are three levels:

- **Blocking:** a failure stops CI.
- **Partial:** CI checks part of the criterion, and the rest is guided.
- **Guided:** the template or a skill shows the way, and nothing fails.

Out of target are S4 (network authentication), S9 (infrastructure) and the HTTP part of S7: Rumblestrip is for CLIs, MCP servers and libraries.

## Why an agent drifts

- **C1. Volume outruns review.** Nobody checks what no machine checks.
- **C2. The agent optimises for green.** It bypasses any rule it can change.
- **C3. No memory between sessions.** Conventions and decisions must live in the repository.
- **C4. Limited context.** The agent reinvents what it doesn't find.
- **C5. Slow drift.** Each change looks fine, but together they degrade the code.

## The layout

```
cmd/<app>/                         wiring only
internal/<m>/                      a module's contract: the only package other modules import
internal/<m>/internal/domain/      pure: no I/O, no clock, no log
internal/<m>/internal/<feature>/   a vertical slice: Run(ctx, Args) (Out, error), its tool, its command
internal/<m>/internal/infra/       I/O adapters
internal/platform/                 the shared kernel, which knows no module
pkg/<lib>/                         libraries
```

Go's nested `internal/` makes importing another module's inside a compile error. Each operation has one `Args` struct. Its `json` tags are the MCP arguments, and the CLI flags derive from them (`template/internal/platform/op`).

## Where the rules live

Rumblestrip ships every rule, pinned by version, and an app never carries its own copy:

- `internal/lint/golangci.yml`, the golangci-lint config;
- `cmd/rumblestrip-vet`, Rumblestrip's own analyzers;
- `internal/lint/gitleaks.toml`, the secret rules;
- `rumblestrip guard` and archtest;
- `tools.txt`, which pins every tool's version. Rumblestrip's image carries exactly those versions.

`rumblestrip check` runs every gate. It runs the same way on the agent's machine and in CI, where each gate is a job of its own. Proof: `TestCheckRunsEveryGateAndFailsOnAny`, `TestCIRunsEachGateOfCheckInAJobOfItsOwn`.

## T2: the agent can't change what judges it (blocking)

Drift (C2): the agent bends tests, thresholds and suppressions to get green.

- **GitHub is the arbiter.**
  - The agent is a GitHub App with access to contents and pull requests only, never to workflows or administration. `rumblestrip init app` creates it, and `rumblestrip app exec` gives its token to one command.
  - Proof: `internal/platform/ghapp` tests. In a live test, GitHub refused the App a workflow push, a ruleset edit and a settings change.
- **An app's ruleset requires two checks, which only GitHub Actions can report:** `Pull request / Check` and `Rumblestrip / Owner approval`.
  - `rumblestrip init github` applies it from `internal/initrepo/files/ruleset.json`. Code owners review the harness only. Once both checks pass, Rumblestrip's merge job merges the change as the App.
  - Proof: `TestTheAppsRulesetMergesAGreenChangeAndHoldsWhatIsTheOwners`, `TestTheRequiredCheckNeverSkips`, `TestTheMergeJobMergesAsTheRepositorysAppOrSaysHowToGiveItOne`, `TestOnlyTheReviewSheetsJobPostsTheOwnersApproval`, `TestAPullRequestOnAnotherBranchIsInAStackOrTheCheckSaysHow`.
- **Only the owner makes tags and releases:** `ruleset-tags.json` restricts tags, and releases are immutable.
  - Proof: `TestTheTagRulesetLetsOnlyTheOwnerTagARelease`.
- **What's the owner's waits for them, and the rest merges by itself.**
  - `rumblestrip guard` reports changes to:
    - the spec;
    - the harness;
    - a new dependency;
    - the `go` line;
    - a library's API, through apidiff;
    - the size.
  - The owner's approval holds the approved content of those files, through later commits and rebases.
  - A change to the code's own tests is free, because mutation and the spec's proof judge it.
  - Proof: `guard-spec`, `guard-protected`, `guard-gomod`, `TestTheMergeCheckKeepsAnApprovalWhileTheProtectedFilesStayAsApproved`, `TestAnApprovalReRunsTheReviewSheetOfTheApprovedCommitsRun`.
- **No bypass in the diff.**
  - `rumblestrip guard` fails on:
    - the comments `//nolint`, `#nosec` and `//lint:ignore`;
    - the directives `//go:linkname` and `//go:debug`, and `import "C"`;
    - an unknown build tag;
    - a `_` or `.` directory;
    - a generated-code header outside `gen/`;
    - a `TestMain` or an `init` in a test.
  - forbidigo refuses `t.Skip` and `testing.Short`.
  - The guard reads the working tree, not only the commits.
  - Proof: `guard-bypass`, `guard-clean`, `guard-uncommitted`, `t2-skipped-tests`, `TestGuardHoldsProtectedChangesAtMergeAndBypassesEverywhere`.
- **Nothing reaches past the type system.**
  - depguard denies `unsafe` and `plugin` everywhere, and `reflect` in the domain.
  - gomoddirectives forbids a local `replace` and a `godebug` line.
  - bidichk, asciicheck and gocheckcompilerdirectives stop Trojan Source, look-alike names and directives disabled by a space.
  - Proof: `t2-directives`, `t2-gomod`, `s-security`.
- **Lint runs for every target system:** `GOOS=linux` and `GOOS=windows`.
  - Proof: `m11-robustness` (its `clean_windows.go`).
- **The CI an app keeps stays Rumblestrip's.**
  - The rumblestrip-files gate fails when an app's workflows aren't exactly what `rumblestrip init` writes for its pin.
  - Proof: `TestCheckFailsOnAWorkflowItsRumblestripVersionWouldNotWrite`, `TestTheCIInitWritesPassesTheWorkflowGates`.
- **The plugin guards the agent's machine.**
  - The `pre-tool` hook refuses:
    - merging, and pushing to the default branch;
    - a force push without a lease;
    - skipping a commit's hooks;
    - changing a ruleset;
    - editing `.github/`, `.rumblestrip/` or `.claude/`;
    - a commit with a staged secret.
  - `post-edit` lints each package the agent edits, and `stop` runs the full check once before the agent stops.
  - Until `rumblestrip setup` has pulled the image, the hooks refuse every command and edit but setup.
  - `.claude/settings.json` holds the deny rules, and `docs/managed-settings.json` puts them where the agent can't undo them.
  - All of this only slows the agent down: GitHub's rules are what hold.
  - Proof: `TestThePreToolHookRefusesAGestureAndACommitWithAStagedSecret`, `TestThePostEditHookTellsTheAgentWhatItsEditBroke`, `TestTheStopHookLetsTheAgentStopOnItsSecondTry`, `TestTheAgentSettingsDenyBypassesAndThePluginRunsRumblestripsHooks`, `TestThePluginShipsWhatItsManifestsAndHooksName`.

Left to review: the rare change to a rule. A pull request runs its own workflows, so a weakened CI is stopped by CODEOWNERS on `.github/`.

## T3: review stays small (blocking)

Drift (C1): the owner can't read what the agent writes.

- **The owner reads the spec and a short sheet, not the diff.**
  - `rumblestrip review` posts the review sheet. It holds a status line, the spec's scenarios added, changed or removed, and one line for each other change that is the owner's.
  - Proof: `t2-review-sheet`, `TestReviewWritesTheSheetForThePullRequest`, `TestReviewSaysWhereTheOwnersApprovalStands`, `TestReviewListsWhatTheOwnerMustRead`.
- **A run says what failed without its log.**
  - Check's summary lists each failing gate, what it found and whose it is to fix. Each finding is an annotation on its line.
  - Proof: `TestCheckAnnotatesEachFindingsLineAndWritesTheOwnersSummary`, `TestCheckSaysWhichTestFailedWhereAndWhy`, `TestTheCheckJobNamesTheGatesThatFailedAndSaysWhatComesNext`.
- **Changes stay small.**
  - A pull request changes at most 400 production lines, in one module. Its title is a conventional commit of 72 characters at most.
  - Proof: `guard-size`, `TestTheTitleGateWantsAShortConventionalCommit`.

## T1: no drift (blocking)

Drift (C5): many small concessions add up.

Every gate wants zero findings on every change, the template included. Proof: every case.

## M6: tests fail when the code breaks (blocking)

Drift: hollow tests, written to pass or changed to pass.

- **Mutation runs on changed lines.**
  - `rumblestrip mutate` runs go-mutesting on a copy of the module. A mutant counts when it starts on a line the change adds, and the gate fails under a score of 0.8.
  - The threshold comes from the template, whose tests kill 76 of its 81 mutants. The 5 that live change nothing a test could see.
  - Mutants that only drop an `if err != nil` return are listed apart, because the compiler and errcheck already refuse a dropped error (ADR 4).
  - Proof: `m6-weak-test`, `TestMutateFailsAChangeItsTestsDontPin`, `TestMutationReportsEveryLivedMutant`.
- **Each step of the spec can fail.**
  - The spec is Gherkin under `features/`, run by godog on the built program.
  - `rumblestrip spec` runs each scenario alone. Then it runs it again once for each value its `Then` steps expect, with that value altered, and that run must fail.
  - Proof: `spec-proof`, `spec-untested`, `TestSpecReportsEveryCase`.
- **The spec runs what a change adds.**
  - `rumblestrip spec` builds the program with coverage for the scenarios as written. A line the change adds to a slice or the domain that no scenario runs is behaviour the owner never approved, and fails.
  - An `if err != nil { return … }` is left out, as mutation leaves it out (ADR 4). The infra, the wiring and the platform stay the unit tests'.
  - Proof: `spec-unreached`.
- **The program people run is tested.**
  - The template's smoke test calls every MCP tool and CLI command of the built program with its declared example.
  - Proof: `m6-broken-program`, `TestTheTemplatesTestsReportEveryCase`.
- **Every test asserts something:** `rumblestrip-vet testassert`.
  - Proof: `e6-tests`.
- **No mocks.** depguard denies gomock and testify, so a test's fake implements the interface.
  - Proof: `m3-consistency`.
- **Guided:**
  - property tests with `rapid`, and native fuzzing;
  - golden files;
  - the `new-feature` skill: the spec goes first, in its own pull request, then the build, with its unit tests first.

There is no coverage gate, because mutation already counts uncovered code.

## M1: local simplicity (blocking)

Drift: long functions, stacked conditions.

- gocognit at most 10.
- revive:
  - functions of at most 30 lines;
  - at most 3 arguments, tests included;
  - files of at most 300 lines.
- `rumblestrip-vet nesting` at most 2, counting every control statement.

Proof: `m1-simplicity`.

## M2: readability (partial)

Drift: comments that paraphrase the code, names that stutter.

- `rumblestrip-vet comments` allows `// WHY:` comments only, plus doc comments on a contract's exported API and on a command's package.
- staticcheck's ST checks, misspell and predeclared.
- `rumblestrip-vet pkgname` refuses stuttering names and `utils` packages.
- gofumpt and goimports.

Proof: `m2-readability`, `e7-names`, `fmt-formatting`. Left to review: names, by sample.

## M3: consistency (partial)

Drift: each session brings its own library.

- depguard allows one library per need: slog, the standard errors, go-cmp, the MCP go-sdk and cobra (ADR 2).
- forbidigo refuses `fmt.Print*`.
- modernize.
- tagliatelle wants snake_case JSON. Another program's format is decoded in its own casing, in infra or the platform.

Proof: `m3-consistency`.

## M4: one home per rule (partial)

dupl catches copies of 35 tokens or more, in production code. Proof: `m4-duplication`, `m4-copied-rule`. Left to review: a rule rewritten another way.

## M5: minimality (partial)

- unused, unparam, wastedassign and unconvert, and interfacebloat at most 5.
- deadcode, starting from `cmd/`.
- `rumblestrip sweep` reports:
  - code only tests reach, methods included;
  - names a module's inside exports though only their own package uses them.

Proof: `m5-minimality`, `m5-deadcode`, `m5-only-tests`. Left to review: speculative abstractions.

## M7: diagnosability (partial)

- slog only, through depguard.
- sloglint wants attributes, static messages, snake_case keys and no global logger.
- errorlint keeps an error's chain.
- `rumblestrip-vet logreturn` refuses an error that is logged and returned.
- `rumblestrip sweep` refuses a log message written twice.
- forbidigo allows `panic` and `recover` in the platform only, and `os.Exit` in main only.

Proof: `m7-diagnosability`, `m7-duplicate-messages`. Left to review: whether an event deserves a log.

## M8, S10: dependencies and supply chain (blocking)

- govulncheck, on the vulnerabilities the code calls.
- `go mod tidy -diff`.
- The guard holds a new `require` and the `go` and `toolchain` lines for the owner. The `go` line stays on a supported Go, the newest one the pinned golangci-lint supports (ADR 1).
- zizmor and actionlint check workflows: pinned actions, least permissions, no injection.
- cyclonedx-gomod writes an SBOM of what the binary links.
- Dependabot proposes each update a week after its release. `rumblestrip init` writes its file.

Proof: `m8-vulnerable-dep`, `m8-untidy`, `t2-gomod`, `guard-gomod`, `ci-workflow`, `TestSBOMListsTheModulesTheBinaryLinks`, `TestRumblestripRunsTheVersionsToolsTxtPins`. Left to review: every new module.

## M9: traced intent (partial)

- ADRs, and `// WHY:` comments.
- `rumblestrip docs` asks for each markdown file to be reread once 25% of the code has changed since its last reread.
  - At most two docs per change, the oldest first.
  - `.rumblestrip/reread.jsonl` records the rereads.

Proof: `m9-docs-due`, `m9-docs-reread`, `guard-reread-skip`, `TestTheAppCodeOwnersLeaveTheRereadsToTheAgent`. Left to review: whether a reread doc says what the code does.

## M10: testability (blocking)

- depguard's `pure-domain` rule keeps these out of the domain: `os`, `net`, `os/exec`, `database/sql`, `log/slog`, `reflect` and randomness.
- forbidigo refuses `time.Now` and `os.Getenv` in the domain.
- gochecknoglobals and gochecknoinits.
- *Guided:* the template reads its configuration once, at startup.

Proof: `m10-testability`.

## M11: robustness (blocking)

- errcheck, govet, forcetypeassert, nilerr, nilnil, nilnesserr, containedctx, fatcontext, bodyclose, noctx, sqlclosecheck, rowserrcheck and exhaustive.
- `rumblestrip-vet goroutine`: `go` statements in the platform only.
- `rumblestrip-vet ctxroot`: no root context outside main.
- nilaway.
- `go test -race`, on Linux.

Proof: `m11-robustness`, `m11-nil`, `m11-race`. Left to review: races in paths no test runs.

## E1, E2, E7: boundaries and structure (blocking)

- archtest, a gate of `rumblestrip check`, holds the layout:
  - the domain imports the standard library and `pkg/` only;
  - a slice imports no other slice and no infra;
  - the platform imports no module;
  - every package sits in the layout.
- `AGENTS.md` says where code goes, and a template test fails on any path it names that doesn't exist.

Proof: `e1-boundaries`. Left to review: where a new feature goes.

## E3: additive extension (guided)

The template's reference slices, `add` and `list`, show where a feature goes. Left to review: changes to shared code.

An app's copy of the template drifts from Rumblestrip's. A platform change reaches an app by hand.

## E4: an explicit domain model (partial)

- `rumblestrip-vet typedid`: no id of a bare string or integer in the domain.
- `rumblestrip-vet domainconv`: a domain type is built by its Parse function, never converted outside the domain.
- Each module has a glossary, and a template test checks it.

Proof: `e4-typed-ids`.

## E5: stable contracts (blocking)

- **MCP tools.** The spec says what they do, and a golden snapshot holds `tools/list`. Every tool is also a CLI command with the same arguments.
- **Libraries.** apidiff compares the API with the base, and an incompatible change is the owner's to approve.

Proof: `e5-api-break`, `t2-review-sheet`, `TestApidiffReportsEveryCase`.

## E6: refactorability (partial)

testpackage wants black-box `_test` packages, and there are no mocks. Proof: `e6-tests`.

## Security

- **S1: no secret.**
  - gitleaks runs with Rumblestrip's config on the tree, on the history, and before each commit the agent makes.
  - Env files, keys and certificates are refused, whatever they contain.
  - Proof: `s1-secrets`, `s1-sensitive-files`. Left: low-entropy secrets.
- **S2: untrusted input.**
  - gosec rules G107, G110, G304 and G305, plus G204 outside infra and the platform.
  - `rumblestrip-vet sqlconst`: every `database/sql` query is a constant.
  - Proof: `s-security`. Left: a program name taken from input, and SQL built through a helper.
- **S3: a tool says what it does.**
  - An operation declares `ReadOnly`, `Additive` or `Destructive`, with no default, and that becomes its MCP annotations.
  - Guided by the template.
- **S5: crypto and transport.**
  - gosec rules G401, G402, G404 and G501, and forbidigo refuses `InsecureSkipVerify`.
  - Proof: `s-security`.
- **S6: sensitive data.**
  - The `secrets` skill's `Secret` type prints as `[redacted]` through fmt, slog, errors and JSON.
  - Proof: `TestTheSecretsSkillsCodePassesEveryGate`.
- **S7: safe defaults.**
  - gosec rule G108 (pprof).
  - Proof: `s-security`.
- **S8: abuse resistance.**
  - `rumblestrip-vet boundedlist`: an operation that returns a list takes a bounded `limit`.
  - Proof: `s8-unbounded-list`.
- **S11: audit trail.**
  - The template journals every run of an operation that isn't read-only.
  - Guided.

## Every file has a gate

| Files | Checked by |
| --- | --- |
| Go code | golangci-lint, rumblestrip-vet, archtest, nilaway, deadcode, govulncheck, mutation, for each target system |
| Tests | testassert, testpackage, thelper, usetesting, mutation |
| The spec | godog, `rumblestrip spec`, the guard |
| Generated code | linted like the rest; the header is allowed only in `gen/`, `*_gen.go` and `*.pb.go` |
| `go.mod`, `go.sum` | gomoddirectives, `go mod tidy -diff`, govulncheck, the guard |
| Workflows | actionlint, zizmor |
| Shell scripts, git hooks | shellcheck (`sh-script`) |
| PowerShell | PSScriptAnalyzer at warning severity (`ps-script`, `TestCheckFailsOnAPowerShellScript`) |
| Any file | gitleaks (`TestCheckFailsOnAWorkflowAScriptAndASecret`) |
| Markdown | the paths `AGENTS.md` names, the glossaries, the Docs gate |
| Rumblestrip's config in an app | never read; CODEOWNERS on the harness |
| Dockerfiles, IaC | hadolint and Checkov, once an app has them |

## Go's traps

The guard and the linters close the ways C2 has around the checks in Go:

- a custom build tag hides a file from every check;
- Go never builds `_x`, `.x` or `testdata/` directories;
- a `*_windows.go` file goes unlinted on Linux, unless lint runs for each target system;
- a generated-code header turns linters off;
- a new `TestMain` can skip a package's tests;
- `go get pkg@latest` can silently raise the `go` line.

## What stays uncovered

- **Semantic duplication:** dupl only sees tokens.
- **Equivalent mutants:** that's why the gate is a score.
- **Code bent to fit a wrong scenario:** the guard reports a spec change instead of failing on it, and the `new-feature` skill says to fix the scenario on the spec branch.
- **A unit test weakened along with its code:** mutation only sees the lines the change adds.
- **Taste:** whether an event deserves a log, whether names are good, speculative abstractions.

## Principles

- A rule that matters fails CI. Docs only say why.
- Every message is written for the agent: the rule, why it exists, and the fix.
- Examples beat instructions, which is why the template has reference slices.
- Thresholds are absolute, on every change.
- Rumblestrip ships pinned by version, out of the agent's reach.
- Any defect that gets past the gates adds a rule.
- An existing tool comes before Rumblestrip's own code (ADR 5).

## Prior art

- [Harness engineering for coding agent users](https://martinfowler.com/articles/harness-engineering.html), by Birgitta Böckeler: guides and sensors.
- [Harness engineering: leveraging Codex in an agent-first world](https://openai.com/en/index/harness-engineering/), by OpenAI: fixed layers per domain, checked by custom linters.
- [Modular Monolith with DDD](https://github.com/kgrzybek/modular-monolith-with-ddd): the module layout's .NET reference.
