package main

import (
	"errors"

	envi "github.com/efureev/envi/v2"
)

// cmdFmt canonicalises files: keys sharing a prefix are gathered into one
// block, and with -sort the document is sorted as well.
//
// A file that is already in order comes through byte for byte identical, which
// is what makes -check and -l trustworthy: they report a difference only when
// there is one.
func cmdFmt(args []string, s ioStreams) int {
	fs := newFlags("fmt", s)
	write := fs.Bool("w", false, "rewrite the file in place instead of printing it")
	list := fs.Bool("l", false, "list the files that would change")
	check := fs.Bool("check", false, "exit 1 if any file would change")
	sort := fs.Bool("sort", false, "sort keys as well as grouping them")
	group := fs.Int("group", 1, "how many keys sharing a prefix make a block")
	indent := fs.Int("indent", 1, "blank lines after each block")
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{defaultFile}
	}

	opts := []envi.Option{envi.WithGroupThreshold(*group), envi.WithIndent(*indent)}
	changed := false

	for _, path := range paths {
		if *write && path == stdinPath {
			return fail(s.err, errors.New("-w cannot rewrite standard input"))
		}

		e, err := readDoc(path, s, opts...)
		if err != nil {
			return fail(s.err, err)
		}

		before := e.String()
		if *sort {
			e.Tidy(opts...)
		} else {
			e.Regroup(opts...)
		}
		after := e.String()
		differs := after != before
		changed = changed || differs

		switch {
		case *list:
			if differs {
				s.out.println(path)
			}
		case *check:
			// Reports through the exit code alone.
		case *write:
			// Nothing to write when the file is already in order: leaving it
			// untouched keeps its timestamp, which build systems watch.
			if differs {
				if err := writeInPlace(path, e, opts...); err != nil {
					return fail(s.err, err)
				}
			}
		default:
			s.out.print(after)
		}
	}

	// -check is the CI gate; -l is the listing gofmt users expect, and reports
	// through its output rather than its status.
	if *check && changed {
		return exitFound
	}
	return exitOK
}
