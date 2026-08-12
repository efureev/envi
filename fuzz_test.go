package envi_test

import (
	"errors"
	"maps"
	"slices"
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
	"# K=1\n# K=2\n",
	"K=1\n# K=2\n",
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

// FuzzModelSurvivesEncoding asserts that writing a document says everything the
// document holds: reading the output back must give the same rows, with the
// same values, the same commented flags and the same shadows.
//
// This is the property FuzzRoundTrip cannot see. That target compares our own
// output against our own output, so anything the encoder drops consistently
// looks like a fixed point — which is exactly how a repeated commented key came
// to lose a line and go unnoticed. Byte identity is deliberately not asserted:
// a key stated twice legitimately comes back rearranged. Nothing may vanish.
func FuzzModelSurvivesEncoding(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		first, err := envi.ParseString(in)
		if err != nil {
			t.Skip() // malformed input is FuzzParse's business
		}

		out := first.String()
		second, err := envi.ParseString(out)
		if err != nil {
			t.Fatalf("our own output does not parse: %v\noutput:\n%q", err, out)
		}

		want := slices.Collect(first.Rows())
		got := slices.Collect(second.Rows())
		if len(got) != len(want) {
			t.Fatalf("row count %d became %d\ninput:\n%q\noutput:\n%q", len(want), len(got), in, out)
		}

		for i, w := range want {
			g := got[i]
			switch {
			case g.Key() != w.Key():
				t.Fatalf("row %d: key %q became %q\noutput:\n%q", i, w.Key(), g.Key(), out)
			case g.Value() != w.Value():
				t.Fatalf("row %d (%s): value %q became %q\noutput:\n%q", i, w.Key(), w.Value(), g.Value(), out)
			case g.IsCommented() != w.IsCommented():
				t.Fatalf("row %d (%s): commented %v became %v\noutput:\n%q", i, w.Key(), w.IsCommented(), g.IsCommented(), out)
			}
			ws := slices.Collect(w.Shadows())
			gs := slices.Collect(g.Shadows())
			if !slices.Equal(gs, ws) {
				t.Fatalf("row %d (%s): shadows %v became %v\ninput:\n%q\noutput:\n%q",
					i, w.Key(), ws, gs, in, out)
			}
		}
	})
}

// FuzzCheck asserts the checker's contract: it accepts anything, never panics,
// and agrees with the parser about what is valid. The last part is what keeps
// the recovering read and the fail-fast one from drifting apart — the two share
// a scanner, and only one of them is exercised by the other fuzz targets.
func FuzzCheck(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		env, rep, err := envi.CheckString(in)
		if err != nil {
			t.Fatalf("Check returned an error for a string reader: %v", err)
		}
		if env == nil || rep == nil {
			t.Fatal("Check returned nothing without an error")
		}

		_ = rep.String()
		_ = rep.Len()
		_ = rep.Err()
		for p := range rep.All() {
			if p.Rule == "" {
				t.Errorf("finding with no rule: %+v", p)
			}
			if p.Line < 0 || p.Col < 0 {
				t.Errorf("finding with a negative position: %+v", p)
			}
		}

		// The two reads must agree about what the format allows. Only the syntax
		// rule speaks to that: the parser is happy to fold a duplicate key,
		// which the checker reports but which is not a parse failure.
		syntax := 0
		for p := range rep.All() {
			if p.Rule == envi.RuleSyntax {
				syntax++
			}
		}
		_, perr := envi.ParseString(in)
		if (syntax == 0) != (perr == nil) {
			t.Fatalf("%d syntax findings but Parse error = %v\nreport:\n%s", syntax, perr, rep)
		}
		if perr != nil && syntax < 1 {
			t.Fatalf("Parse failed with %v but the checker found no syntax problem", perr)
		}
	})
}

// FuzzRegroup asserts that tidying rearranges a document without changing what
// it says, and that it settles: regrouping twice is regrouping once, and the
// output regroups to itself.
func FuzzRegroup(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		for _, tc := range []struct {
			name string
			tidy func(*envi.Env)
		}{
			{"Regroup", func(e *envi.Env) { e.Regroup() }},
			{"Tidy", func(e *envi.Env) { e.Tidy() }},
		} {
			e, err := envi.ParseString(in)
			if err != nil {
				t.Skip() // malformed input is FuzzParse's business
			}

			want := map[string]string{}
			for k, v := range e.All() {
				want[k] = v
			}

			tc.tidy(e)

			got := map[string]string{}
			for k, v := range e.All() {
				got[k] = v
			}
			if len(got) != len(want) {
				t.Fatalf("%s changed the key count: %d became %d", tc.name, len(want), len(got))
			}
			for k, v := range want {
				if have, ok := got[k]; !ok || have != v {
					t.Fatalf("%s changed %q: %q became %q (present: %v)", tc.name, k, v, have, ok)
				}
			}
			for k := range got {
				if e.Get(k) == nil {
					t.Fatalf("%s left %q unreachable through Get", tc.name, k)
				}
			}

			once := e.String()
			tc.tidy(e)
			if twice := e.String(); twice != once {
				t.Fatalf("%s is not idempotent\nfirst:  %q\nsecond: %q", tc.name, once, twice)
			}

			// The output must come back as the same document. Byte identity is
			// deliberately not asserted here: reproducing input line for line
			// is the parser's property, tested by FuzzRoundTrip, and a tidied
			// document has given it up by design.
			again, err := envi.ParseString(once)
			if err != nil {
				t.Fatalf("%s output does not parse: %v\noutput:\n%q", tc.name, err, once)
			}
			for k, v := range got {
				if have, ok := again.Lookup(k); !ok || have != v {
					t.Fatalf("%s output lost %q=%q on the way back (got %q, present %v)\noutput:\n%q",
						tc.name, k, v, have, ok, once)
				}
			}
			tc.tidy(again)
			for k, v := range got {
				if have, ok := again.Lookup(k); !ok || have != v {
					t.Fatalf("%s does not settle: %q=%q became %q (present %v)", tc.name, k, v, have, ok)
				}
			}
		}
	})
}

// FuzzDiff asserts that a comparison is complete and points the right way:
// applying what it reports to the left document must make it configure exactly
// what the right one does.
//
// That closes the loop in one property. A diff that misses a change leaves the
// documents disagreeing; one that reports a change backwards moves the left
// document away from the right; one that invents a change shows up as a
// leftover on the second comparison.
func FuzzDiff(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s, s)
	}
	f.Add("A=1\nB=2\n", "A=9\nC=3\n")
	f.Add("# K=1\n", "K=1\n")
	f.Add("K=1\n", "# K=1\n")

	f.Fuzz(func(t *testing.T, left, right string) {
		a, err := envi.ParseString(left)
		if err != nil {
			t.Skip() // malformed input is FuzzParse's business
		}
		b, err := envi.ParseString(right)
		if err != nil {
			t.Skip()
		}

		// A document never differs from itself.
		if d := a.Diff(a); !d.Empty() {
			t.Fatalf("a document differs from itself:\n%s", d)
		}

		d := a.Diff(b)
		for c := range d.All() {
			if c.Key == "" {
				t.Fatalf("change with no key: %+v", c)
			}
			_ = c.String()
		}

		// Apply the comparison to a fresh copy of the left document.
		applied, err := envi.ParseString(left)
		if err != nil {
			t.Fatal(err)
		}
		for c := range d.All() {
			switch c.Kind {
			case envi.ChangeAdded, envi.ChangeChanged:
				// SetCommented(false) is not decoration. Env.Set edits whatever
				// row it finds, and Row.SetValue leaves the commented flag
				// alone, so setting a key whose only row is "# KEY=old" stores
				// the value in a row that still configures nothing. Applying a
				// change means making the key configured.
				applied.Set(c.Key, c.New).SetCommented(false)
			case envi.ChangeRemoved:
				applied.Delete(c.Key)
			}
		}

		// Both must now configure the same thing, and comparing again must
		// find nothing left to do.
		if again := applied.Diff(b); !again.Empty() {
			t.Fatalf("applying the comparison did not close the gap\nleft:  %q\nright: %q\nfirst:\n%s\nleft over:\n%s",
				left, right, d, again)
		}
		if want, got := configuredOf(b), configuredOf(applied); !maps.Equal(got, want) {
			t.Fatalf("configuration differs after applying\nleft:  %q\nright: %q\ngot  %v\nwant %v",
				left, right, got, want)
		}
	})
}

// configuredOf collects the keys a document actually configures, which is what
// Diff compares: a commented-out row sets nothing.
func configuredOf(e *envi.Env) map[string]string {
	out := map[string]string{}
	for r := range e.Rows() {
		if !r.IsCommented() {
			out[r.Key()] = r.Value()
		}
	}
	return out
}
