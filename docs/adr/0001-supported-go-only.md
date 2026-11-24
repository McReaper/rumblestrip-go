# 1. Build on a supported Go release, its latest patch

## Decision

- The `go` line is the newest Go that the pinned golangci-lint can analyze, because golangci-lint must be built with a Go at least as new. The `toolchain` line is that release's latest patch.
- The release must be one that Go still supports, and govulncheck fails on any vulnerability the code calls.
- Moving either line is a protected change, so it needs the owner's approval.

## Why

- Go fixes vulnerabilities in its two newest minor releases only. An unsupported release keeps known vulnerabilities, even at its last patch.
- `go get pkg@latest` can raise the `go` line silently, and that switches the toolchain too.

## Consequences

When a Go release reaches end of life, govulncheck fails the gate before anything ships. Moving to the next release is a reviewed change, made together with golangci-lint.
