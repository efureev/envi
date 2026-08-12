package envi_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// readmeExample is the document the project's own documentation uses. It
// exercises block headers, comments, shadows, commented-out rows, top-level
// rows and a prefix that reappears out of sequence.
const readmeExample = `###   ---[ Application section ]---   ###
# Application name
APP_NAME="App name"
APP_DEBUG=false

# Default dev.host
# APP_URL=http://dev.example.com
APP_URL=https://example.com

###   ---[ NGINX cache section ]---   ###
# Nginx cache path
CACHE_NGINX_PATH=./storage/cache
# Enable caching a page
CACHE_NGINX_ENABLED=false

TEST=false

#APP_TRACE_LOAD=true
DEBUGBAR_ENABLED=false

#HYPE=false
`

func TestRoundTripIsByteIdentical(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := e.String()
	if got != readmeExample {
		t.Errorf("round trip changed the document.\n--- got ---\n%s\n--- want ---\n%s", got, readmeExample)
	}
}

func TestRoundTripIsStable(t *testing.T) {
	t.Parallel()

	first, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	second, err := envi.ParseString(first.String())
	if err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Error("parsing our own output does not reproduce it")
	}
}

func TestParseSemantics(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("values and quotes", func(t *testing.T) {
		t.Parallel()
		if got, _ := e.Lookup("APP_NAME"); got != "App name" {
			t.Errorf("APP_NAME = %q, want quotes resolved", got)
		}
		if got, _ := e.Lookup("CACHE_NGINX_PATH"); got != "./storage/cache" {
			t.Errorf("CACHE_NGINX_PATH = %q", got)
		}
	})

	t.Run("block headers become comments", func(t *testing.T) {
		t.Parallel()
		b := e.Block("APP")
		if b == nil {
			t.Fatal("block APP missing")
		}
		if b.Comment() != "Application section" {
			t.Errorf("block comment = %q", b.Comment())
		}
	})

	t.Run("row comments", func(t *testing.T) {
		t.Parallel()
		if got := e.Get("APP_NAME").Comment(); got != "Application name" {
			t.Errorf("comment = %q", got)
		}
	})

	t.Run("shadows", func(t *testing.T) {
		t.Parallel()
		r := e.Get("APP_URL")
		if r == nil {
			t.Fatal("APP_URL missing")
		}
		if r.Value() != "https://example.com" {
			t.Errorf("value = %q", r.Value())
		}
		if !r.HasShadow("http://dev.example.com") {
			t.Error("the commented alternative was not recorded as a shadow")
		}
		if r.IsCommented() {
			t.Error("the live row must not be marked commented")
		}
	})

	t.Run("commented rows stay inert", func(t *testing.T) {
		t.Parallel()
		r := e.Get("APP_TRACE_LOAD")
		if r == nil {
			t.Fatal("APP_TRACE_LOAD missing")
		}
		if !r.IsCommented() {
			t.Error("a commented-out row must be marked commented")
		}
		if r.Value() != "true" {
			t.Errorf("value = %q", r.Value())
		}
	})

	t.Run("top level rows", func(t *testing.T) {
		t.Parallel()
		if got, ok := e.Lookup("TEST"); !ok || got != "false" {
			t.Errorf("TEST = %q, %v", got, ok)
		}
		if got, ok := e.Lookup("HYPE"); !ok || got != "false" {
			t.Errorf("HYPE = %q, %v", got, ok)
		}
	})
}

func TestParseLineForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, key, value, inline string
	}{
		{"plain", "K=v", "K", "v", ""},
		{"spaces around separator", "K = v", "K", "v", ""},
		{"export prefix", "export K=v", "K", "v", ""},
		{"colon separator", "K: v", "K", "v", ""},
		{"double quotes", `K="a b"`, "K", "a b", ""},
		{"single quotes", `K='a b'`, "K", "a b", ""},
		{"escaped quote", `K="a\"b"`, "K", `a"b`, ""},
		{"escaped newline", `K="a\nb"`, "K", "a\nb", ""},
		{"escaped dollar", `K="a\$b"`, "K", "a$b", ""},
		{"inline comment", "K=v # why", "K", "v", "why"},
		{"hash inside quotes", `K="a#b"`, "K", "a#b", ""},
		{"empty value", "K=", "K", "", ""},
		{"trailing spaces", "K=v   ", "K", "v", ""},
		{"lowercase key", "k=v", "K", "v", ""},
		{"dotted key", "a.b=v", "A.B", "v", ""},
		{"url value", "K=http://x/y?a=1", "K", "http://x/y?a=1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, err := envi.ParseString(tc.in + "\n")
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			r := e.Get(tc.key)
			if r == nil {
				t.Fatalf("key %q missing after parsing %q", tc.key, tc.in)
			}
			if r.Value() != tc.value {
				t.Errorf("value = %q, want %q", r.Value(), tc.value)
			}
			// A trailing comment stays trailing: moving it to a line of its
			// own would turn "# KEY=v" into something that reads back as a
			// shadow.
			if r.InlineComment() != tc.inline {
				t.Errorf("inline comment = %q, want %q", r.InlineComment(), tc.inline)
			}
		})
	}
}

func TestParseSyntaxErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		line int
	}{
		{"bare word", "K=v\nnonsense\n", 2},
		{"missing separator", "GOOD=1\nBAD\n", 2},
		{"unterminated double quote", "K=\"abc\n", 1},
		{"unterminated single quote", "K='abc\n", 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := envi.ParseString(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want a syntax error", tc.in)
			}
			var se *envi.SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("error %v is not a *SyntaxError", err)
			}
			if se.Line != tc.line {
				t.Errorf("Line = %d, want %d", se.Line, tc.line)
			}
			if se.Col <= 0 {
				t.Errorf("Col = %d, want a position", se.Col)
			}
			if !strings.Contains(se.Error(), "line ") {
				t.Errorf("message %q does not state the position", se.Error())
			}
		})
	}
}

// A comment that happens not to parse as an assignment stays a comment rather
// than becoming an error.
func TestCommentsAreNeverSyntaxErrors(t *testing.T) {
	t.Parallel()

	const in = "# just prose, with = signs and \"quotes\n#\n#####\nK=v\n"
	e, err := envi.ParseString(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, _ := e.Lookup("K"); got != "v" {
		t.Errorf("K = %q", got)
	}
	if got := e.String(); got != in {
		t.Errorf("round trip changed prose comments:\ngot  %q\nwant %q", got, in)
	}
}

// bufio.Scanner caps a line at 64 KiB; the hand-written scanner must not.
func TestParseVeryLongLine(t *testing.T) {
	t.Parallel()

	value := strings.Repeat("x", 300*1024)
	e, err := envi.ParseString("SECRET=" + value + "\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, ok := e.Lookup("SECRET")
	if !ok {
		t.Fatal("SECRET missing")
	}
	if len(got) != len(value) {
		t.Errorf("value length = %d, want %d", len(got), len(value))
	}
}

func TestParseCRLF(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("A=1\r\nB=2\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := e.Lookup("A"); got != "1" {
		t.Errorf("A = %q", got)
	}
	if got, _ := e.Lookup("B"); got != "2" {
		t.Errorf("B = %q", got)
	}
}

func TestParseEmptyInput(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("")
	if err != nil {
		t.Fatal(err)
	}
	if e.Len() != 0 {
		t.Errorf("Len = %d, want 0", e.Len())
	}
	if got := e.String(); got != "" {
		t.Errorf("String = %q, want empty", got)
	}
}

func TestParseNoTrailingNewline(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("A=1")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := e.Lookup("A"); got != "1" {
		t.Errorf("A = %q", got)
	}
}

func TestGroupThreshold(t *testing.T) {
	t.Parallel()

	const in = "APP_A=1\nAPP_B=2\nDB_C=3\n"

	t.Run("default groups every prefix", func(t *testing.T) {
		t.Parallel()
		e, err := envi.ParseString(in)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := e.NumBlocks(), 2; got != want {
			t.Errorf("NumBlocks = %d, want %d", got, want)
		}
	})

	t.Run("threshold keeps short runs at top level", func(t *testing.T) {
		t.Parallel()
		e, err := envi.ParseString(in, envi.WithGroupThreshold(2))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := e.NumBlocks(), 1; got != want {
			t.Errorf("NumBlocks = %d, want %d: only APP has two rows", got, want)
		}
		if got, _ := e.Lookup("DB_C"); got != "3" {
			t.Errorf("DB_C = %q, want it reachable at top level", got)
		}
	})
}

// C3: merging files whose keys differ in case lost the earlier comment in v1.
func TestLoadMergesFilesKeepingComments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	base := filepath.Join(dir, ".env")
	local := filepath.Join(dir, ".env.local")

	if err := os.WriteFile(base, []byte("# comment from file 1\nAPP_NAME=one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("app_name=two\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	e, err := envi.Load(base, local)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	r := e.Get("APP_NAME")
	if r == nil {
		t.Fatal("APP_NAME lost during merge")
	}
	if r.Value() != "two" {
		t.Errorf("value = %q, want the override %q", r.Value(), "two")
	}
	if r.Comment() != "comment from file 1" {
		t.Errorf("comment = %q, want it preserved", r.Comment())
	}
	if e.Len() != 1 {
		t.Errorf("Len = %d, want 1", e.Len())
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	_, err := envi.Load(filepath.Join(t.TempDir(), "nope.env"))
	if err == nil {
		t.Fatal("Load of a missing file succeeded")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error %v does not wrap os.ErrNotExist", err)
	}
}

func TestLoadReportsFileInSyntaxError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=1\nbroken line\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := envi.Load(path)
	if err == nil {
		t.Fatal("Load of a malformed file succeeded")
	}
	if !strings.Contains(err.Error(), ".env") {
		t.Errorf("error %q does not name the file", err)
	}
	var se *envi.SyntaxError
	if !errors.As(err, &se) {
		t.Errorf("error %v does not unwrap to *SyntaxError", err)
	}
}

func TestDuplicateKeyFolds(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("K=first\nK=second\n")
	if err != nil {
		t.Fatal(err)
	}
	if e.Len() != 1 {
		t.Errorf("Len = %d, want 1: a repeated key must fold", e.Len())
	}
	r := e.Get("K")
	if r.Value() != "second" {
		t.Errorf("value = %q, want the later definition", r.Value())
	}
	if !r.HasShadow("first") {
		t.Error("the superseded value should be kept as a shadow")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")

	original, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	if err := envi.Save(original, path); err != nil {
		t.Fatal(err)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != readmeExample {
		t.Errorf("file on disk differs from the input:\n%s", onDisk)
	}
}
