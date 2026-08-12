package envi_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// diffOf parses both sides and compares them, failing the test if either does
// not parse.
func diffOf(t *testing.T, a, b string) *envi.Delta {
	t.Helper()

	left, err := envi.ParseString(a)
	if err != nil {
		t.Fatalf("parsing the left side: %v", err)
	}
	right, err := envi.ParseString(b)
	if err != nil {
		t.Fatalf("parsing the right side: %v", err)
	}
	return left.Diff(right)
}

// linesOfDelta renders the comparison as the lines Text writes.
func linesOfDelta(d *envi.Delta) []string {
	s := d.String()
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func TestDiffKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want []string
	}{
		{"identical", "A=1\nB=2\n", "A=1\nB=2\n", nil},
		{"added", "A=1\n", "A=1\nB=2\n", []string{`+ B="2"`}},
		{"removed", "A=1\nB=2\n", "A=1\n", []string{`- B="2"`}},
		{"changed", "A=1\n", "A=2\n", []string{`~ A: "1" -> "2"`}},
		{
			"all three at once",
			"A=1\nB=2\nC=3\n",
			"A=1\nB=9\nD=4\n",
			[]string{`~ B: "2" -> "9"`, `- C="3"`, `+ D="4"`},
		},
		{"empty to empty", "", "", nil},
		{"empty to populated", "", "A=1\n", []string{`+ A="1"`}},
		{"populated to empty", "A=1\n", "", []string{`- A="1"`}},
		{"value becomes empty", "A=1\n", "A=\n", []string{`~ A: "1" -> ""`}},
		{"value gains spaces", "A=x\n", `A=" x "` + "\n", []string{`~ A: "x" -> " x "`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := diffOf(t, tt.a, tt.b)
			if got := linesOfDelta(d); !slices.Equal(got, tt.want) {
				t.Errorf("diff =\n%v\nwant\n%v", got, tt.want)
			}
			if d.Empty() != (len(tt.want) == 0) {
				t.Errorf("Empty = %v, want %v", d.Empty(), len(tt.want) == 0)
			}
			if d.Len() != len(tt.want) {
				t.Errorf("Len = %d, want %d", d.Len(), len(tt.want))
			}
		})
	}
}

// The decision this test pins: a commented-out row configures nothing, the view
// Env.Export takes. It is deliberately not the view Env.Lookup takes, which
// hands back a commented row's value — so this must not be "fixed" to agree
// with Lookup without changing the documented contract.
func TestDiffTreatsCommentedRowsAsAbsent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want []string
	}{
		{"live becomes commented", "K=1\n", "# K=1\n", []string{`- K="1"`}},
		{"commented becomes live", "# K=1\n", "K=1\n", []string{`+ K="1"`}},
		{"commented on both sides, same value", "# K=1\n", "# K=1\n", nil},
		{"commented on both sides, different values", "# K=1\n", "# K=2\n", nil},
		{"commented only on the left", "# K=1\nA=1\n", "A=1\n", nil},
		{"commented only on the right", "A=1\n", "# K=1\nA=1\n", nil},
		{
			"live one side, commented the other, different values",
			"K=1\n",
			"# K=2\n",
			[]string{`- K="1"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := linesOfDelta(diffOf(t, tt.a, tt.b)); !slices.Equal(got, tt.want) {
				t.Errorf("diff =\n%v\nwant\n%v", got, tt.want)
			}
		})
	}
}

// Only values are compared: everything about how the document is written is
// out of scope, and saying otherwise would make the result noise in CI.
func TestDiffIgnoresEverythingButValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
	}{
		{"comment added", "K=1\n", "# a note\nK=1\n"},
		{"comment changed", "# before\nK=1\n", "# after\nK=1\n"},
		{"trailing comment", "K=1\n", "K=1 # inline\n"},
		{"shadow added", "K=1\n", "# K=9\nK=1\n"},
		{"quote style", "K=1\n", `K="1"` + "\n"},
		{"spacing", "K=1\n", "K = 1\n"},
		{"export prefix", "K=1\n", "export K=1\n"},
		{"key case", "k=1\n", "K=1\n"},
		{"blank lines", "A=1\nB=2\n", "A=1\n\n\nB=2\n"},
		{"block header", "APP_A=1\nAPP_B=2\n", "###   ---[ App ]---   ###\nAPP_A=1\nAPP_B=2\n"},
		{"row order", "A=1\nB=2\n", "B=2\nA=1\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if d := diffOf(t, tt.a, tt.b); !d.Empty() {
				t.Errorf("diff reported %d changes, want none:\n%s", d.Len(), d)
			}
		})
	}
}

// Regrouping rearranges a document without changing what it configures, so it
// must be invisible here.
func TestDiffIgnoresBlockMembership(t *testing.T) {
	t.Parallel()

	const src = "APP_NAME=one\nZED=x\nAPP_DEBUG=false\n"

	before, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	after, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	after.Regroup()

	if d := before.Diff(after); !d.Empty() {
		t.Errorf("regrouping produced %d changes, want none:\n%s", d.Len(), d)
	}
}

func TestDiffOrder(t *testing.T) {
	t.Parallel()

	// Removals and changes follow the left document; additions follow the right.
	d := diffOf(t,
		"ZED=1\nALPHA=2\nMIKE=3\n",
		"ALPHA=9\nYANKEE=4\nBRAVO=5\n")

	want := []string{
		`- ZED="1"`,
		`~ ALPHA: "2" -> "9"`,
		`- MIKE="3"`,
		`+ YANKEE="4"`,
		`+ BRAVO="5"`,
	}
	if got := linesOfDelta(d); !slices.Equal(got, want) {
		t.Errorf("order =\n%v\nwant\n%v", got, want)
	}
}

func TestDiffCounts(t *testing.T) {
	t.Parallel()

	d := diffOf(t, "A=1\nB=2\nC=3\n", "A=1\nB=9\nD=4\nE=5\n")

	tests := []struct {
		kind envi.ChangeKind
		want int
	}{
		{envi.ChangeAdded, 2},
		{envi.ChangeRemoved, 1},
		{envi.ChangeChanged, 1},
	}
	for _, tt := range tests {
		if got := d.Count(tt.kind); got != tt.want {
			t.Errorf("Count(%v) = %d, want %d", tt.kind, got, tt.want)
		}
	}
	if d.Len() != 4 {
		t.Errorf("Len = %d, want 4", d.Len())
	}

	var keys []string
	for c := range d.All() {
		keys = append(keys, c.Key)
	}
	if want := []string{"B", "C", "D", "E"}; !slices.Equal(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
}

func TestDiffNil(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("K=1\n")
	if err != nil {
		t.Fatal(err)
	}
	var absent *envi.Env

	t.Run("against nil everything is removed", func(t *testing.T) {
		t.Parallel()
		if got := linesOfDelta(e.Diff(nil)); !slices.Equal(got, []string{`- K="1"`}) {
			t.Errorf("diff = %v, want [- K=\"1\"]", got)
		}
	})

	t.Run("from nil everything is added", func(t *testing.T) {
		t.Parallel()
		if got := linesOfDelta(absent.Diff(e)); !slices.Equal(got, []string{`+ K="1"`}) {
			t.Errorf("diff = %v, want [+ K=\"1\"]", got)
		}
	})

	t.Run("nil against nil is empty", func(t *testing.T) {
		t.Parallel()
		if d := absent.Diff(nil); !d.Empty() {
			t.Errorf("Len = %d, want 0", d.Len())
		}
	})
}

func TestZeroDeltaIsUsable(t *testing.T) {
	t.Parallel()

	var d envi.Delta

	if !d.Empty() {
		t.Error("Empty = false, want true")
	}
	if d.Len() != 0 {
		t.Errorf("Len = %d, want 0", d.Len())
	}
	if d.Count(envi.ChangeAdded) != 0 {
		t.Errorf("Count = %d, want 0", d.Count(envi.ChangeAdded))
	}
	if got := d.String(); got != "" {
		t.Errorf("String = %q, want empty", got)
	}
	for range d.All() {
		t.Error("All yielded a change from an empty Delta")
	}
}

func TestDeltaJSON(t *testing.T) {
	t.Parallel()

	d := diffOf(t, "A=1\nB=2\n", "A=9\nC=3\n")

	var b strings.Builder
	if err := d.JSON(&b); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got []envi.Change
	if err := json.Unmarshal([]byte(b.String()), &got); err != nil {
		t.Fatalf("the output does not parse back: %v\n%s", err, b.String())
	}
	if want := slices.Collect(d.All()); !slices.Equal(got, want) {
		t.Errorf("round trip changed the comparison:\ngot  %+v\nwant %+v", got, want)
	}
	if !strings.Contains(b.String(), `"kind": "changed"`) {
		t.Errorf("kind is not written as a name:\n%s", b.String())
	}
}

func TestDeltaJSONOnEmptyDeltaWritesAnArray(t *testing.T) {
	t.Parallel()

	var d envi.Delta
	var b strings.Builder
	if err := d.JSON(&b); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got := strings.TrimSpace(b.String()); got != "[]" {
		t.Errorf("JSON = %q, want []", got)
	}
}

func TestDeltaReportsWriteErrors(t *testing.T) {
	t.Parallel()

	d := diffOf(t, "A=1\n", "A=2\n")
	want := errors.New("disk on fire")

	tests := []struct {
		name string
		run  func() error
	}{
		{"Text", func() error { return d.Text(failingWriter{err: want}) }},
		{"Text terminator", func() error { return d.Text(&flakyWriter{ok: 1, err: want}) }},
		{"JSON", func() error { return d.JSON(failingWriter{err: want}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.run(); !errors.Is(got, want) {
				t.Errorf("error = %v, want %v", got, want)
			}
		})
	}
}

func TestChangeKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   envi.ChangeKind
		want string
	}{
		{envi.ChangeAdded, "added"},
		{envi.ChangeRemoved, "removed"},
		{envi.ChangeChanged, "changed"},
		{envi.ChangeKind(42), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("ChangeKind(%d).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestChangeKindJSON(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()

		for _, k := range []envi.ChangeKind{envi.ChangeAdded, envi.ChangeRemoved, envi.ChangeChanged} {
			data, err := json.Marshal(k)
			if err != nil {
				t.Fatalf("Marshal(%v): %v", k, err)
			}
			if want := `"` + k.String() + `"`; string(data) != want {
				t.Errorf("Marshal = %s, want %s", data, want)
			}
			var back envi.ChangeKind
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("Unmarshal(%s): %v", data, err)
			}
			if back != k {
				t.Errorf("round trip = %v, want %v", back, k)
			}
		}
	})

	t.Run("nonsense is rejected", func(t *testing.T) {
		t.Parallel()

		for _, in := range []string{`"vanished"`, `7`} {
			var k envi.ChangeKind
			if err := json.Unmarshal([]byte(in), &k); err == nil {
				t.Errorf("Unmarshal(%s) = nil error, want one", in)
			}
		}
	})
}

func TestChangeStringOnUnknownKind(t *testing.T) {
	t.Parallel()

	c := envi.Change{Kind: envi.ChangeKind(42), Key: "K"}
	if got, want := c.String(), "unknown K"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

// The use case the feature was asked for: checking a working .env against the
// example that documents it.
func TestDiffChecksAnExampleFile(t *testing.T) {
	t.Parallel()

	const example = `APP_NAME=changeme
APP_PORT=8080
DB_HOST=localhost
`
	const actual = `APP_NAME=my-app
APP_PORT=9090
EXTRA=surprise
`

	d := diffOf(t, example, actual)

	var missing, extra []string
	for c := range d.All() {
		switch c.Kind {
		case envi.ChangeRemoved:
			missing = append(missing, c.Key) // in the example, absent from .env
		case envi.ChangeAdded:
			extra = append(extra, c.Key) // in .env, undocumented
		case envi.ChangeChanged:
			// A different value is the point of a real .env, not a problem.
		}
	}

	if want := []string{"DB_HOST"}; !slices.Equal(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
	if want := []string{"EXTRA"}; !slices.Equal(extra, want) {
		t.Errorf("extra = %v, want %v", extra, want)
	}
}
