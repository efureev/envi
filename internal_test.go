package envi

// Tests in the package itself, for contracts that cannot be observed from
// outside it. Everything a consumer can reach is tested from envi_test; see
// .claude/rules/tests.md for where the line is.

import (
	"slices"
	"strings"
	"testing"
)

// foreignItem implements Item from inside the package, which is the only place
// it can be done. Outside, the unexported marker method makes the interface
// closed — and that guarantee is what the tests below pin down.
type foreignItem struct{}

func (foreignItem) Key() string { return "FOREIGN" }
func (foreignItem) sealed()     {}

func TestAddRejectsAnUnknownItemKind(t *testing.T) {
	t.Parallel()

	e := &Env{}
	err := e.Add(foreignItem{})

	if err == nil {
		t.Fatal("Add accepted an item that is neither a row nor a block")
	}
	if !strings.Contains(err.Error(), "unsupported item type") {
		t.Errorf("error = %v, want it to name the problem", err)
	}
	if e.Len() != 0 {
		t.Errorf("Len = %d, want the document left untouched", e.Len())
	}
}

func TestNewPanicsOnAnUnknownItemKind(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("New accepted an item it cannot place")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "envi: New") {
			t.Errorf("panic value = %v, want it to name New", r)
		}
	}()

	New(foreignItem{})
}

// The sort comparator must be total. Two rows cannot share a key through the
// public API, but the comparator is not allowed to depend on that.
func TestSortRowsHandlesEqualKeys(t *testing.T) {
	t.Parallel()

	first := &Row{key: "APP_A", value: "first"}
	second := &Row{key: "APP_A", value: "second"}
	b := &Block{prefix: "APP", rows: []*Row{first, second}}

	b.sortRows()

	if len(b.rows) != 2 {
		t.Fatalf("rows = %d, want both kept", len(b.rows))
	}
	// Stable sorting keeps equal elements in their original order.
	if b.rows[0] != first || b.rows[1] != second {
		t.Error("equal keys were reordered; the sort must be stable")
	}
}

// foldDuplicate carries the incoming row's shadows across. The parser cannot
// currently produce a duplicate that already has shadows, but the rule belongs
// to the merge, not to the parser, and must not quietly stop working.
func TestFoldDuplicateCarriesShadows(t *testing.T) {
	t.Parallel()

	prev := &Row{key: "K", value: "first"}
	next := &Row{key: "K", value: "second", shadows: []string{"alt-a", "alt-b"}}

	foldDuplicate(prev, next, false)

	if prev.value != "second" {
		t.Errorf("value = %q, want the later definition", prev.value)
	}
	want := []string{"first", "alt-a", "alt-b"}
	if !slices.Equal(prev.shadows, want) {
		t.Errorf("shadows = %v, want %v", prev.shadows, want)
	}
	if prev.parsed || prev.rawLine != "" {
		t.Error("a folded row must give up its verbatim rendering")
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in string
		n        int
		want     string
	}{
		{"shorter than the limit", "abc", 10, "abc"},
		{"exactly the limit", "abcde", 5, "abcde"},
		{"one over", "abcdef", 5, "abcde…"},
		{"empty", "", 5, ""},
		{"cut lands mid-rune", "ЖЖЖ", 3, "Ж…"},
		{"limit of zero", "abc", 0, "…"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := truncate(tc.in, tc.n); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}
