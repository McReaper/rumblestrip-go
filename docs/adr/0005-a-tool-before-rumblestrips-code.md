# 5. A rule is an existing tool's before it is Rumblestrip's code

## Decision

Rumblestrip writes a check in Go only when no tool it already runs can hold the rule, proven on the rule's case. Whenever a tool can hold the rule, Rumblestrip's code for it goes.

## Why

- Every line Rumblestrip writes is a line someone maintains and an agent can misread. A tool the ecosystem maintains carries its own tests.
- A tool that needs another runtime in every app's CI costs more than the lines it saves.

## What was tried

| Rule | Tool | Result |
| --- | --- | --- |
| A function copied from another module | dupl at 35 tokens | Holds it |
| Keys, certificates and env files committed | a gitleaks path rule | Holds it |
| `t.Skip`, `testing.Short` | forbidigo | Holds it |
| Mutation on changed lines | gremlins | Rejected: on Windows it reports every mutant as not covered |
| Architecture rules | go-arch-lint | Rejected: on Windows it folds every package into one component |
| The guard's bypasses (`//nolint`, `//go:linkname`, cgo, hidden directories, build tags, generated headers, `TestMain`) | semgrep | Rejected: it needs Python in every app's CI, against about 50 lines in the guard |
| The review sheet, size and module rules | Danger | Rejected: it needs Node, and rules in JavaScript |
| rumblestrip-vet's analyzers | golangci-lint's linters | None holds the rule. revive's nesting counts `if` only, gosec misses SQL built through a variable, and no linter knows the template's layout |
