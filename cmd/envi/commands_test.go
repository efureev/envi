package main

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	const bad = "app-name=x\nK=\"unterminated\nDUP=1\nDUP=2\nEMPTY=\n"

	t.Run("findings carry the file they came from", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", bad)
		got := execCLI("", "check", path)

		if got.code != exitFound {
			t.Errorf("code = %d, want %d", got.code, exitFound)
		}
		for _, line := range strings.Split(strings.TrimSpace(got.stdout), "\n") {
			if !strings.HasPrefix(line, path+":") {
				t.Errorf("finding %q does not start with the path %q", line, path)
			}
		}
	})

	t.Run("a clean file reports nothing", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "APP_NAME=x\n")
		got := execCLI("", "check", path)

		if got.code != exitOK {
			t.Errorf("code = %d, want %d", got.code, exitOK)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want empty", got.stdout)
		}
	})

	t.Run("-strict turns warnings into a failure", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "app-name=x\n")

		if got := execCLI("", "check", path); got.code != exitOK {
			t.Errorf("code = %d, want %d without -strict", got.code, exitOK)
		}
		if got := execCLI("", "check", "-strict", path); got.code != exitFound {
			t.Errorf("code = %d, want %d with -strict", got.code, exitFound)
		}
	})

	t.Run("-off silences a rule", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "app-name=x\nEMPTY=\n")
		got := execCLI("", "check", "-off", "key-not-canonical,empty-value", path)

		if got.stdout != "" {
			t.Errorf("stdout = %q, want empty once both rules are off", got.stdout)
		}
	})

	t.Run("-json parses back", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", bad)
		got := execCLI("", "check", "-json", path)

		var findings []jsonFinding
		if err := json.Unmarshal([]byte(got.stdout), &findings); err != nil {
			t.Fatalf("output does not parse: %v\n%s", err, got.stdout)
		}
		if len(findings) == 0 {
			t.Fatal("no findings")
		}
		for _, f := range findings {
			if f.File != path {
				t.Errorf("File = %q, want %q", f.File, path)
			}
			if f.Rule == "" || f.Severity == "" {
				t.Errorf("incomplete finding: %+v", f)
			}
		}
	})

	t.Run("-json on a clean file writes an empty array", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "APP_NAME=x\n")
		got := execCLI("", "check", "-json", path)

		if want := "[]"; strings.TrimSpace(got.stdout) != want {
			t.Errorf("stdout = %q, want %q", strings.TrimSpace(got.stdout), want)
		}
	})
}

func TestDiff(t *testing.T) {
	t.Parallel()

	t.Run("differences exit 1", func(t *testing.T) {
		t.Parallel()

		left := writeFile(t, "a.env", "A=1\nB=2\n")
		right := writeFile(t, "b.env", "A=9\nC=3\n")
		got := execCLI("", "diff", left, right)

		if got.code != exitFound {
			t.Errorf("code = %d, want %d", got.code, exitFound)
		}
		want := "~ A: \"1\" -> \"9\"\n- B=\"2\"\n+ C=\"3\"\n"
		if got.stdout != want {
			t.Errorf("stdout = %q, want %q", got.stdout, want)
		}
	})

	t.Run("identical documents exit 0", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, "a.env", "A=1\n")
		got := execCLI("", "diff", path, path)

		if got.code != exitOK {
			t.Errorf("code = %d, want %d", got.code, exitOK)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want empty", got.stdout)
		}
	})

	t.Run("-json parses back", func(t *testing.T) {
		t.Parallel()

		left := writeFile(t, "a.env", "A=1\n")
		right := writeFile(t, "b.env", "A=2\n")
		got := execCLI("", "diff", "-json", left, right)

		var changes []map[string]any
		if err := json.Unmarshal([]byte(got.stdout), &changes); err != nil {
			t.Fatalf("output does not parse: %v\n%s", err, got.stdout)
		}
		if len(changes) != 1 || changes[0]["kind"] != "changed" {
			t.Errorf("changes = %v, want one changed", changes)
		}
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		key     string
		code    int
		stdout  string
	}{
		{"configured key", "APP_NAME=one\n", "APP_NAME", exitOK, "one\n"},
		{"key spelled differently", "APP_NAME=one\n", "app-name", exitOK, "one\n"},
		{"absent key", "APP_NAME=one\n", "NOPE", exitFound, ""},
		{"empty value counts as configured", "K=\n", "K", exitOK, "\n"},
		// A commented row configures nothing, so get must say so rather than
		// hand a script a value the process will never see.
		{"commented key", "# K=1\n", "K", exitFound, ""},
		{"commented beside a live one", "# K=1\nK=2\n", "K", exitOK, "2\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, ".env", tt.content)
			got := execCLI("", "get", "-f", path, tt.key)

			if got.code != tt.code {
				t.Errorf("code = %d, want %d", got.code, tt.code)
			}
			if got.stdout != tt.stdout {
				t.Errorf("stdout = %q, want %q", got.stdout, tt.stdout)
			}
		})
	}
}

func TestSet(t *testing.T) {
	t.Parallel()

	const src = `###   ---[ Application ]---   ###
# human readable name
APP_NAME="My App"
APP_DEBUG=false
`

	t.Run("changes one line and leaves the rest", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", src)
		if got := execCLI("", "set", "-f", path, "APP_DEBUG=true"); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}

		want := strings.Replace(src, "APP_DEBUG=false", "APP_DEBUG=true", 1)
		if on := readFile(t, path); on != want {
			t.Errorf("file =\n%q\nwant\n%q", on, want)
		}
	})

	t.Run("several keys at once", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "A=1\nB=2\n")
		if got := execCLI("", "set", "-f", path, "A=9", "C=3"); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}
		if want, on := "A=9\nB=2\nC=3\n", readFile(t, path); on != want {
			t.Errorf("file = %q, want %q", on, want)
		}
	})

	// Env.Set edits whatever row it finds and Row.SetValue leaves the commented
	// flag alone, so without SetCommented(false) the key would keep its value in
	// a row that configures nothing.
	t.Run("a commented key becomes configured", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "# K=old\n")
		if got := execCLI("", "set", "-f", path, "K=new"); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}

		if on := readFile(t, path); strings.Contains(on, "#") {
			t.Errorf("file = %q, want the row uncommented", on)
		}
		if got := execCLI("", "get", "-f", path, "K"); got.code != exitOK || got.stdout != "new\n" {
			t.Errorf("get = %q (code %d), want %q", got.stdout, got.code, "new\n")
		}
	})

	t.Run("-n leaves the file alone", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "A=1\n")
		got := execCLI("", "set", "-n", "-f", path, "A=2")

		if got.stdout != "A=2\n" {
			t.Errorf("stdout = %q, want %q", got.stdout, "A=2\n")
		}
		if on := readFile(t, path); on != "A=1\n" {
			t.Errorf("file = %q, want it untouched", on)
		}
	})

	t.Run("a missing file is created", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), ".env")
		if got := execCLI("", "set", "-f", path, "A=1"); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}
		if on := readFile(t, path); on != "A=1\n" {
			t.Errorf("file = %q, want %q", on, "A=1\n")
		}
	})

	t.Run("a value may contain an equals sign", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "")
		if got := execCLI("", "set", "-f", path, "Q=a=b"); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}
		if got := execCLI("", "get", "-f", path, "Q"); got.stdout != "a=b\n" {
			t.Errorf("get = %q, want %q", got.stdout, "a=b\n")
		}
	})
}

func TestUnset(t *testing.T) {
	t.Parallel()

	t.Run("removes keys", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "A=1\nB=2\nC=3\n")
		if got := execCLI("", "unset", "-f", path, "A", "C"); got.code != exitOK {
			t.Fatalf("code = %d: %s", got.code, got.stderr)
		}
		if want, on := "B=2\n", readFile(t, path); on != want {
			t.Errorf("file = %q, want %q", on, want)
		}
	})

	t.Run("an absent key is not an error", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "A=1\n")
		if got := execCLI("", "unset", "-f", path, "NOPE"); got.code != exitOK {
			t.Errorf("code = %d, want %d", got.code, exitOK)
		}
		if on := readFile(t, path); on != "A=1\n" {
			t.Errorf("file = %q, want it unchanged", on)
		}
	})
}

func TestJSON(t *testing.T) {
	t.Parallel()

	path := writeFile(t, ".env", "B=2\nA=1\n# HIDDEN=x\nSPACED=\"a b\"\n")
	got := execCLI("", "json", path)

	if got.code != exitOK {
		t.Fatalf("code = %d: %s", got.code, got.stderr)
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(got.stdout), &out); err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, got.stdout)
	}
	want := map[string]string{"A": "1", "B": "2", "SPACED": "a b"}
	if !maps.Equal(out, want) {
		t.Errorf("json = %v, want %v", out, want)
	}
}

func TestExport(t *testing.T) {
	t.Parallel()

	t.Run("commented rows are skipped", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "A=1\n# HIDDEN=x\n")
		got := execCLI("", "export", path)

		if want := "export A='1'\n"; got.stdout != want {
			t.Errorf("stdout = %q, want %q", got.stdout, want)
		}
	})

	t.Run("-n drops the keyword", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "A=1\n")
		if got := execCLI("", "export", "-n", path); got.stdout != "A='1'\n" {
			t.Errorf("stdout = %q, want %q", got.stdout, "A='1'\n")
		}
	})

	// A key may hold a dot, which a .env file allows and the shell does not.
	// Emitting it would make the whole eval a syntax error, taking the valid
	// assignments down with it.
	t.Run("keys the shell cannot name are skipped with a warning", func(t *testing.T) {
		t.Parallel()

		path := writeFile(t, ".env", "A.B=1\nGOOD=2\n")
		got := execCLI("", "export", path)

		if got.stdout != "export GOOD='2'\n" {
			t.Errorf("stdout = %q, want only the usable key", got.stdout)
		}
		if !strings.Contains(got.stderr, "A.B") {
			t.Errorf("stderr = %q, want it to name the skipped key", got.stderr)
		}
		if got.code != exitOK {
			t.Errorf("code = %d, want %d: what was written is still valid", got.code, exitOK)
		}
	})
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "abc", `'abc'`},
		{"empty", "", `''`},
		{"space", "a b", `'a b'`},
		{"single quote", "it's", `'it'\''s'`},
		{"double quote", `he said "hi"`, `'he said "hi"'`},
		{"dollar", "$HOME", `'$HOME'`},
		{"backtick", "a`b`c", "'a`b`c'"},
		{"hash", "x#y", `'x#y'`},
		{"newline", "a\nb", "'a\nb'"},
		{"only quotes", "''", `''\'''\'''`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shellQuote(tt.value); got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// The claim export makes is that its output is valid shell. The only way to
// know is to hand it to one: quoting that looks right and expands wrong is
// exactly the failure this command exists to prevent.
func TestExportSurvivesTheShell(t *testing.T) {
	t.Parallel()

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}

	values := map[string]string{
		"TRICKY":   "it's",
		"SPACED":   "a b  c",
		"HASH":     "x#y",
		"DOLLAR":   "$HOME",
		"BACKTICK": "a`b`c",
		"QUOTED":   `he said "hi"`,
		"EMPTY":    "",
	}

	// Build the fixture with the library so the file is encoded the way .env
	// encodes things. Hand-quoting it here would only test this test.
	doc := envi.New()
	for _, key := range sortedKeys(values) {
		doc.Set(key, values[key])
	}
	path := writeFile(t, ".env", doc.String())

	exported := execCLI("", "export", path)
	if exported.code != exitOK {
		t.Fatalf("export failed: %s", exported.stderr)
	}

	// Evaluate what export produced, then print each value back one per line.
	var script strings.Builder
	script.WriteString(exported.stdout)
	for _, key := range sortedKeys(values) {
		script.WriteString("printf '%s\\n' \"$" + key + "\"\n")
	}

	out, err := exec.Command(sh, "-c", script.String()).Output() //nolint:gosec // the script is built here, not taken from input
	if err != nil {
		t.Fatalf("the shell rejected the output: %v\nscript:\n%s", err, script.String())
	}

	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	keys := sortedKeys(values)
	if len(got) != len(keys) {
		t.Fatalf("got %d values, want %d:\n%s", len(got), len(keys), out)
	}
	for i, key := range keys {
		if got[i] != values[key] {
			t.Errorf("%s = %q after eval, want %q", key, got[i], values[key])
		}
	}
}

func sortedKeys(m map[string]string) []string {
	return slices.Sorted(maps.Keys(m))
}

func TestStdin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		stdin  string
		stdout string
	}{
		{"fmt", []string{"fmt", "-"}, "A_X=1\nA_Y=2\n", "A_X=1\nA_Y=2\n"},
		{"get", []string{"get", "-f", "-", "K"}, "K=v\n", "v\n"},
		{"json", []string{"json", "-"}, "K=v\n", "{\n  \"K\": \"v\"\n}\n"},
		{"export", []string{"export", "-"}, "K=v\n", "export K='v'\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execCLI(tt.stdin, tt.args...)
			if got.code != exitOK {
				t.Fatalf("code = %d: %s", got.code, got.stderr)
			}
			if got.stdout != tt.stdout {
				t.Errorf("stdout = %q, want %q", got.stdout, tt.stdout)
			}
		})
	}
}

// A file this command creates holds configuration, often secrets, so nobody
// but its owner should be able to read it. Save gives it 0600 and there is no
// earlier mode to restore.
//
// Windows has no such bits — os.Chmod there uses only 0200 and Stat reports
// 0666 for anything writable — so the property cannot be stated, let alone
// checked.
func TestFileModeOnCreate(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("windows does not carry unix permission bits")
	}

	path := filepath.Join(t.TempDir(), ".env")
	if got := execCLI("", "set", "-f", path, "SECRET=x"); got.code != exitOK {
		t.Fatalf("code = %d: %s", got.code, got.stderr)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Errorf("mode = %o, want nothing readable by group or other", got)
	}
}
