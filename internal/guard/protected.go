package guard

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/staledocs"
)

var (
	goLine      = regexp.MustCompile(`^(go|toolchain) \S+$`)
	requirement = regexp.MustCompile(`^(?:require\s+)?([\w.~/-]+\.[\w.~/-]+)\s+v\S+`)
)

// Harness is what tells the agent how to work and the merge check how to
// judge it: a change to it is the owner's to approve. An app's CODEOWNERS
// names the same paths, so GitHub holds them even if the check is broken.
// A path ending in / is a directory from the root.
//
// WHY: a module's glossary is the agent's: it says what the code's types
// are, a test checks it names each one, and the Docs check has it reread.
// Held for the owner, a fix to it made a change that needed nothing else
// of them wait (the sandbox's first run).
func Harness() []string {
	return []string{".github/", ".claude/", ".rumblestrip/", "AGENTS.md", "CLAUDE.md", "docs/adr/"}
}

// Free lists the files of the harness the agent changes without the
// owner: the rereads of the docs, which a change records as it rereads
// them. A line that leaves a doc out of them is still the owner's.
func Free() []string {
	return []string{staledocs.File}
}

func isHarness(p string) bool {
	if slices.Contains(Free(), p) {
		return false
	}
	for _, h := range Harness() {
		switch {
		case strings.HasSuffix(h, "/") && strings.HasPrefix(p, h), p == h:
			return true
		}
	}
	return false
}

// WHY: the owner reads the spec, not the code, so a change to a test of
// the code is the machines' to judge (mutation, the spec's own proof);
// what stays theirs is what no test can judge: the spec, the harness, what
// the module depends on and the compiler it builds with.
func protected(f File) []Finding {
	var out []Finding
	if isHarness(f.Path) || f.OldPath != "" && isHarness(f.OldPath) {
		out = append(out, Finding{
			"protected-harness", f.Path, 0,
			"T2: " + harnessKind(f.Path) + " " + statusWord(f.Status) + ": the harness tells the agent how to work and the CI how to judge, so the owner approves a change to it before merge",
		})
	}
	if path.Base(f.Path) == "go.mod" && f.Status != 'D' {
		out = append(out, goModChanges(f)...)
	}
	if f.Path == staledocs.File {
		out = append(out, rereadSkips(f)...)
	}
	return out
}

// WHY: what the agent stops rereading would rot unseen, so leaving a doc
// out is the owner's call, like the harness; rereading one is not.
func rereadSkips(f File) []Finding {
	var out []Finding
	for _, l := range f.Added {
		if r, err := staledocs.ParseLine(l.Text); err == nil && r.Skip {
			out = append(out, Finding{
				"protected-reread-skip", f.Path, l.N,
				"T2: " + r.Doc + " left out of the docs to reread: a doc no one rereads goes stale, so the owner approves leaving one out",
			})
		}
	}
	return out
}

func harnessKind(p string) string {
	switch {
	case strings.HasPrefix(p, ".github/"):
		return "the CI or the code owners"
	case strings.HasPrefix(p, ".claude/") || strings.HasPrefix(p, ".rumblestrip/"):
		return "the agent's settings"
	case p == "AGENTS.md" || p == "CLAUDE.md":
		return "the agent's instructions"
	}
	return "a decision record"
}

func statusWord(s byte) string {
	switch s {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	}
	return "changed"
}

func isTestFile(p string) bool {
	return strings.HasSuffix(p, "_test.go") || strings.Contains("/"+p, "/testdata/")
}

func goModChanges(f File) []Finding {
	had := map[string]bool{}
	for _, l := range f.Gone {
		if m := requirement.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			had[m[1]] = true
		}
	}
	var out []Finding
	for _, l := range f.Added {
		t := strings.TrimSpace(l.Text)
		if goLine.MatchString(t) {
			out = append(out, Finding{
				"protected-gomod", f.Path, l.N,
				"M8: the " + strings.Fields(t)[0] + " line moves to " + strings.Fields(t)[1] +
					": the go and toolchain lines choose the compiler and the language, so the owner approves a move before merge, and a dependency bump must not move them by itself",
			})
		}
		m := requirement.FindStringSubmatch(t)
		if m != nil && !had[m[1]] && !strings.Contains(t, "// indirect") {
			out = append(out, Finding{
				"protected-dependency", f.Path, l.N,
				"M8: a new dependency, " + m[1] + ": what the app depends on is the owner's call, bumping one it has is not",
			})
		}
	}
	return out
}
