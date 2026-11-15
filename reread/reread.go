// Package reread keeps a repository's docs from going stale (criterion M9):
// each markdown file is reread against the code once a share of the code
// has changed since its last reread, a few docs per change at most, the
// oldest first.
//
// The rereads live in File, one JSON object per line, so that git's history
// of a line says when its doc was last reread: no line, and the doc was
// never reread. A line can also leave a doc, or a directory, out.
package reread

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
)

const (
	// File holds the rereads, relative to the repository's root.
	File = ".rumblestrip/reread.jsonl"
	// Share is how much of the code, in percent, changes before a doc is
	// due again.
	Share = 25
	// PerChange is how many due docs a change must reread, at most.
	PerChange = 2
)

// Line is one line of File: Doc reread on the date Reread, or, with Skip,
// a doc or a directory (ending in /) left out of the rereads.
type Line struct {
	Doc    string `json:"doc"`
	Reread string `json:"reread,omitempty"`
	Skip   bool   `json:"skip,omitempty"`
}

// ParseLine reads one line of File.
func ParseLine(text string) (Line, error) {
	var l Line
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return Line{}, fmt.Errorf("not a reread line: %w", err)
	}
	if l.Doc == "" || l.Skip == (l.Reread != "") {
		return Line{}, errors.New(`not a reread line: it takes "doc" and either "reread" or "skip"`)
	}
	return l, nil
}

// WHY: Mark writes back the text of every line it doesn't reread, not a
// re-encoding, so that git sees those lines unchanged.
type numbered struct {
	n    int
	text string
	line Line
}

func parse(content []byte) ([]numbered, error) {
	var out []numbered
	n := 0
	for text := range strings.SplitSeq(string(content), "\n") {
		n++
		if strings.TrimSpace(text) == "" {
			continue
		}
		l, err := ParseLine(text)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", File, n, err)
		}
		out = append(out, numbered{n, text, l})
	}
	return out, nil
}

func read(root *os.Root) ([]numbered, error) {
	b, err := root.ReadFile(File)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", File, err)
	}
	return parse(b)
}

// Mark records docs as reread on date. A doc's line changes, or is added at
// the end; every other line stays as it is, so its history does too. The
// line of a doc gone from the repository is dropped.
func Mark(dir string, docs []string, date string) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	lines, err := read(root)
	if err != nil {
		return err
	}
	m := marker{root, docs, date, map[string]bool{}}
	var out bytes.Buffer
	for _, l := range lines {
		text, keep, err := m.mark(l)
		if err != nil {
			return err
		}
		if keep {
			out.WriteString(text + "\n")
		}
	}
	if err := m.appendNew(&out); err != nil {
		return err
	}
	return write(root, out.Bytes())
}

type marker struct {
	root   *os.Root
	docs   []string
	date   string
	listed map[string]bool
}

func (m marker) mark(l numbered) (string, bool, error) {
	m.listed[l.line.Doc] = true
	switch {
	case l.line.Skip:
		return l.text, true, nil
	case slices.Contains(m.docs, l.line.Doc):
		text, err := encode(Line{Doc: l.line.Doc, Reread: m.date})
		return text, true, err
	}
	return l.text, exists(m.root, l.line.Doc), nil
}

func (m marker) appendNew(out *bytes.Buffer) error {
	for _, d := range m.docs {
		if m.listed[d] {
			continue
		}
		m.listed[d] = true
		text, err := encode(Line{Doc: d, Reread: m.date})
		if err != nil {
			return err
		}
		out.WriteString(text + "\n")
	}
	return nil
}

func encode(l Line) (string, error) {
	b, err := json.Marshal(l)
	if err != nil {
		return "", fmt.Errorf("encode the line of %s: %w", l.Doc, err)
	}
	return string(b), nil
}

func exists(root *os.Root, doc string) bool {
	_, err := root.Stat(doc)
	return err == nil
}

func write(root *os.Root, content []byte) error {
	if err := root.MkdirAll(".rumblestrip", 0o750); err != nil {
		return fmt.Errorf("write %s: %w", File, err)
	}
	if err := root.WriteFile(File, content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", File, err)
	}
	return nil
}
