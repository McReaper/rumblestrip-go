package pr_test

import (
	"strings"
	"testing"

	"github.com/McReaper/rumblestrip-go/internal/pr"
)

func TestATitleTheCheckRefusesSaysEachProblem(t *testing.T) {
	long := strings.Repeat("a", 60)
	for title, want := range map[string]int{
		"Refuse blank titles":                         1,
		"feat(notes): " + long:                        1,
		"Refuse blank titles " + long:                 2,
		"fix!: the list starts at the newest note":    0,
		"spec(notes): a deleted note leaves the list": 0,
	} {
		if got := pr.TitleProblems(title); len(got) != want {
			t.Errorf("%q: %v, want %d problems", title, got, want)
		}
	}
}
