package envi_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"

	envi "github.com/efureev/envi/v2"
)

// failWriter fails after letting a fixed number of bytes through, so both the
// immediate and the mid-document failure are reachable.
type failWriter struct {
	allow int
	err   error
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.allow <= 0 {
		return 0, w.err
	}
	if len(p) > w.allow {
		n := w.allow
		w.allow = 0
		return n, w.err
	}
	w.allow -= len(p)
	return len(p), nil
}

var errWrite = errors.New("write failed")

func TestEncodePropagatesWriteError(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("APP_NAME", "value"))

	t.Run("Encode", func(t *testing.T) {
		t.Parallel()
		err := envi.NewEncoder(&failWriter{err: errWrite}).Encode(e)
		if !errors.Is(err, errWrite) {
			t.Errorf("error = %v, want it to wrap the writer's", err)
		}
	})

	t.Run("WriteTo", func(t *testing.T) {
		t.Parallel()
		n, err := e.WriteTo(&failWriter{err: errWrite})
		if !errors.Is(err, errWrite) {
			t.Errorf("error = %v, want it to wrap the writer's", err)
		}
		if n != 0 {
			t.Errorf("reported %d bytes written, want 0", n)
		}
	})
}

func TestSaveErrors(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("APP_NAME", "value"))

	t.Run("directory does not exist", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "nope", ".env")
		if err := envi.Save(e, path); err == nil {
			t.Error("Save succeeded into a missing directory")
		}
	})

	t.Run("target is a directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		target := filepath.Join(dir, "taken")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := envi.Save(e, target); err == nil {
			t.Error("Save succeeded onto a directory")
		}

		// The temporary file must not survive the failure.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, ent := range entries {
			if strings.HasPrefix(ent.Name(), ".envi-") {
				t.Errorf("temporary file %s left behind after a failed save", ent.Name())
			}
		}
	})
}

func TestParsePropagatesReaderError(t *testing.T) {
	t.Parallel()

	want := errors.New("reader failed")

	t.Run("immediately", func(t *testing.T) {
		t.Parallel()
		if _, err := envi.Parse(iotest.ErrReader(want)); !errors.Is(err, want) {
			t.Errorf("error = %v, want %v", err, want)
		}
	})

	t.Run("after some input", func(t *testing.T) {
		t.Parallel()
		r := iotest.TimeoutReader(strings.NewReader("A=1\nB=2\nC=3\n"))
		if _, err := envi.Parse(r); err == nil {
			t.Error("Parse succeeded on a reader that failed midway")
		}
	})
}

func TestSyntaxErrorMessage(t *testing.T) {
	t.Parallel()

	t.Run("carries position and source", func(t *testing.T) {
		t.Parallel()
		_, err := envi.ParseString("GOOD=1\nbroken line\n")

		var se *envi.SyntaxError
		if !errors.As(err, &se) {
			t.Fatalf("error %v is not a *SyntaxError", err)
		}
		msg := se.Error()
		for _, want := range []string{"line 2", "column", "broken line"} {
			if !strings.Contains(msg, want) {
				t.Errorf("message %q does not mention %q", msg, want)
			}
		}
	})

	t.Run("truncates a long source line", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("A", 300) + " broken"
		_, err := envi.ParseString(long + "\n")

		var se *envi.SyntaxError
		if !errors.As(err, &se) {
			t.Fatalf("error %v is not a *SyntaxError", err)
		}
		msg := se.Error()
		if !strings.Contains(msg, "…") {
			t.Errorf("long source was not truncated: %q", msg)
		}
		if len(msg) > 200 {
			t.Errorf("message is %d bytes, want it bounded", len(msg))
		}
	})

	t.Run("omits column when unknown", func(t *testing.T) {
		t.Parallel()
		se := &envi.SyntaxError{Line: 7, Msg: "something"}
		if got := se.Error(); strings.Contains(got, "column") {
			t.Errorf("message %q mentions a column it does not have", got)
		}
	})

	t.Run("truncation respects rune boundaries", func(t *testing.T) {
		t.Parallel()
		// Cyrillic is two bytes per rune, so a byte-counting truncation would
		// cut one in half and leave the message invalid.
		se := &envi.SyntaxError{Line: 1, Col: 1, Msg: "x", Src: strings.Repeat("Ж", 100)}

		msg := se.Error()
		if !strings.ContainsRune(msg, '…') {
			t.Fatal("expected the source to be truncated")
		}
		if !utf8.ValidString(msg) {
			t.Errorf("truncation split a multi-byte rune: %q", msg)
		}
	})
}

// Export reports the first failure rather than pressing on silently.
func TestExportReportsFailure(t *testing.T) {
	e := envi.New(envi.NewRow("", "value"))

	if err := e.Export(true); err == nil {
		t.Error("Export succeeded with a key the environment cannot hold")
	}
}

// A value too large for int64 is not a bare integer and must be quoted.
func TestQuoteAlwaysQuotesOversizedNumbers(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"99999999999999999999": `"99999999999999999999"`, // overflows int64
		"+7":                   "+7",
		"-7":                   "-7",
		"7.5":                  `"7.5"`,
		"":                     `""`,
	}
	for value, want := range tests {
		e := envi.New(envi.NewRow("K", value))
		if got, wantLine := encode(t, e, envi.WithQuoting(envi.QuoteAlways)), "K="+want+"\n"; got != wantLine {
			t.Errorf("value %q encoded as %q, want %q", value, got, wantLine)
		}
	}
}

// A comment that merely starts like a block header but is too short to be one
// stays an ordinary comment.
func TestHeaderLookalikeIsJustAComment(t *testing.T) {
	t.Parallel()

	const src = "###   ---[\nAPP_A=1\n"

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if b := e.Block("APP"); b != nil && b.Comment() != "" {
		t.Errorf("block comment = %q, want none", b.Comment())
	}
	if got := e.String(); got != src {
		t.Errorf("round trip changed the document:\ngot  %q\nwant %q", got, src)
	}
}

// Truncation must back off to a rune boundary. Three-byte runes make the cut
// land mid-rune, which two-byte ones happen not to.
func TestSyntaxErrorTruncationBacksOffMidRune(t *testing.T) {
	t.Parallel()

	se := &envi.SyntaxError{Line: 1, Col: 1, Msg: "x", Src: strings.Repeat("…", 60)}

	msg := se.Error()
	if !strings.Contains(msg, "…") {
		t.Fatal("expected truncation")
	}
	if !utf8.ValidString(msg) {
		t.Errorf("truncation split a three-byte rune: %q", msg)
	}
}

// When both definitions of a repeated key carry a trailing comment, the first
// one wins rather than being overwritten.
func TestDuplicateKeyKeepsFirstInlineComment(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("K=1 # first\nK=2 # second\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r := e.Get("K")
	if r.Value() != "2" {
		t.Errorf("value = %q, want the later definition", r.Value())
	}
	if got := r.InlineComment(); got != "first" {
		t.Errorf("inline comment = %q, want the first one kept", got)
	}
}
