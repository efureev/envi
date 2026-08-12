package main

import (
	"errors"
)

// cmdDiff compares what two files configure.
//
// It answers what changed in the configuration, not what changed in the file:
// comments, quote style, block membership and order produce nothing. For the
// other question there is already git diff. Exit status follows diff(1) — 1
// means the two differ.
func cmdDiff(args []string, s ioStreams) int {
	fs := newFlags("diff", s)
	asJSON := fs.Bool("json", false, "write the differences as JSON")
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	paths := fs.Args()
	if len(paths) != 2 {
		return fail(s.err, errors.New("diff needs two files"))
	}
	if paths[0] == stdinPath && paths[1] == stdinPath {
		return fail(s.err, errors.New("only one side can be standard input"))
	}

	left, err := readDoc(paths[0], s)
	if err != nil {
		return fail(s.err, err)
	}
	right, err := readDoc(paths[1], s)
	if err != nil {
		return fail(s.err, err)
	}

	delta := left.Diff(right)

	if *asJSON {
		if err := delta.JSON(s.out); err != nil {
			return fail(s.err, err)
		}
	} else if err := delta.Text(s.out); err != nil {
		return fail(s.err, err)
	}

	if !delta.Empty() {
		return exitFound
	}
	return exitOK
}
