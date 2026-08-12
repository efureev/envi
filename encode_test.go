package envi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// block builds a block, failing the test if a row is rejected.
func block(t *testing.T, prefix, comment string, rows ...*envi.Row) *envi.Block {
	t.Helper()
	b := envi.NewBlock(prefix)
	if comment != "" {
		b.SetComment(comment)
	}
	if err := b.Add(rows...); err != nil {
		t.Fatalf("building block %q: %v", prefix, err)
	}
	return b
}

func encode(t *testing.T, e *envi.Env, opts ...envi.Option) string {
	t.Helper()
	var sb strings.Builder
	if err := envi.NewEncoder(&sb, opts...).Encode(e); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return sb.String()
}

func TestEncodeTopLevelRow(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("HYPE", "false"))
	if got, want := e.String(), "HYPE=false\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEncodeBlockWithComment(t *testing.T) {
	t.Parallel()

	e := envi.New(block(t, "APP", "Application",
		envi.NewRow("APP_NAME", "App"),
		envi.NewRow("APP_DEBUG", "false"),
	))

	want := "###   ---[ Application ]---   ###\n" +
		"APP_NAME=App\n" +
		"APP_DEBUG=false\n"
	if got := e.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEncodeBlocksSeparatedByIndent(t *testing.T) {
	t.Parallel()

	e := envi.New(
		block(t, "APP", "", envi.NewRow("APP_A", "1")),
		block(t, "DB", "", envi.NewRow("DB_B", "2")),
	)

	t.Run("default indent", func(t *testing.T) {
		t.Parallel()
		want := "APP_A=1\n\nDB_B=2\n"
		if got := encode(t, e); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("indent 0", func(t *testing.T) {
		t.Parallel()
		want := "APP_A=1\nDB_B=2\n"
		if got := encode(t, e, envi.WithIndent(0)); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("indent 3", func(t *testing.T) {
		t.Parallel()
		want := "APP_A=1\n\n\n\nDB_B=2\n"
		if got := encode(t, e, envi.WithIndent(3)); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestEncodeCommentsAndShadows(t *testing.T) {
	t.Parallel()

	r := envi.NewRow("REDIS_HOST", "127.0.0.1").
		SetComment("for docker").
		AddShadow("redis")
	e := envi.New(r)

	t.Run("all included", func(t *testing.T) {
		t.Parallel()
		want := "# for docker\n# REDIS_HOST=redis\nREDIS_HOST=127.0.0.1\n"
		if got := encode(t, e); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("without comments", func(t *testing.T) {
		t.Parallel()
		want := "# REDIS_HOST=redis\nREDIS_HOST=127.0.0.1\n"
		if got := encode(t, e, envi.WithComments(false)); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("without shadows", func(t *testing.T) {
		t.Parallel()
		want := "# for docker\nREDIS_HOST=127.0.0.1\n"
		if got := encode(t, e, envi.WithShadows(false)); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestEncodeMultiLineComment(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("DB_PATH", "/data").SetComment("Path to db\nUse carefully"))
	want := "# Path to db\n# Use carefully\nDB_PATH=/data\n"
	if got := e.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEncodeCommentedRowDropsShadows(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("HYPE", "false").SetCommented(true).AddShadow("true"))

	t.Run("included", func(t *testing.T) {
		t.Parallel()
		// A row that is itself inert has nothing for a shadow to shadow.
		if got, want := encode(t, e), "# HYPE=false\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("excluded", func(t *testing.T) {
		t.Parallel()
		if got, want := encode(t, e, envi.WithCommentedRows(false)), ""; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestEncodeQuoting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		value           string
		minimal, always string
	}{
		{"plain", "App", "App", `"App"`},
		{"bool", "false", "false", "false"},
		{"int", "8080", "8080", "8080"},
		{"spaces inside", "a b", "a b", `"a b"`},
		{"hash", "a#b", `"a#b"`, `"a#b"`},
		{"quote", `a"b`, `"a\"b"`, `"a\"b"`},
		{"newline", "a\nb", `"a\nb"`, `"a\nb"`},
		{"dollar", "a$b", `"a\$b"`, `"a\$b"`},
		{"leading space", " x", `" x"`, `" x"`},
		{"empty", "", "", `""`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := envi.New(envi.NewRow("K", tc.value))

			if got, want := encode(t, e, envi.WithQuoting(envi.QuoteMinimal)), "K="+tc.minimal+"\n"; got != want {
				t.Errorf("minimal: got %q, want %q", got, want)
			}
			if got, want := encode(t, e, envi.WithQuoting(envi.QuoteAlways)), "K="+tc.always+"\n"; got != want {
				t.Errorf("always: got %q, want %q", got, want)
			}
		})
	}
}

func TestEncodeSortedOrderDoesNotMutate(t *testing.T) {
	t.Parallel()

	e := envi.New(
		envi.NewRow("ZULU", "1"),
		envi.NewRow("ALPHA", "2"),
	)

	if got, want := encode(t, e, envi.WithOrder(envi.OrderSorted)), "ALPHA=2\nZULU=1\n"; got != want {
		t.Errorf("sorted: got %q, want %q", got, want)
	}
	// The document itself must be untouched by having been encoded.
	if got, want := e.String(), "ZULU=1\nALPHA=2\n"; got != want {
		t.Errorf("after sorted encode the document changed: got %q, want %q", got, want)
	}
}

func TestEncodeEmptyBlockIsSkipped(t *testing.T) {
	t.Parallel()

	e := envi.New(
		envi.NewBlock("EMPTY").SetComment("nothing here"),
		envi.NewRow("HYPE", "false"),
	)
	if got, want := e.String(), "HYPE=false\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEncodeEmptyDocument(t *testing.T) {
	t.Parallel()

	if got := envi.New().String(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	var nilEnv *envi.Env
	if err := envi.NewEncoder(&strings.Builder{}).Encode(nilEnv); err != nil {
		t.Errorf("encoding a nil document: %v", err)
	}
}

func TestSaveIsAtomicAndComplete(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	e := envi.New(envi.NewRow("APP_NAME", "App"))
	if err := envi.Save(e, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "APP_NAME=App\n"; string(got) != want {
		t.Errorf("file holds %q, want %q", got, want)
	}

	// No temporary file may survive a successful save.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if strings.HasPrefix(ent.Name(), ".envi-") {
			t.Errorf("temporary file %s left behind", ent.Name())
		}
	}
}

func TestWriteToReportsCount(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("APP_NAME", "App"))
	var sb strings.Builder
	n, err := e.WriteTo(&sb)
	if err != nil {
		t.Fatal(err)
	}
	if int(n) != sb.Len() {
		t.Errorf("WriteTo reported %d bytes, wrote %d", n, sb.Len())
	}
}

func TestMarshalText(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("APP_NAME", "App"))
	b, err := e.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "APP_NAME=App\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
