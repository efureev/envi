// These tests live in package main because a main package cannot be imported,
// which is the one case .claude/rules/tests.md exempts from the external-test
// rule. Everything else about them follows it: t.Parallel throughout, tables
// built by hand, no dependencies.
//
// They drive run() in process rather than building and executing a binary,
// which is why run takes its streams as arguments.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// result is one invocation of the command.
type result struct {
	code   int
	stdout string
	stderr string
}

// execCLI runs the command with the given arguments and standard input.
func execCLI(stdin string, args ...string) result {
	var out, errOut strings.Builder
	code := run(args, ioStreams{
		in:  strings.NewReader(stdin),
		out: &outWriter{w: &out},
		err: &errOut,
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

// writeFile puts content in a temporary file and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// readFile reads a file the command was expected to write.
func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUsageAndVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		code int
		out  bool // output goes to stdout rather than stderr
	}{
		{"no arguments", nil, exitFailure, false},
		{"help", []string{"help"}, exitOK, true},
		{"-h", []string{"-h"}, exitOK, true},
		{"version", []string{"version"}, exitOK, true},
		{"unknown command", []string{"frobnicate"}, exitFailure, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execCLI("", tt.args...)
			if got.code != tt.code {
				t.Errorf("code = %d, want %d", got.code, tt.code)
			}
			written := got.stderr
			if tt.out {
				written = got.stdout
			}
			if written == "" {
				t.Error("nothing was written")
			}
		})
	}
}

func TestExitCodeForFailures(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "absent.env")

	tests := []struct {
		name string
		args []string
	}{
		{"missing file", []string{"check", missing}},
		{"unknown flag", []string{"fmt", "-nope"}},
		{"diff needs two files", []string{"diff", missing}},
		{"get needs a key", []string{"get"}},
		{"set needs an assignment", []string{"set"}},
		{"set rejects a bare word", []string{"set", "NOTANASSIGNMENT"}},
		{"unset needs a key", []string{"unset"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := execCLI("", tt.args...); got.code != exitFailure {
				t.Errorf("code = %d, want %d\nstderr: %s", got.code, exitFailure, got.stderr)
			}
		})
	}
}

func TestFmt(t *testing.T) {
	t.Parallel()

	const messy = "APP_NAME=one\nDB_HOST=localhost\nAPP_DEBUG=false\n"
	const tidy = "APP_NAME=one\nAPP_DEBUG=false\n\nDB_HOST=localhost\n"

	t.Run("prints to stdout and leaves the file alone", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", messy)
		got := execCLI("", "fmt", path)

		if got.code != exitOK {
			t.Fatalf("code = %d, want %d: %s", got.code, exitOK, got.stderr)
		}
		if got.stdout != tidy {
			t.Errorf("stdout = %q, want %q", got.stdout, tidy)
		}
		if on := readFile(t, path); on != messy {
			t.Errorf("the file changed: %q", on)
		}
	})

	t.Run("-w rewrites the file", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", messy)
		if got := execCLI("", "fmt", "-w", path); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}
		if on := readFile(t, path); on != tidy {
			t.Errorf("file = %q, want %q", on, tidy)
		}
	})

	// Save writes through a temporary file, which carries 0600. Restoring the
	// original mode keeps an edit from showing up in version control as a
	// permission change.
	t.Run("-w keeps the file's permissions", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", messy)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}

		if got := execCLI("", "fmt", "-w", path); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("mode = %o, want %o", got, 0o644)
		}
	})

	t.Run("-l lists what would change and exits 0", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", messy)
		got := execCLI("", "fmt", "-l", path)

		if got.code != exitOK {
			t.Errorf("code = %d, want %d", got.code, exitOK)
		}
		if strings.TrimSpace(got.stdout) != path {
			t.Errorf("stdout = %q, want the path %q", got.stdout, path)
		}
	})

	t.Run("-check reports through the exit code", func(t *testing.T) {
		t.Parallel()

		messyPath := writeFile(t, ".env", messy)
		if got := execCLI("", "fmt", "-check", messyPath); got.code != exitFound {
			t.Errorf("code = %d, want %d for an unformatted file", got.code, exitFound)
		}

		tidyPath := writeFile(t, ".env", tidy)
		if got := execCLI("", "fmt", "-check", tidyPath); got.code != exitOK {
			t.Errorf("code = %d, want %d for a formatted file", got.code, exitOK)
		}
	})

	t.Run("-sort orders keys as well", func(t *testing.T) {
		t.Parallel()

		got := execCLI("Z=1\nA=2\n", "fmt", "-sort", "-")
		if want := "A=2\nZ=1\n"; got.stdout != want {
			t.Errorf("stdout = %q, want %q", got.stdout, want)
		}
	})

	t.Run("an already formatted file is left byte for byte", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", tidy)
		if got := execCLI("", "fmt", "-w", path); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}
		if on := readFile(t, path); on != tidy {
			t.Errorf("file = %q, want it unchanged", on)
		}
	})

	t.Run("-w cannot rewrite stdin", func(t *testing.T) {
		t.Parallel()

		if got := execCLI("K=v\n", "fmt", "-w", "-"); got.code != exitFailure {
			t.Errorf("code = %d, want %d", got.code, exitFailure)
		}
	})

	t.Run("a malformed file fails", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "K=\"unterminated\n")
		got := execCLI("", "fmt", path)
		if got.code != exitFailure {
			t.Errorf("code = %d, want %d", got.code, exitFailure)
		}
		if !strings.Contains(got.stderr, "unterminated") {
			t.Errorf("stderr = %q, want it to name the problem", got.stderr)
		}
	})
}
