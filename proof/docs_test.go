package proof_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

const docsTool = "docs/"

// WHY: when a doc was last reread is the history of its line, so each case
// is a commit on top of the clean trial, the way a pull request sits on main.
func docsFindings(t *testing.T, dir string) []finding {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), kitCommand(t, "rumblestrip"), "docs", "--base", "base", "--json")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil && exitCode(err) != 1 {
		t.Fatalf("rumblestrip docs: %v\n%s", err, stderrOf(err))
	}
	var rows []guardRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("rumblestrip docs: %v\n%s", err, out)
	}
	findings := make([]finding, 0, len(rows))
	for _, r := range rows {
		findings = append(findings, finding{tool: docsTool + r.Rule, file: r.File, line: r.Line, text: r.Message})
	}
	return findings
}

func TestDocsOweNothingInTheCleanTrial(t *testing.T) {
	check(t, wantsOf(docsTool), docsFindings(t, changeOf(t, trialCase{name: "no change"})))
}

func TestDocsReportEveryCase(t *testing.T) {
	for _, c := range loadCases(t) {
		if !c.uses(docsTool) {
			continue
		}
		if !c.isolated {
			t.Errorf("case %s: a docs case is one change: mark it !isolated", c.name)
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			check(t, wantsOf(docsTool, c), docsFindings(t, changeOf(t, c)))
		})
	}
}
