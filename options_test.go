package envi_test

import (
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

func TestQuoteStyleString(t *testing.T) {
	t.Parallel()

	tests := map[envi.QuoteStyle]string{
		envi.QuotePreserve: "preserve",
		envi.QuoteMinimal:  "minimal",
		envi.QuoteAlways:   "always",
		envi.QuoteStyle(9): "unknown",
	}
	for style, want := range tests {
		if got := style.String(); got != want {
			t.Errorf("QuoteStyle(%d).String() = %q, want %q", style, got, want)
		}
	}
}

func TestOrderString(t *testing.T) {
	t.Parallel()

	tests := map[envi.Order]string{
		envi.OrderSource: "source",
		envi.OrderSorted: "sorted",
		envi.Order(9):    "unknown",
	}
	for order, want := range tests {
		if got := order.String(); got != want {
			t.Errorf("Order(%d).String() = %q, want %q", order, got, want)
		}
	}
}

// The block comment template is used in both directions: it recognises a header
// while reading and writes one while encoding.
func TestWithBlockComment(t *testing.T) {
	t.Parallel()

	const custom = "# <-- Section --> #"
	opts := []envi.Option{envi.WithBlockComment("# <-- ", " --> #")}

	e, err := envi.ParseString(custom+"\nAPP_A=1\nAPP_B=2\n", opts...)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	b := e.Block("APP")
	if b == nil {
		t.Fatal("block APP missing")
	}
	if b.Comment() != "Section" {
		t.Errorf("comment = %q, want the custom template recognised", b.Comment())
	}

	// Written back with the same template, the header reappears verbatim.
	if got := encode(t, e, opts...); !strings.HasPrefix(got, custom) {
		t.Errorf("output does not start with the header:\n%s", got)
	}

	// With the template emptied, a header line is just a comment.
	plain, err := envi.ParseString(custom+"\nAPP_A=1\n", envi.WithBlockComment("", ""))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if b := plain.Block("APP"); b != nil && b.Comment() != "" {
		t.Errorf("comment = %q, want none when the template is empty", b.Comment())
	}
}

// Nonsensical option values are clamped rather than propagated.
func TestOptionValuesAreClamped(t *testing.T) {
	t.Parallel()

	e := envi.New(
		block(t, "APP", "", envi.NewRow("APP_A", "1")),
		block(t, "DB", "", envi.NewRow("DB_B", "2")),
	)

	// A negative indent behaves as zero, not as a panic or a huge run.
	if got, want := encode(t, e, envi.WithIndent(-5)), "APP_A=1\nDB_B=2\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A threshold below one behaves as one: every prefix still forms a block.
	parsed, err := envi.ParseString("APP_A=1\n", envi.WithGroupThreshold(0))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.NumBlocks() != 1 {
		t.Errorf("NumBlocks = %d, want 1", parsed.NumBlocks())
	}
}

// A nil option is ignored rather than panicking, so callers can build option
// slices conditionally.
func TestNilOptionIgnored(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("A=1\n", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, _ := e.Lookup("A"); got != "1" {
		t.Errorf("A = %q", got)
	}
}

func TestSetInlineComment(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("A=1\n")
	if err != nil {
		t.Fatal(err)
	}
	row := e.Get("A")

	if row.InlineComment() != "" {
		t.Errorf("InlineComment = %q, want empty", row.InlineComment())
	}

	row.SetInlineComment("why")
	if got := row.InlineComment(); got != "why" {
		t.Errorf("InlineComment = %q, want %q", got, "why")
	}
	if got, want := e.String(), "A=1 # why\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Setting it discards the verbatim rendering, so the comment cannot be
	// silently dropped on write.
	row.SetInlineComment("")
	if got, want := e.String(), "A=1\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseBytes(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseBytes([]byte("APP_NAME=api\n"))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if got, _ := e.Lookup("APP_NAME"); got != "api" {
		t.Errorf("APP_NAME = %q", got)
	}

	if _, err := envi.ParseBytes([]byte("broken line\n")); err == nil {
		t.Error("ParseBytes accepted a malformed document")
	}
}
