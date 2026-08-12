package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// brokenWriter fails every write, standing in for a closed pipe or a full disk.
type brokenWriter struct{ err error }

func (b brokenWriter) Write([]byte) (int, error) { return 0, b.err }

// execBrokenOut runs a command whose standard output cannot be written to.
func execBrokenOut(args ...string) (code int, stderr string) {
	var errOut strings.Builder
	broken := errors.New("broken pipe")
	code = run(args, ioStreams{
		in:  strings.NewReader(""),
		out: &outWriter{w: brokenWriter{err: broken}},
		err: &errOut,
	})
	return code, errOut.String()
}

// A command that cannot deliver its output has not done its job, whatever the
// command itself thought. Piping into head is the everyday way to hit this.
func TestOutputFailureIsReported(t *testing.T) {
	t.Parallel()

	path := writeFile(t, ".env", "A=1\nB=2\n")

	tests := []struct {
		name string
		args []string
	}{
		{"help", []string{"help"}},
		{"version", []string{"version"}},
		{"fmt", []string{"fmt", path}},
		{"json", []string{"json", path}},
		{"export", []string{"export", path}},
		{"get", []string{"get", "-f", path, "A"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, stderr := execBrokenOut(tt.args...)
			if code != exitFailure {
				t.Errorf("code = %d, want %d", code, exitFailure)
			}
			if !strings.Contains(stderr, "broken pipe") {
				t.Errorf("stderr = %q, want it to name the write failure", stderr)
			}
		})
	}
}

func TestMissingFileFails(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "absent.env")

	tests := []struct {
		name string
		args []string
	}{
		{"fmt", []string{"fmt", missing}},
		{"check", []string{"check", missing}},
		{"diff left", []string{"diff", missing, missing}},
		{"get", []string{"get", "-f", missing, "K"}},
		{"unset", []string{"unset", "-f", missing, "K"}},
		{"export", []string{"export", missing}},
		{"json", []string{"json", missing}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execCLI("", tt.args...)
			if got.code != exitFailure {
				t.Errorf("code = %d, want %d", got.code, exitFailure)
			}
			if got.stderr == "" {
				t.Error("nothing was written to stderr")
			}
		})
	}
}

func TestTooManyFiles(t *testing.T) {
	t.Parallel()

	path := writeFile(t, ".env", "A=1\n")

	for _, name := range []string{"export", "json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := execCLI("", name, path, path); got.code != exitFailure {
				t.Errorf("code = %d, want %d", got.code, exitFailure)
			}
		})
	}
}

func TestBadFlags(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"fmt", "check", "diff", "get", "set", "unset", "export", "json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := execCLI("", name, "-nosuchflag"); got.code != exitFailure {
				t.Errorf("code = %d, want %d", got.code, exitFailure)
			}
		})
	}
}

func TestDiffRejectsTwoStdins(t *testing.T) {
	t.Parallel()

	if got := execCLI("K=v\n", "diff", "-", "-"); got.code != exitFailure {
		t.Errorf("code = %d, want %d", got.code, exitFailure)
	}
}

func TestEditingStdinPrintsInstead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"set", []string{"set", "-f", "-", "B=2"}, "A=1\nB=2\n"},
		{"unset", []string{"unset", "-f", "-", "A"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execCLI("A=1\n", tt.args...)
			if got.code != exitOK {
				t.Fatalf("code = %d: %s", got.code, got.stderr)
			}
			if got.stdout != tt.want {
				t.Errorf("stdout = %q, want %q", got.stdout, tt.want)
			}
		})
	}
}

func TestUnsetDryRun(t *testing.T) {
	t.Parallel()

	path := writeFile(t, ".env", "A=1\nB=2\n")
	got := execCLI("", "unset", "-n", "-f", path, "A")

	if got.stdout != "B=2\n" {
		t.Errorf("stdout = %q, want %q", got.stdout, "B=2\n")
	}
	if on := readFile(t, path); on != "A=1\nB=2\n" {
		t.Errorf("file = %q, want it untouched", on)
	}
}

func TestWriteFailure(t *testing.T) {
	t.Parallel()

	// A path whose parent does not exist: Save cannot place its temporary file.
	path := filepath.Join(t.TempDir(), "no-such-dir", ".env")
	if got := execCLI("", "set", "-f", path, "A=1"); got.code != exitFailure {
		t.Errorf("code = %d, want %d", got.code, exitFailure)
	}
}

func TestMalformedInputFails(t *testing.T) {
	t.Parallel()

	path := writeFile(t, ".env", "K=\"unterminated\n")

	// check recovers from a syntax error and reports it; the others do not.
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"fmt", []string{"fmt", path}, exitFailure},
		{"get", []string{"get", "-f", path, "K"}, exitFailure},
		{"json", []string{"json", path}, exitFailure},
		{"export", []string{"export", path}, exitFailure},
		{"check reports instead", []string{"check", path}, exitFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := execCLI("", tt.args...); got.code != tt.code {
				t.Errorf("code = %d, want %d", got.code, tt.code)
			}
		})
	}
}

func TestParseAssignments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		ok   bool
	}{
		{"one pair", []string{"A=1"}, true},
		{"several", []string{"A=1", "B=2"}, true},
		{"empty value", []string{"A="}, true},
		{"value with equals", []string{"A=b=c"}, true},
		{"nothing", nil, false},
		{"bare word", []string{"A"}, false},
		{"no key", []string{"=1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseAssignments(tt.args)
			if (err == nil) != tt.ok {
				t.Errorf("error = %v, want ok = %v", err, tt.ok)
			}
		})
	}
}

func TestIsShellName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want bool
	}{
		{"APP_NAME", true},
		{"_LEADING", true},
		{"A1", true},
		{"lower", true},
		{"", false},
		{"1LEADING", false},
		{"A.B", false},
		{"A-B", false},
		{"A B", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			if got := isShellName(tt.key); got != tt.want {
				t.Errorf("isShellName(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestPathArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{"none defaults", nil, defaultFile, true},
		{"one", []string{"other.env"}, "other.env", true},
		{"two is an error", []string{"a", "b"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := pathArg(tt.args)
			if (err == nil) != tt.ok {
				t.Fatalf("error = %v, want ok = %v", err, tt.ok)
			}
			if got != tt.want {
				t.Errorf("path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	if got := version(); !strings.HasPrefix(got, "envi ") {
		t.Errorf("version = %q, want it to start with %q", got, "envi ")
	}
}

func TestWriteInPlaceOnANewFile(t *testing.T) {
	t.Parallel()

	// Nothing to preserve, so the file keeps what Save gives it.
	path := filepath.Join(t.TempDir(), ".env")
	if got := execCLI("", "set", "-f", path, "A=1"); got.code != exitOK {
		t.Fatalf("code = %d: %s", got.code, got.stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
