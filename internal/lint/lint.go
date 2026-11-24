// Package lint holds Rumblestrip's golangci-lint config, embedded so that rumblestrip
// check applies it whatever an app carries.
package lint

import _ "embed"

// Config is golangci.yml, beside this file.
//
//go:embed golangci.yml
var Config []byte

// Gitleaks is gitleaks.toml, beside this file: gitleaks' default rules and
// Rumblestrip's sensitive files.
//
//go:embed gitleaks.toml
var Gitleaks []byte
