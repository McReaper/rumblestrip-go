package check

import (
	"context"
	"path/filepath"

	"github.com/McReaper/rumblestrip-go/reread"
)

func docsGate(ctx context.Context, o Options) Result {
	fs, err := reread.Check(ctx, o.Dir, o.Base)
	if err != nil {
		return failed(err)
	}
	lines := make([]string, 0, len(fs))
	for _, f := range fs {
		lines = append(lines, position(o.Dir, at{filepath.Join(o.Dir, filepath.FromSlash(f.File)), f.Line}, f.Rule+": "+f.Message))
	}
	return found(lines)
}
