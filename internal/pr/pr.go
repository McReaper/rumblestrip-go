// Package pr holds what Rumblestrip's check asks of a pull request: a short
// conventional title, which becomes the commit's subject on the default
// branch.
package pr

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// MaxTitle is the longest pull request title: GitHub cuts a longer one in
// its lists, and the title becomes the commit's subject on the default
// branch.
const MaxTitle = 72

var conventional = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|spec)(\([a-z0-9][a-z0-9/._-]*\))?!?: \S`)

// TitleProblems says what is wrong with a pull request's title, one line
// each, the rule first; none when it is a conventional commit within
// MaxTitle.
func TitleProblems(title string) []string {
	var out []string
	if n := utf8.RuneCountInString(title); n > MaxTitle {
		out = append(out, fmt.Sprintf("title: the title has %d characters, the limit is %d: say less, the description holds the rest", n, MaxTitle))
	}
	if !conventional.MatchString(title) {
		out = append(out, "title: the title is no conventional commit: write type(scope): what changes, the type one of feat, fix, docs, refactor, perf, test, build, ci, chore, revert, style, or spec for a spec's pull request")
	}
	return out
}
