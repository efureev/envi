package envi_test

import (
	"os"
	"slices"
	"testing"

	envi "github.com/efureev/envi/v2"
)

func keysOf(e *envi.Env) []string {
	var out []string
	for k := range e.All() {
		out = append(out, k)
	}
	return out
}

func TestSetCreatesAndUpdates(t *testing.T) {
	t.Parallel()

	e := &envi.Env{} // the zero Env must be usable

	r := e.Set("app-name", "one")
	if r.Key() != "APP_NAME" {
		t.Errorf("key = %q, want it normalised to APP_NAME", r.Key())
	}
	if got, ok := e.Lookup("APP_NAME"); !ok || got != "one" {
		t.Errorf("Lookup = %q, %v", got, ok)
	}

	e.Set("APP_NAME", "two")
	if got, _ := e.Lookup("APP_NAME"); got != "two" {
		t.Errorf("value = %q, want it updated to two", got)
	}
	if e.Len() != 1 {
		t.Errorf("Len = %d, want 1: updating must not add a row", e.Len())
	}
}

func TestSetJoinsExistingBlock(t *testing.T) {
	t.Parallel()

	e := envi.New(block(t, "APP", "", envi.NewRow("APP_NAME", "one")))
	e.Set("APP_DEBUG", "true")

	if e.NumItems() != 1 {
		t.Errorf("NumItems = %d, want 1: the row belongs in the block", e.NumItems())
	}
	if b := e.Block("APP"); b == nil || b.Len() != 2 {
		t.Fatalf("block holds %v rows, want 2", b.Len())
	}
	if got, _ := e.Lookup("APP_DEBUG"); got != "true" {
		t.Errorf("APP_DEBUG = %q", got)
	}
}

func TestLookupMissing(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("HYPE", "false"))

	if v, ok := e.Lookup("NOPE"); ok || v != "" {
		t.Errorf("Lookup(NOPE) = %q, %v; want \"\", false", v, ok)
	}
	if e.Get("NOPE") != nil {
		t.Error("Get returned a row for an absent key")
	}
	if e.Has("NOPE") {
		t.Error("Has reported an absent key as present")
	}
	if e.Block("NOPE") != nil {
		t.Error("Block returned a block that does not exist")
	}
}

func TestDeleteCompactsAndKeepsOrder(t *testing.T) {
	t.Parallel()

	e := envi.New(
		envi.NewRow("ONE", "1"),
		envi.NewRow("TWO", "2"),
		envi.NewRow("THREE", "3"),
		envi.NewRow("FOUR", "4"),
	)

	if !e.Delete("TWO") || !e.Delete("THREE") {
		t.Fatal("Delete failed on present keys")
	}

	want := []string{"ONE", "FOUR"}
	if got := keysOf(e); !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if e.Len() != 2 {
		t.Errorf("Len = %d, want 2", e.Len())
	}
	// The surviving keys must still resolve through the rebuilt index.
	if got, _ := e.Lookup("FOUR"); got != "4" {
		t.Errorf("FOUR = %q after compaction", got)
	}
}

func TestDeleteBlock(t *testing.T) {
	t.Parallel()

	e := envi.New(
		block(t, "APP", "", envi.NewRow("APP_NAME", "one")),
		envi.NewRow("HYPE", "false"),
	)

	if !e.DeleteBlock("APP") {
		t.Fatal("DeleteBlock failed on a present block")
	}
	if e.DeleteBlock("APP") {
		t.Error("DeleteBlock reported removing an absent block")
	}
	if e.Has("APP_NAME") {
		t.Error("rows of a removed block are still reachable")
	}
	if got := keysOf(e); !slices.Equal(got, []string{"HYPE"}) {
		t.Errorf("keys = %v, want [HYPE]", got)
	}
}

func TestDeleteRowInsideBlock(t *testing.T) {
	t.Parallel()

	e := envi.New(block(t, "APP", "",
		envi.NewRow("APP_NAME", "one"),
		envi.NewRow("APP_DEBUG", "true"),
	))

	if !e.Delete("APP_NAME") {
		t.Fatal("Delete failed on a row inside a block")
	}
	if e.Has("APP_NAME") {
		t.Error("row survived deletion")
	}
	if b := e.Block("APP"); b.Len() != 1 {
		t.Errorf("block holds %d rows, want 1", b.Len())
	}
}

func TestBlockGetAcceptsBothKeyForms(t *testing.T) {
	t.Parallel()

	b := block(t, "APP", "", envi.NewRow("APP_NAME", "one"))

	if b.Get("APP_NAME") == nil {
		t.Error("full key not found")
	}
	if b.Get("NAME") == nil {
		t.Error("key relative to the prefix not found")
	}
	if b.Get("name") == nil {
		t.Error("lower-case relative key not found")
	}
	if !b.Has("NAME") {
		t.Error("Has disagrees with Get")
	}
}

func TestCounts(t *testing.T) {
	t.Parallel()

	e := envi.New(
		block(t, "APP", "", envi.NewRow("APP_A", "1"), envi.NewRow("APP_B", "2")),
		block(t, "DB", "", envi.NewRow("DB_C", "3")),
		envi.NewRow("HYPE", "false"),
	)

	if got, want := e.Len(), 4; got != want {
		t.Errorf("Len = %d, want %d", got, want)
	}
	if got, want := e.NumBlocks(), 2; got != want {
		t.Errorf("NumBlocks = %d, want %d", got, want)
	}
	if got, want := e.NumItems(), 3; got != want {
		t.Errorf("NumItems = %d, want %d", got, want)
	}
}

func TestItemsIteration(t *testing.T) {
	t.Parallel()

	e := envi.New(
		block(t, "APP", "", envi.NewRow("APP_A", "1")),
		envi.NewRow("HYPE", "false"),
	)

	var kinds []string
	for it := range e.Items() {
		switch it.(type) {
		case *envi.Block:
			kinds = append(kinds, "block:"+it.Key())
		case *envi.Row:
			kinds = append(kinds, "row:"+it.Key())
		}
	}
	want := []string{"block:APP", "row:HYPE"}
	if !slices.Equal(kinds, want) {
		t.Errorf("items = %v, want %v", kinds, want)
	}
}

func TestIteratorsStopEarly(t *testing.T) {
	t.Parallel()

	e := envi.New(
		block(t, "APP", "", envi.NewRow("APP_A", "1"), envi.NewRow("APP_B", "2")),
		envi.NewRow("HYPE", "false"),
	)

	n := 0
	for range e.Rows() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("Rows yielded %d values after break, want 1", n)
	}

	n = 0
	for range e.All() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("All yielded %d values after break, want 1", n)
	}
}

func TestRowShadows(t *testing.T) {
	t.Parallel()

	r := envi.NewRow("K", "v").AddShadow("a").AddShadow("b").AddShadow("a")

	if got, want := r.NumShadows(), 2; got != want {
		t.Errorf("NumShadows = %d, want %d: duplicates must be ignored", got, want)
	}
	if !r.HasShadow("a") || r.HasShadow("zzz") {
		t.Error("HasShadow disagrees with what was added")
	}
	var got []string
	for s := range r.Shadows() {
		got = append(got, s)
	}
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("shadows = %v, want [a b]", got)
	}
}

func TestSetValueDropsRecordedRendering(t *testing.T) {
	t.Parallel()

	// A row built in memory has no recorded rendering to begin with, so this
	// checks the observable consequence: the new value is what gets written.
	e := envi.New(envi.NewRow("K", "old"))
	e.Set("K", "new")
	if got, want := e.String(), "K=new\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMergeCopiesInsteadOfSharing(t *testing.T) {
	t.Parallel()

	dst := envi.New()
	src := envi.New(envi.NewRow("APP_NAME", "one"))

	if err := dst.Merge(src); err != nil {
		t.Fatal(err)
	}
	// Mutating the source afterwards must not reach into the destination.
	src.Set("APP_NAME", "changed")

	if got, _ := dst.Lookup("APP_NAME"); got != "one" {
		t.Errorf("destination value = %q, want it independent at one", got)
	}
}

func TestMergeNil(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("K", "v"))
	if err := e.Merge(nil); err != nil {
		t.Errorf("merging nil: %v", err)
	}
	if e.Len() != 1 {
		t.Errorf("Len = %d, want 1", e.Len())
	}
}

// Export mutates the process environment, so it cannot run in parallel.
func TestExport(t *testing.T) {
	const key = "ENVI_V2_EXPORT_TEST"

	e := envi.New(
		envi.NewRow(key, "from-file"),
		envi.NewRow(key+"_SKIPPED", "x").SetCommented(true),
	)

	t.Run("sets when absent", func(t *testing.T) {
		t.Setenv(key, "")
		os.Unsetenv(key)

		if err := e.Export(false); err != nil {
			t.Fatal(err)
		}
		if got := os.Getenv(key); got != "from-file" {
			t.Errorf("%s = %q, want from-file", key, got)
		}
		if _, ok := os.LookupEnv(key + "_SKIPPED"); ok {
			t.Error("a commented-out row must not reach the environment")
		}
	})

	t.Run("respects existing without override", func(t *testing.T) {
		t.Setenv(key, "preset")

		if err := e.Export(false); err != nil {
			t.Fatal(err)
		}
		if got := os.Getenv(key); got != "preset" {
			t.Errorf("%s = %q, want the preset value kept", key, got)
		}
	})

	t.Run("overrides when asked", func(t *testing.T) {
		t.Setenv(key, "preset")

		if err := e.Export(true); err != nil {
			t.Fatal(err)
		}
		if got := os.Getenv(key); got != "from-file" {
			t.Errorf("%s = %q, want from-file", key, got)
		}
	})
}
