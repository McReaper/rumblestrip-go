package staledocs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/rumblestrip-go/internal/staledocs"
)

func TestALineTakesADocAndEitherADateOrSkip(t *testing.T) {
	for text, ok := range map[string]bool{
		`{"doc":"README.md","reread":"2026-11-15"}`:     true,
		`{"doc":"plugin/evals/","skip":true}`:           true,
		`{"doc":"README.md"}`:                           false,
		`{"doc":"README.md","reread":"x","skip":true}`:  false,
		`{"reread":"2026-11-15"}`:                       false,
		`{"doc":"README.md","reread":"x","extra":true}`: false,
		`README.md 2026-11-15`:                          false,
	} {
		if _, err := staledocs.ParseLine(text); (err == nil) != ok {
			t.Errorf("ParseLine(%s): %v, want ok %v", text, err, ok)
		}
	}
}

func TestMarkChangesOnlyTheLinesOfTheDocsItRereads(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"README.md", "docs/guide.md", "docs/new.md"} {
		write(t, filepath.Join(dir, f), "# doc\n")
	}
	write(t, filepath.Join(dir, staledocs.File), strings.Join([]string{
		`{"doc":"README.md","reread":"2026-01-01"}`,
		`{"doc":"evals/","skip":true}`,
		`{"doc":"gone.md","reread":"2026-01-01"}`,
		`{"doc":"docs/guide.md", "reread": "2026-01-01"}`,
	}, "\n")+"\n")
	if err := staledocs.Mark(dir, []string{"README.md", "docs/new.md"}, "2026-11-15"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, staledocs.File))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		`{"doc":"README.md","reread":"2026-11-15"}`,
		`{"doc":"evals/","skip":true}`,
		`{"doc":"docs/guide.md", "reread": "2026-01-01"}`,
		`{"doc":"docs/new.md","reread":"2026-11-15"}`,
	}, "\n") + "\n"
	if string(got) != want {
		t.Errorf("after Mark:\n%s\nwant:\n%s", got, want)
	}
}

func TestMarkWritesTheFileWhenThereIsNone(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "README.md"), "# doc\n")
	if err := staledocs.Mark(dir, []string{"README.md", "README.md"}, "2026-11-15"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, staledocs.File))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"doc":"README.md","reread":"2026-11-15"}` + "\n"; string(got) != want {
		t.Errorf("after Mark: %q, want %q", got, want)
	}
}

func TestAChangeOwesTheOldestDueDocsCountingThoseItRereads(t *testing.T) {
	r := staledocs.Report{Docs: []staledocs.Doc{
		{Path: "a.md", Due: true, Commit: "c1", Date: "2026-01-01"},
		{Path: "b.md", Due: true},
		{Path: "c.md", Due: true, Commit: "c1", Date: "2026-01-01"},
		{Path: "d.md", Due: false, Here: true, Commit: "c1"},
	}}
	if got := paths(r.Required()); got != "b.md a.md" {
		t.Errorf("Required: %s, want the never-reread doc first, then by date and path: b.md a.md", got)
	}
	r.Docs[0].Here = true
	if got := paths(r.Required()); got != "b.md" {
		t.Errorf("Required, a.md reread here: %s, want b.md", got)
	}
	r.Docs[1].Here = true
	if got := paths(r.Required()); got != "" {
		t.Errorf("Required, two due docs reread here: %s, want none", got)
	}
}

func paths(docs []staledocs.Doc) string {
	var out []string
	for _, d := range docs {
		out = append(out, d.Path)
	}
	return strings.Join(out, " ")
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
