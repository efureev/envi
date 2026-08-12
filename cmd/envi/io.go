package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	envi "github.com/efureev/envi/v2"
)

// defaultFile is what a command reads when given no path, matching the default
// [envi.Load] takes.
const defaultFile = ".env"

// stdinPath is the path that means standard input.
const stdinPath = "-"

// ioStreams carries the three streams a command talks to. They are passed in
// rather than reached for through os so that tests can drive the whole program
// in process.
type ioStreams struct {
	in  io.Reader
	out *outWriter
	err io.Writer
}

// newFlags returns a flag set that reports errors to the command's own stderr
// and never exits the process.
//
// Every command builds its own: package-level flags are mutable global state,
// which would panic on a second registration and race under t.Parallel.
func newFlags(name string, s ioStreams) *flag.FlagSet {
	fs := flag.NewFlagSet("envi "+name, flag.ContinueOnError)
	fs.SetOutput(s.err)
	return fs
}

// pathArg returns the single file a command was given, or the default. More
// than one path is a usage error, which the caller reports.
func pathArg(args []string) (string, error) {
	switch len(args) {
	case 0:
		return defaultFile, nil
	case 1:
		return args[0], nil
	default:
		return "", fmt.Errorf("expected at most one file, got %d", len(args))
	}
}

// readDoc parses one document, from a file or from standard input.
func readDoc(path string, s ioStreams, opts ...envi.Option) (*envi.Env, error) {
	if path == stdinPath {
		e, err := envi.Parse(s.in, opts...)
		if err != nil {
			return nil, fmt.Errorf("stdin: %w", err)
		}
		return e, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	e, err := envi.Parse(f, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return e, nil
}

// readOrCreate is readDoc for the editing commands, where a missing file means
// a document waiting to be written rather than a failure — seeding a fresh
// .env is one of the things this command exists for.
func readOrCreate(path string, s ioStreams, opts ...envi.Option) (*envi.Env, error) {
	e, err := readDoc(path, s, opts...)
	if errors.Is(err, os.ErrNotExist) {
		return envi.New(), nil
	}
	return e, err
}

// writeInPlace saves the document over path, keeping the file's permissions.
//
// [envi.Save] writes through a temporary file and renames it, which is what
// makes the write atomic — and which also means the result carries the
// temporary file's 0600 rather than the original's mode. Restoring it keeps an
// edit from showing up in version control as a permission change.
func writeInPlace(path string, e *envi.Env, opts ...envi.Option) error {
	mode := os.FileMode(0)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	if err := envi.Save(e, path, opts...); err != nil {
		return err
	}

	// A file that did not exist keeps the 0600 Save gives it, which is the
	// right default for something that holds secrets.
	if mode == 0 {
		return nil
	}
	return os.Chmod(path, mode)
}

// configured returns the value a document actually sets for key.
//
// A commented-out row sets nothing, so it reads as absent. That is the view
// [envi.Env.Export] takes; [envi.Env.Lookup] would hand back the value, which
// for a shell script asking what is configured would be a lie.
func configured(e *envi.Env, key string) (string, bool) {
	r := e.Get(key)
	if r == nil || r.IsCommented() {
		return "", false
	}
	return r.Value(), true
}

// configuredPairs lists what a document sets, in document order.
func configuredPairs(e *envi.Env) [][2]string {
	out := make([][2]string, 0, e.Len())
	for r := range e.Rows() {
		if !r.IsCommented() {
			out = append(out, [2]string{r.Key(), r.Value()})
		}
	}
	return out
}

// openReader opens a path for reading, or hands back standard input for "-".
func openReader(path string, s ioStreams) (io.ReadCloser, error) {
	if path == stdinPath {
		return io.NopCloser(s.in), nil
	}
	return os.Open(path)
}

// closeReader closes what openReader returned. A read that already produced its
// result has nothing to learn from a failure to close.
func closeReader(rc io.ReadCloser) {
	_ = rc.Close()
}

// writeJSON writes v indented, the same shape the library's own JSON output
// takes, so that piping either into jq feels the same.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// outWriter remembers the first write failure so that the commands do not have
// to check every print.
//
// It matters: piping into head closes the pipe, and a command that ignored the
// resulting EPIPE would exit 0 having written nothing. run checks the recorded
// error once, in one place.
type outWriter struct {
	w   io.Writer
	err error
}

func (o *outWriter) Write(p []byte) (int, error) {
	if o.err != nil {
		return 0, o.err
	}
	n, err := o.w.Write(p)
	o.err = err
	return n, err
}

func (o *outWriter) printf(format string, a ...any) { _, _ = fmt.Fprintf(o, format, a...) }
func (o *outWriter) println(a ...any)               { _, _ = fmt.Fprintln(o, a...) }
func (o *outWriter) print(a ...any)                 { _, _ = fmt.Fprint(o, a...) }

// warnf writes to standard error, where a failure leaves nowhere to complain.
func warnf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
