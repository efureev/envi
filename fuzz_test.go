package envi_test

import (
	"errors"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// fuzzSeeds covers every construct the format has, so the fuzzer starts from
// interesting shapes rather than discovering them.
var fuzzSeeds = []string{
	readmeExample,
	"",
	"\n\n\n",
	"K=v",
	"K=v\n",
	"K = v # comment",
	"export K=v",
	"K: v",
	`K="a b"`,
	`K='a b'`,
	`K="a\"b\n\r\t\$\\"`,
	"# just a comment",
	"#K=v",
	"# K=v\nK=w",
	"###   ---[ Section ]---   ###\nAPP_A=1\nAPP_B=2",
	"A_B_C=1",
	"_LEADING=1",
	"TRAILING_=1",
	"K=\nL=",
	"K=v\nK=w",
	"a.b=1",
	"K=http://x/y?a=1#frag",
	"\xff\xfe=1",
	"K=\"unterminated",
	"K='unterminated",
	"nonsense",
}

// FuzzParse asserts the parser's contract: any input yields either a document
// or a *SyntaxError, and never a panic. Encoding whatever came back must also
// hold up.
func FuzzParse(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		e, err := envi.ParseString(in)
		if err != nil {
			var se *envi.SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("error %v (%T) is not a *SyntaxError", err, err)
			}
			if se.Line < 1 {
				t.Errorf("SyntaxError.Line = %d, want at least 1", se.Line)
			}
			_ = se.Error()
			return
		}
		if e == nil {
			t.Fatal("nil document with nil error")
		}

		// Every accessor must survive whatever was parsed.
		_ = e.String()
		_ = e.Len()
		_ = e.NumItems()
		_ = e.NumBlocks()
		for k, v := range e.All() {
			if got, ok := e.Lookup(k); !ok || got != v {
				t.Fatalf("Lookup(%q) = %q, %v; iteration gave %q", k, got, ok, v)
			}
		}
	})
}

// FuzzRoundTrip asserts that encoding is a fixed point: whatever the parser
// accepts, its own output must parse again and encode identically.
func FuzzRoundTrip(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		first, err := envi.ParseString(in)
		if err != nil {
			t.Skip() // malformed input is FuzzParse's business
		}

		once := first.String()
		second, err := envi.ParseString(once)
		if err != nil {
			t.Fatalf("our own output does not parse: %v\noutput:\n%q", err, once)
		}

		if twice := second.String(); twice != once {
			t.Errorf("encoding is not idempotent\nfirst:  %q\nsecond: %q", once, twice)
		}
	})
}

// FuzzRoundTripRewritten does the same for documents whose rendering has been
// recomputed rather than reproduced, which exercises the quoting rules instead
// of the verbatim path.
func FuzzRoundTripRewritten(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		first, err := envi.ParseString(in)
		if err != nil {
			t.Skip()
		}

		// Re-setting every value drops the recorded rendering, forcing the
		// encoder to quote from scratch.
		for k, v := range first.All() {
			first.Set(k, v)
		}

		once := first.String()
		second, err := envi.ParseString(once)
		if err != nil {
			t.Fatalf("rewritten output does not parse: %v\noutput:\n%q", err, once)
		}

		for k, v := range first.All() {
			got, ok := second.Lookup(k)
			if !ok {
				t.Fatalf("key %q lost when rewritten output was reparsed\noutput:\n%q", k, once)
			}
			if got != v {
				t.Fatalf("value for %q changed: %q became %q\noutput:\n%q", k, v, got, once)
			}
		}
	})
}
