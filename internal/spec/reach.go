package spec

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/analyzers/layout"
	"github.com/McReaper/rumblestrip-go/internal/platform/run"
)

// Reachable reports whether a scenario must run the lines a change adds
// to the file at p, from the module's root: the Go files of a module's
// slices and domain, tests left out.
//
// WHY: the owner reads the spec, not the code, so what no scenario runs
// is behaviour they never approved. The infra, the wiring and the
// platform stay the unit tests': a full disk can't be a scenario.
func Reachable(p string) bool {
	if path.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") || strings.Contains("/"+p, "/testdata/") {
		return false
	}
	dir := path.Dir(p)
	return layout.IsDomainPath(dir) || layout.IsSlicePath(dir)
}

// WHY: a coverage profile counts blocks of statements, not lines: a line
// ran when a block that holds it ran.
type block struct {
	file       string
	start, end int
	reached    bool
}

func (b block) holds(file string, line int) bool {
	return b.file == file && line >= b.start && line <= b.end
}

const profileFile = ".rumblestrip-cover.txt"

var profileLine = regexp.MustCompile(`^(.+\.go):(\d+)\.\d+,(\d+)\.\d+ \d+ (\d+)$`)

// WHY: the runs of the scenarios as written are all that a program built
// with -cover reached; their counters merge into one text profile.
func reachedBlocks(ctx context.Context, root *os.Root, dirs []string) ([]block, error) {
	out, err := run.Command(ctx, root.Name(), "go", "tool", "covdata", "textfmt", "-i="+strings.Join(dirs, ","), "-o="+profileFile)
	if err != nil {
		return nil, err
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("merge the scenarios' coverage: %s", strings.TrimSpace(string(out.Stderr)))
	}
	data, err := root.ReadFile(profileFile)
	if err != nil {
		return nil, fmt.Errorf("read the scenarios' coverage: %w", err)
	}
	return parseProfile(string(data), modulePath(root)), nil
}

// WHY: a profile names files by import path; findings name them from the
// module's root, as the diff does.
func parseProfile(text, module string) []block {
	var out []block
	for l := range strings.Lines(text) {
		m := profileLine.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		file, ok := strings.CutPrefix(m[1], module+"/")
		start, serr := strconv.Atoi(m[2])
		end, eerr := strconv.Atoi(m[3])
		if ok && serr == nil && eerr == nil {
			out = append(out, block{file: file, start: start, end: end, reached: m[4] != "0"})
		}
	}
	return out
}

var moduleLine = regexp.MustCompile(`(?m)^module\s+(\S+)`)

func modulePath(root *os.Root) string {
	data, err := root.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	if m := moduleLine.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}

// WHY: one finding a block no run reached, at the first line the change
// added to it, says where to look without repeating itself line by line.
func unreached(root *os.Root, blocks []block, added map[string][]int) ([]Finding, error) {
	var out []Finding
	files := make([]string, 0, len(added))
	for f := range added {
		files = append(files, f)
	}
	slices.Sort(files)
	for _, f := range files {
		src, err := root.ReadFile(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		c := changedFile{path: f, added: added[f], errReturn: errorReturns(f, src)}
		for _, line := range c.unreached(blocks) {
			out = append(out, Finding{
				"spec-unreached", f, line,
				"T2: no scenario reaches this, so the owner never approved what it does: give it a scenario, a change to the spec they approve, or remove it",
			})
		}
	}
	return out, nil
}

// WHY: a line a reached block also holds ran, and a line that only
// returns the error a call gave back is the unit tests' (errorReturns).
type changedFile struct {
	path      string
	added     []int
	errReturn func(line int) bool
}

func (c changedFile) unreached(blocks []block) []int {
	var out []int
	for _, b := range blocks {
		if l, ok := c.firstUnreached(b, blocks); ok && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

func (c changedFile) firstUnreached(b block, blocks []block) (int, bool) {
	if b.file != c.path || b.reached {
		return 0, false
	}
	for _, l := range c.added {
		if b.holds(c.path, l) && !c.errReturn(l) && !slices.ContainsFunc(blocks, func(o block) bool { return o.reached && o.holds(c.path, l) }) {
			return l, true
		}
	}
	return 0, false
}

// WHY: an `if err != nil { return … }` runs only when a call fails, which
// for the infra's I/O no scenario can make happen; the compiler and
// errcheck already refuse a dropped error, and mutation leaves these out
// too (ADR 4). A refusal the user sees is made elsewhere, where a scenario
// must reach it.
func errorReturns(file string, src []byte) func(int) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
	if err != nil {
		return func(int) bool { return false }
	}
	type span struct{ from, to int }
	var spans []span
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.IfStmt); ok && isErrReturn(s) {
			spans = append(spans, span{fset.Position(s.Body.Lbrace).Line, fset.Position(s.Body.Rbrace).Line})
		}
		return true
	})
	return func(l int) bool {
		return slices.ContainsFunc(spans, func(s span) bool { return l > s.from && l <= s.to })
	}
}

func isErrReturn(s *ast.IfStmt) bool {
	cond, ok := s.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.NEQ || s.Else != nil || len(s.Body.List) != 1 {
		return false
	}
	x, xok := cond.X.(*ast.Ident)
	y, yok := cond.Y.(*ast.Ident)
	_, returns := s.Body.List[0].(*ast.ReturnStmt)
	return xok && yok && x.Name == "err" && y.Name == "nil" && returns
}
