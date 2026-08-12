package envi_test

import (
	"errors"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// Regressions for the defects listed under Fixed in the 2.0.0 entry of
// CHANGELOG.md. Each test names the finding it closes, so the defect and the
// proof that it stays closed can be read together.

// C1: merging a key absent from the receiver used to panic.
func TestRegressionC1_MergeMissingKey(t *testing.T) {
	t.Parallel()

	dst := envi.New(envi.NewRow("APP_NAME", "one"))
	src := envi.New(envi.NewRow("HYPE", "false"))

	if err := dst.Merge(src); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	got, ok := dst.Lookup("HYPE")
	if !ok {
		t.Fatal("missing key was not added by Merge")
	}
	if got != "false" {
		t.Errorf("HYPE = %q, want %q", got, "false")
	}
	if _, ok := dst.Lookup("APP_NAME"); !ok {
		t.Error("Merge dropped the pre-existing key")
	}
}

// C2: deleting a key whose block does not exist used to panic.
func TestRegressionC2_DeleteWithoutBlock(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("HYPE", "false"))

	if e.Delete("APP_NAME") {
		t.Error("Delete reported removing a key that was never there")
	}
	if !e.Delete("HYPE") {
		t.Error("Delete failed to remove an existing key")
	}
	if e.Has("HYPE") {
		t.Error("key survived deletion")
	}
}

// C3: merging documents whose keys differ only in case used to lose the earlier
// comment, because the index was written under one form of the key and read
// under another.
func TestRegressionC3_MergePreservesCommentAcrossCase(t *testing.T) {
	t.Parallel()

	dst := envi.New(envi.NewRow("APP_NAME", "one").SetComment("comment from file 1"))
	src := envi.New(envi.NewRow("app_name", "two"))

	if err := dst.Merge(src); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	r := dst.Get("APP_NAME")
	if r == nil {
		t.Fatal("key lost during merge")
	}
	if r.Value() != "two" {
		t.Errorf("value = %q, want the overriding %q", r.Value(), "two")
	}
	if r.Comment() != "comment from file 1" {
		t.Errorf("comment = %q, want it preserved", r.Comment())
	}
	if n := dst.Len(); n != 1 {
		t.Errorf("Len = %d, want 1: the differing case must not create a second row", n)
	}
}

// C4: a row whose key did not carry the block prefix used to vanish silently.
func TestRegressionC4_PrefixMismatchIsAnError(t *testing.T) {
	t.Parallel()

	b := envi.NewBlock("APP")
	err := b.Add(envi.NewRow("OTHER_KEY", "value"))

	if err == nil {
		t.Fatal("Add accepted a row from another block")
	}
	if !errors.Is(err, envi.ErrPrefixMismatch) {
		t.Errorf("error = %v, want it to wrap ErrPrefixMismatch", err)
	}
	if b.Len() != 0 {
		t.Errorf("Len = %d, want 0", b.Len())
	}
}

// H1: a top-level row used to become unreachable once a block of the same
// prefix existed. Adding the block now adopts the row instead.
func TestRegressionH1_BlockAdoptsTopLevelRows(t *testing.T) {
	t.Parallel()

	e := envi.New(
		envi.NewRow("APP_NAME", "one"),
		envi.NewRow("APP_DEBUG", "true"),
	)
	if e.Get("APP_NAME") == nil {
		t.Fatal("precondition: the row must be reachable before the block exists")
	}

	if err := e.Add(envi.NewBlock("APP")); err != nil {
		t.Fatalf("Add block: %v", err)
	}

	r := e.Get("APP_NAME")
	if r == nil {
		t.Fatal("row became unreachable after its block was added")
	}
	if r.Value() != "one" {
		t.Errorf("value = %q, want %q", r.Value(), "one")
	}

	b := e.Block("APP")
	if b == nil {
		t.Fatal("block missing")
	}
	if b.Len() != 2 {
		t.Errorf("block holds %d rows, want 2 adopted", b.Len())
	}
	if e.NumItems() != 1 {
		t.Errorf("NumItems = %d, want 1: adopted rows must leave top level", e.NumItems())
	}
}

// H2: an empty incoming value used to erase a set one.
func TestRegressionH2_EmptyValueDoesNotErase(t *testing.T) {
	t.Parallel()

	dst := envi.New(envi.NewRow("HYPE", "false"))
	src := envi.New(envi.NewRow("HYPE", ""))

	if err := dst.Merge(src); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if got, _ := dst.Lookup("HYPE"); got != "false" {
		t.Errorf("HYPE = %q, want the original %q kept", got, "false")
	}
}

// H3: reading a document used to reorder it.
func TestRegressionH3_OrderIsPreserved(t *testing.T) {
	t.Parallel()

	e := envi.New(
		envi.NewRow("ZULU", "1"),
		envi.NewRow("ALPHA", "2"),
		envi.NewRow("MIKE", "3"),
	)

	var got []string
	for k := range e.All() {
		got = append(got, k)
	}
	want := []string{"ZULU", "ALPHA", "MIKE"}
	if !equal(got, want) {
		t.Errorf("order = %v, want insertion order %v", got, want)
	}

	e.SortByKey()
	got = got[:0]
	for k := range e.All() {
		got = append(got, k)
	}
	want = []string{"ALPHA", "MIKE", "ZULU"}
	if !equal(got, want) {
		t.Errorf("after SortByKey order = %v, want %v", got, want)
	}
}

// H5: configuration used to live in package globals, so concurrent use raced.
// Encoders now carry their own configuration; run under -race.
func TestRegressionH5_ConcurrentEncodersDoNotRace(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("APP_NAME", "one"), envi.NewRow("APP_DEBUG", "true"))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 500 {
			_ = e.String()
		}
	}()
	for range 500 {
		var sb capturingWriter
		if err := envi.NewEncoder(&sb, envi.WithIndent(2)).Encode(e); err != nil {
			t.Error(err)
			return
		}
	}
	<-done
}

type capturingWriter struct{ n int }

func (c *capturingWriter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
