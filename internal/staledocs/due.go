package staledocs

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/platform/git"
)

// Doc is a watched doc and where its reread stands for a change.
type Doc struct {
	Path string
	// Date and Commit are its last reread before the change, both empty
	// when it was never reread.
	Date, Commit string
	// Changed is how much of the code, in percent, has changed since then.
	Changed int
	// Due is whether it was due before the change: never reread, or
	// Changed reached Share.
	Due bool
	// Here is whether the change rereads it.
	Here bool
	time int64
}

// Report is where each watched doc stands for the change from base's merge
// base to the working tree, and how many lines of code that change adds or
// removes.
type Report struct {
	Docs []Doc
	Code int
}

// Owed returns the due docs the change doesn't reread, the oldest first:
// those never reread, then by the date of their last reread.
func (r Report) Owed() []Doc {
	var out []Doc
	for _, d := range r.Docs {
		if d.Due && !d.Here {
			out = append(out, d)
		}
	}
	slices.SortStableFunc(out, func(a, b Doc) int {
		return cmp.Or(cmp.Compare(len(a.Commit), len(b.Commit)), cmp.Compare(a.time, b.time), cmp.Compare(a.Path, b.Path))
	})
	return out
}

// Required returns the docs the change must still reread: the oldest it
// owes, PerChange at most, counting the due docs it already rereads.
//
// WHY: a change that touches no code gives a doc nothing new to be wrong
// about, so it owes nothing: a dependency's update, a doc's fix, or a
// session that hasn't changed anything yet.
func (r Report) Required() []Doc {
	if r.Code == 0 {
		return nil
	}
	reread := 0
	for _, d := range r.Docs {
		if d.Due && d.Here {
			reread++
		}
	}
	owed := r.Owed()
	return owed[:min(len(owed), max(0, PerChange-reread))]
}

// Finding is a doc the change must reread.
type Finding struct {
	Rule    string
	File    string
	Line    int
	Message string
}

// Check returns the docs the change from base to the working tree of the
// repository at dir must still reread.
func Check(ctx context.Context, dir, base string) ([]Finding, error) {
	r, err := Status(ctx, dir, base)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, d := range r.Required() {
		out = append(out, Finding{"due", d.Path, 0, "M9: " + d.Path + " " + d.Since() + ": reread it against the code, fix what no longer holds, then run rumblestrip docs reread " + d.Path})
	}
	return out, nil
}

// Since says how long ago d was reread, in changed code.
func (d Doc) Since() string {
	if d.Commit == "" {
		return "was never reread"
	}
	return fmt.Sprintf("was last reread on %s, and %d%% of the code has changed since", d.Date, d.Changed)
}

// Status returns where each watched doc of the repository at dir stands
// for the change from base's merge base to the working tree.
func Status(ctx context.Context, dir, base string) (r Report, err error) {
	if err := git.CheckRef(base); err != nil {
		return Report{}, fmt.Errorf("base: %w", err)
	}
	mb, err := git.Run(ctx, dir, "merge-base", base, "HEAD")
	if err != nil {
		return Report{}, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Report{}, fmt.Errorf("open %s: %w", dir, err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	h := &history{dir: dir, root: root, base: strings.TrimSpace(mb)}
	if h.before, err = h.rereadsAt(ctx, h.base); err != nil {
		return Report{}, err
	}
	var skips []string
	if h.now, skips, err = h.rereadsNow(ctx); err != nil {
		return Report{}, err
	}
	docs, err := watched(ctx, dir, skips)
	if err != nil {
		return Report{}, err
	}
	return h.report(ctx, docs)
}

func (h *history) report(ctx context.Context, docs []string) (Report, error) {
	c, err := measure(ctx, h.dir, h.root)
	if err != nil {
		return Report{}, err
	}
	var r Report
	if r.Code, err = c.linesSince(ctx, h.base); err != nil {
		return Report{}, err
	}
	for _, p := range docs {
		d, err := h.doc(ctx, c, p)
		if err != nil {
			return Report{}, err
		}
		r.Docs = append(r.Docs, d)
	}
	return r, nil
}

func (h *history) doc(ctx context.Context, c *code, p string) (Doc, error) {
	d := Doc{Path: p, Due: true}
	if b, ok := h.before[p]; ok {
		changed, err := c.changedSince(ctx, b.commit)
		if err != nil {
			return Doc{}, err
		}
		d.Date, d.Commit, d.time, d.Changed, d.Due = b.date, b.commit, b.time, changed, changed >= Share
	}
	n, ok := h.now[p]
	d.Here = ok && n.commit != d.Commit
	return d, nil
}

// Watched lists the docs of the repository at dir that are reread: its
// markdown files but those left out.
func Watched(ctx context.Context, dir string) (docs []string, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dir, err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	lines, err := read(root)
	if err != nil {
		return nil, err
	}
	_, skips := byDoc(lines, nil)
	return watched(ctx, dir, skips)
}

// WHY: test data isn't a doc; a decision record is replaced, never
// updated; a changelog says what was. What File skips is the owner's call.
func watched(ctx context.Context, dir string, skips []string) ([]string, error) {
	out, err := git.Run(ctx, dir, "ls-files", "-z", "-c", "-o", "--exclude-standard", "--", "*.md")
	if err != nil {
		return nil, err
	}
	var docs []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p != "" && !leftOut(p, skips) {
			docs = append(docs, p)
		}
	}
	slices.Sort(docs)
	return slices.Compact(docs), nil
}

func leftOut(p string, skips []string) bool {
	if strings.Contains("/"+p, "/testdata/") || strings.HasPrefix(p, "docs/adr/") || path.Base(p) == "CHANGELOG.md" {
		return true
	}
	return slices.ContainsFunc(skips, func(s string) bool {
		return s == p || strings.HasSuffix(s, "/") && strings.HasPrefix(p, s)
	})
}
