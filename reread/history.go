package reread

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/McReaper/rumblestrip-go/internal/platform/git"
)

// WHY: the commit is what the share of changed code is counted from. It is
// all zeros when the working tree changed the line and no commit holds it
// yet, or when git doesn't track File.
type entry struct {
	date, commit string
	time         int64
}

const uncommitted = "0000000000000000000000000000000000000000"

type history struct {
	dir         string
	root        *os.Root
	before, now map[string]entry
}

func (h *history) rereadsAt(ctx context.Context, mb string) (map[string]entry, error) {
	there, err := git.Run(ctx, h.dir, "ls-tree", "--name-only", mb, "--", File)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(there) == "" {
		return map[string]entry{}, nil
	}
	content, err := git.Run(ctx, h.dir, "show", mb+":"+File)
	if err != nil {
		return nil, err
	}
	lines, err := parse([]byte(content))
	if err != nil {
		return nil, err
	}
	origins, err := blame(ctx, h.dir, mb)
	if err != nil {
		return nil, err
	}
	rereads, _ := byDoc(lines, origins)
	return rereads, nil
}

func (h *history) rereadsNow(ctx context.Context) (map[string]entry, []string, error) {
	lines, err := read(h.root)
	if err != nil || len(lines) == 0 {
		return map[string]entry{}, nil, err
	}
	origins, err := blame(ctx, h.dir, "")
	if err != nil {
		return nil, nil, err
	}
	rereads, skips := byDoc(lines, origins)
	return rereads, skips, nil
}

type origin struct {
	commit string
	time   int64
}

func byDoc(lines []numbered, origins map[int]origin) (map[string]entry, []string) {
	rereads := map[string]entry{}
	var skips []string
	for _, l := range lines {
		if l.line.Skip {
			skips = append(skips, l.line.Doc)
			continue
		}
		o, ok := origins[l.n]
		if !ok {
			o = origin{commit: uncommitted}
		}
		rereads[l.line.Doc] = entry{l.line.Reread, o.commit, o.time}
	}
	return rereads, skips
}

// WHY: blaming the working tree, rather than HEAD, counts what the agent
// rereads before it commits, as the guard counts its edits.
func blame(ctx context.Context, dir, rev string) (map[int]origin, error) {
	args := []string{"blame", "--line-porcelain", rev, "--", File}
	if rev == "" {
		tracked, err := git.Run(ctx, dir, "ls-files", "--", File)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(tracked) == "" {
			return map[int]origin{}, nil
		}
		args = []string{"blame", "--line-porcelain", "--", File}
	}
	out, err := git.Run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	return parseBlame(out)
}

var blameHeader = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64}) \d+ (\d+)`)

// WHY: --line-porcelain repeats a commit's fields before each line it
// last changed, so a line's commit and time are the ones read above it.
func parseBlame(out string) (map[int]origin, error) {
	origins := map[int]origin{}
	var o origin
	n := 0
	for l := range strings.SplitSeq(out, "\n") {
		var err error
		switch m := blameHeader.FindStringSubmatch(l); {
		case strings.HasPrefix(l, "\t"):
			origins[n] = o
		case m != nil:
			o.commit = m[1]
			n, err = strconv.Atoi(m[2])
		case strings.HasPrefix(l, "committer-time "):
			o.time, err = strconv.ParseInt(strings.TrimPrefix(l, "committer-time "), 10, 64)
		}
		if err != nil {
			return nil, fmt.Errorf("git blame: %q: %w", l, err)
		}
	}
	return origins, nil
}
