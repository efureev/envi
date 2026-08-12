package envi_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// wideBlock returns a block holding n rows, which for n above the internal
// threshold exercises the hashed lookup path rather than the linear scan.
func wideBlock(t *testing.T, n int) *envi.Block {
	t.Helper()
	b := envi.NewBlock("APP")
	rows := make([]*envi.Row, n)
	for i := range n {
		rows[i] = envi.NewRow(fmt.Sprintf("APP_K%02d", i), fmt.Sprintf("v%d", i))
	}
	if err := b.Add(rows...); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return b
}

// A block switches from scanning to hashing once it outgrows the threshold.
// Both sides of that switch must behave identically.
func TestBlockLookupAcrossIndexThreshold(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 8, 9, 40} {
		t.Run(fmt.Sprintf("rows=%d", n), func(t *testing.T) {
			t.Parallel()
			b := wideBlock(t, n)

			if b.Len() != n {
				t.Fatalf("Len = %d, want %d", b.Len(), n)
			}
			for i := range n {
				full := fmt.Sprintf("APP_K%02d", i)
				short := fmt.Sprintf("K%02d", i)

				if r := b.Get(full); r == nil || r.Value() != fmt.Sprintf("v%d", i) {
					t.Errorf("Get(%s) = %v", full, r)
				}
				if !b.Has(short) {
					t.Errorf("Has(%s) = false", short)
				}
			}
			if b.Get("APP_MISSING") != nil || b.Has("MISSING") {
				t.Error("an absent key was reported present")
			}
		})
	}
}

// Growing past the threshold mid-flight must keep earlier rows reachable: the
// index is built from what is already there, not only from what arrives after.
func TestBlockIndexBuiltFromExistingRows(t *testing.T) {
	t.Parallel()

	b := envi.NewBlock("APP")
	for i := range 20 {
		if err := b.Add(envi.NewRow(fmt.Sprintf("APP_K%02d", i), "v")); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
		// Every row added so far stays reachable, on both sides of the switch.
		for j := 0; j <= i; j++ {
			if b.Get(fmt.Sprintf("APP_K%02d", j)) == nil {
				t.Fatalf("after adding %d rows, K%02d became unreachable", i+1, j)
			}
		}
	}
}

// Shrinking back below the threshold drops the index; lookups must survive it.
func TestBlockDeleteAcrossIndexThreshold(t *testing.T) {
	t.Parallel()

	b := wideBlock(t, 12)

	for i := range 8 {
		if !b.Delete(fmt.Sprintf("APP_K%02d", i)) {
			t.Fatalf("Delete K%02d reported nothing removed", i)
		}
	}
	if b.Len() != 4 {
		t.Fatalf("Len = %d, want 4", b.Len())
	}
	for i := 8; i < 12; i++ {
		if b.Get(fmt.Sprintf("APP_K%02d", i)) == nil {
			t.Errorf("K%02d unreachable after the index was dropped", i)
		}
	}
	if b.Delete("APP_K00") {
		t.Error("Delete reported removing an already removed key")
	}
}

func TestBlockPrefixAndRows(t *testing.T) {
	t.Parallel()

	b := wideBlock(t, 3)

	if b.Prefix() != "APP" || b.Key() != "APP" {
		t.Errorf("Prefix = %q, Key = %q, want APP", b.Prefix(), b.Key())
	}

	var got []string
	for r := range b.Rows() {
		got = append(got, r.Key())
	}
	want := []string{"APP_K00", "APP_K01", "APP_K02"}
	if !slices.Equal(got, want) {
		t.Errorf("Rows = %v, want %v", got, want)
	}

	// Iteration must honour an early exit.
	n := 0
	for range b.Rows() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("Rows yielded %d values after break, want 1", n)
	}
}

func TestBlockAddIgnoresNilAndMergesDuplicates(t *testing.T) {
	t.Parallel()

	b := envi.NewBlock("APP")
	if err := b.Add(nil, envi.NewRow("APP_NAME", "one"), nil); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if b.Len() != 1 {
		t.Fatalf("Len = %d, want 1: nil rows must be skipped", b.Len())
	}

	// A repeat of the same key merges rather than duplicating.
	if err := b.Add(envi.NewRow("APP_NAME", "two")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if b.Len() != 1 {
		t.Errorf("Len = %d, want 1 after re-adding the same key", b.Len())
	}
	if got := b.Get("NAME").Value(); got != "two" {
		t.Errorf("value = %q, want the later definition", got)
	}
}

// Merging documents clones blocks and their rows; the copies must be
// independent, including in a block large enough to be indexed.
func TestMergeClonesWideBlock(t *testing.T) {
	t.Parallel()

	src := envi.New(wideBlock(t, 12))
	dst := envi.New()

	if err := dst.Merge(src); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if dst.Len() != 12 {
		t.Fatalf("Len = %d, want 12", dst.Len())
	}
	if got, _ := dst.Lookup("APP_K05"); got != "v5" {
		t.Errorf("APP_K05 = %q", got)
	}

	// Mutating the source must not reach the destination.
	src.Set("APP_K05", "changed")
	if got, _ := dst.Lookup("APP_K05"); got != "v5" {
		t.Errorf("APP_K05 = %q after the source changed, want it independent", got)
	}
}

// Merging into an existing block of the same prefix adds what is missing and
// merges what is present, keeping the destination's index correct.
func TestMergeIntoExistingBlock(t *testing.T) {
	t.Parallel()

	dst := envi.New(block(t, "APP", "", envi.NewRow("APP_A", "1")))
	src := envi.New(block(t, "APP", "section", envi.NewRow("APP_A", "2"), envi.NewRow("APP_B", "3")))

	if err := dst.Merge(src); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if got, _ := dst.Lookup("APP_A"); got != "2" {
		t.Errorf("APP_A = %q, want the override", got)
	}
	if got, _ := dst.Lookup("APP_B"); got != "3" {
		t.Errorf("APP_B = %q, want it added", got)
	}
	if b := dst.Block("APP"); b == nil || b.Comment() != "section" {
		t.Errorf("comment = %q, want it taken from the source", b.Comment())
	}
	if dst.NumItems() != 1 {
		t.Errorf("NumItems = %d, want 1", dst.NumItems())
	}
}

// SortByKey orders the rows inside a block too, and a wide block must keep its
// index consistent with the new positions.
func TestSortByKeySortsInsideWideBlock(t *testing.T) {
	t.Parallel()

	b := envi.NewBlock("APP")
	for i := 11; i >= 0; i-- { // added in reverse
		if err := b.Add(envi.NewRow(fmt.Sprintf("APP_K%02d", i), fmt.Sprintf("v%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	e := envi.New(b)

	e.SortByKey()

	var keys []string
	for k := range e.All() {
		keys = append(keys, k)
	}
	if !slices.IsSorted(keys) {
		t.Errorf("keys not sorted: %v", keys)
	}
	// Lookup must still resolve after everything moved.
	for i := range 12 {
		key := fmt.Sprintf("APP_K%02d", i)
		if got, _ := e.Lookup(key); got != fmt.Sprintf("v%d", i) {
			t.Errorf("%s = %q after sorting", key, got)
		}
	}
}

// A block whose rows are all commented out writes nothing when commented rows
// are excluded — including the header it would otherwise introduce.
func TestBlockWithOnlyCommentedRowsIsSkipped(t *testing.T) {
	t.Parallel()

	e := envi.New(block(t, "APP", "section",
		envi.NewRow("APP_A", "1").SetCommented(true),
		envi.NewRow("APP_B", "2").SetCommented(true),
	))

	got := encode(t, e, envi.WithCommentedRows(false))
	if strings.Contains(got, "section") {
		t.Errorf("header written for a block with no visible rows:\n%s", got)
	}
}
