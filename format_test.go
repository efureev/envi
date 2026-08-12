package envi_test

import (
	"slices"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// layoutOf describes the document's structure: one entry per top-level item,
// a block written as "PREFIX[A,B]" and a loose row as its key.
func layoutOf(e *envi.Env) []string {
	var got []string
	for it := range e.Items() {
		switch v := it.(type) {
		case *envi.Row:
			got = append(got, v.Key())
		case *envi.Block:
			var keys []string
			for r := range v.Rows() {
				keys = append(keys, r.Key())
			}
			got = append(got, v.Prefix()+"["+strings.Join(keys, ",")+"]")
		}
	}
	return got
}

func TestRegroupGathersScatteredPrefixes(t *testing.T) {
	t.Parallel()

	const src = `APP_NAME=one
DB_HOST=localhost
APP_DEBUG=false
DB_PORT=5432
LONE=x
APP_URL=https://example.com
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	e.Regroup()

	want := []string{"APP[APP_NAME,APP_DEBUG,APP_URL]", "DB[DB_HOST,DB_PORT]", "LONE"}
	if got := layoutOf(e); !slices.Equal(got, want) {
		t.Errorf("layout = %v, want %v", got, want)
	}
}

func TestRegroupKeepsEveryRow(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	before := keysOf(e)
	n := e.Len()

	e.Regroup()

	if e.Len() != n {
		t.Errorf("Len = %d, want %d", e.Len(), n)
	}
	after := keysOf(e)
	slices.Sort(before)
	slices.Sort(after)
	if !slices.Equal(before, after) {
		t.Errorf("keys = %v, want %v", after, before)
	}
	for _, k := range before {
		if e.Get(k) == nil {
			t.Errorf("Get(%q) = nil after Regroup", k)
		}
	}
}

func TestRegroupPreservesValuesAndComments(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	e.Regroup()

	if got := e.Get("APP_URL"); got == nil {
		t.Fatal("APP_URL is gone")
	} else {
		if got.Value() != "https://example.com" {
			t.Errorf("APP_URL = %q, want %q", got.Value(), "https://example.com")
		}
		if got.Comment() != "Default dev.host" {
			t.Errorf("APP_URL comment = %q, want %q", got.Comment(), "Default dev.host")
		}
		if !got.HasShadow("http://dev.example.com") {
			t.Error("APP_URL lost its shadow")
		}
	}
	if r := e.Get("APP_TRACE_LOAD"); r == nil || !r.IsCommented() {
		t.Error("APP_TRACE_LOAD is no longer a commented row")
	}
}

func TestRegroupCarriesBlockComment(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	e.Regroup()

	b := e.Block("APP")
	if b == nil {
		t.Fatal("Block(APP) = nil")
	}
	if b.Comment() != "Application section" {
		t.Errorf("Comment = %q, want %q", b.Comment(), "Application section")
	}
	// The rows of the second and third APP runs have joined the first block.
	if b.Len() != 4 {
		t.Errorf("Len = %d, want 4", b.Len())
	}
	if !strings.Contains(e.String(), "###   ---[ Application section ]---   ###") {
		t.Error("the header comment is not written back")
	}
}

func TestRegroupHonoursGroupThreshold(t *testing.T) {
	t.Parallel()

	const src = `APP_NAME=one
APP_DEBUG=false
DB_HOST=localhost
`

	tests := []struct {
		name      string
		threshold int
		want      []string
	}{
		{"one", 1, []string{"APP[APP_NAME,APP_DEBUG]", "DB[DB_HOST]"}},
		{"two", 2, []string{"APP[APP_NAME,APP_DEBUG]", "DB_HOST"}},
		{"three", 3, []string{"APP_NAME", "APP_DEBUG", "DB_HOST"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e, err := envi.ParseString(src)
			if err != nil {
				t.Fatal(err)
			}
			e.Regroup(envi.WithGroupThreshold(tt.threshold))

			if got := layoutOf(e); !slices.Equal(got, tt.want) {
				t.Errorf("layout = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegroupDissolvesBlockBelowThreshold(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("APP_NAME=one\nAPP_DEBUG=false\n")
	if err != nil {
		t.Fatal(err)
	}
	if e.NumBlocks() != 1 {
		t.Fatalf("NumBlocks = %d, want 1", e.NumBlocks())
	}
	e.Delete("APP_DEBUG")
	e.Regroup(envi.WithGroupThreshold(2))

	if e.NumBlocks() != 0 {
		t.Errorf("NumBlocks = %d, want 0", e.NumBlocks())
	}
	if got := layoutOf(e); !slices.Equal(got, []string{"APP_NAME"}) {
		t.Errorf("layout = %v, want [APP_NAME]", got)
	}
}

// A block that dissolves must not take its header comment with it: the text is
// the only place the section was described.
func TestDissolvedBlockKeepsItsHeaderAsAComment(t *testing.T) {
	t.Parallel()

	const src = `###   ---[ Application section ]---   ###
APP_NAME=one
APP_DEBUG=false
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	e.Regroup(envi.WithGroupThreshold(3))

	if e.NumBlocks() != 0 {
		t.Fatalf("NumBlocks = %d, want 0", e.NumBlocks())
	}
	r := e.Get("APP_NAME")
	if r == nil {
		t.Fatal("APP_NAME is gone")
	}
	if r.Comment() != "Application section" {
		t.Errorf("Comment = %q, want %q", r.Comment(), "Application section")
	}
	if !strings.Contains(e.String(), "# Application section") {
		t.Errorf("the header text is not written back:\n%s", e.String())
	}
}

// Demoting a header must not overwrite a comment the row already had.
func TestDissolvedBlockDoesNotOverwriteARowComment(t *testing.T) {
	t.Parallel()

	const src = `###   ---[ Application section ]---   ###
# the name
APP_NAME=one
APP_DEBUG=false
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	e.Regroup(envi.WithGroupThreshold(3))

	if got := e.Get("APP_NAME").Comment(); got != "the name" {
		t.Errorf("Comment = %q, want %q", got, "the name")
	}
}

// A document already in order must come through regrouping untouched, verbatim
// renderings and all. This is what makes Regroup safe to call unconditionally.
func TestRegroupLeavesAnOrderedDocumentByteIdentical(t *testing.T) {
	t.Parallel()

	const src = `###   ---[ Application section ]---   ###
# Application name
APP_NAME="App name"
APP_DEBUG=false

CACHE_PATH=./storage/cache

TEST=false
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.String(); got != src {
		t.Fatalf("the document does not round-trip to begin with:\n--- got ---\n%s", got)
	}

	e.Regroup()

	if got := e.String(); got != src {
		t.Errorf("Regroup changed an ordered document.\n--- got ---\n%s\n--- want ---\n%s", got, src)
	}
}

func TestRegroupIsIdempotent(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	e.Regroup()
	once := e.String()
	e.Regroup()
	if twice := e.String(); twice != once {
		t.Errorf("second Regroup changed the document.\n--- got ---\n%s\n--- want ---\n%s", twice, once)
	}
}

// Regrouped output must reparse into the same arrangement, or a tidied file
// would drift every time it went through the library.
func TestRegroupOutputIsAFixedPoint(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	e.Regroup()
	want := e.String()

	again, err := envi.ParseString(want)
	if err != nil {
		t.Fatalf("regrouped output does not parse: %v", err)
	}
	if got := again.String(); got != want {
		t.Fatalf("regrouped output does not round-trip.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	again.Regroup()
	if got := again.String(); got != want {
		t.Errorf("regrouping the output changed it.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestTidySortsAndGroups(t *testing.T) {
	t.Parallel()

	const src = `ZED=last
APP_NAME=one
DB_HOST=localhost
APP_DEBUG=false
DB_PORT=5432
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	e.Tidy()

	want := []string{"APP[APP_DEBUG,APP_NAME]", "DB[DB_HOST,DB_PORT]", "ZED"}
	if got := layoutOf(e); !slices.Equal(got, want) {
		t.Errorf("layout = %v, want %v", got, want)
	}
}

func TestTidyIsIdempotent(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString(readmeExample)
	if err != nil {
		t.Fatal(err)
	}
	e.Tidy()
	once := e.String()
	e.Tidy()
	if twice := e.String(); twice != once {
		t.Errorf("second Tidy changed the document.\n--- got ---\n%s\n--- want ---\n%s", twice, once)
	}

	again, err := envi.ParseString(once)
	if err != nil {
		t.Fatalf("tidied output does not parse: %v", err)
	}
	again.Tidy()
	if got := again.String(); got != once {
		t.Errorf("tidying the output changed it.\n--- got ---\n%s\n--- want ---\n%s", got, once)
	}
}

func TestRegroupOnEmptyDocument(t *testing.T) {
	t.Parallel()

	var e envi.Env
	e.Regroup()
	e.Tidy()

	if e.Len() != 0 {
		t.Errorf("Len = %d, want 0", e.Len())
	}
	if got := e.String(); got != "" {
		t.Errorf("String = %q, want empty", got)
	}
}

func TestRegroupAfterDeleteSweepsTombstones(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("A_ONE=1\nA_TWO=2\nLOOSE=3\nB_ONE=4\n")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Delete("LOOSE") {
		t.Fatal("Delete(LOOSE) = false")
	}
	e.Regroup()

	want := []string{"A[A_ONE,A_TWO]", "B[B_ONE]"}
	if got := layoutOf(e); !slices.Equal(got, want) {
		t.Errorf("layout = %v, want %v", got, want)
	}
}

// Rows added after parsing land wherever Set could put them; regrouping is what
// files them.
func TestRegroupFilesRowsAddedLater(t *testing.T) {
	t.Parallel()

	e, err := envi.ParseString("APP_NAME=one\nLOOSE=x\n")
	if err != nil {
		t.Fatal(err)
	}
	e.Set("DB_HOST", "localhost")
	e.Set("DB_PORT", "5432")

	if e.Block("DB") != nil {
		t.Fatal("Set built a block on its own; the test no longer proves anything")
	}
	e.Regroup()

	b := e.Block("DB")
	if b == nil {
		t.Fatal("Block(DB) = nil")
	}
	if b.Len() != 2 {
		t.Errorf("Len = %d, want 2", b.Len())
	}
}

// Folding a repeated commented key used to discard the verbatim lines above the
// first statement along with its rendering. Those lines belong to no other row,
// so a blank line or a comment sitting there vanished from the file.
func TestFoldedCommentedDuplicateKeepsTheLinesAboveIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"blank line", "A_B=1\n\n# K=1\n", "A_B=1\n\n# K=1\n"},
		{"comment", "A_B=1\n# note\n# K=1\n", "A_B=1\n# note\n# K=1\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e, err := envi.ParseString(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			if got := e.String(); got != tt.want {
				t.Errorf("String = %q, want %q", got, tt.want)
			}
		})
	}
}

// A header comment that introduces a row no block ends up owning is kept
// verbatim above that row. It used to live nowhere else, so moving the row
// deleted the text; the row now records it as its own comment.
func TestUnconsumedHeaderSurvivesAMove(t *testing.T) {
	t.Parallel()

	const src = `###   ---[ Loose section ]---   ###
ZED=last
APP_NAME=one
APP_DEBUG=false
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.String(); got != src {
		t.Fatalf("the document does not round-trip to begin with:\n%s", got)
	}
	if got := e.Get("ZED").Comment(); got != "Loose section" {
		t.Errorf("Comment = %q, want %q", got, "Loose section")
	}

	e.Tidy()

	if got := e.Get("ZED").Comment(); got != "Loose section" {
		t.Errorf("after Tidy, Comment = %q, want %q", got, "Loose section")
	}
	if !strings.Contains(e.String(), "Loose section") {
		t.Errorf("the header text was dropped:\n%s", e.String())
	}
}

func TestSortedOrderSortsRowsInsideBlocks(t *testing.T) {
	t.Parallel()

	const src = `APP_ZED=z
APP_ALPHA=a
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	if err := envi.NewEncoder(&b, envi.WithOrder(envi.OrderSorted)).Encode(e); err != nil {
		t.Fatal(err)
	}

	const want = "APP_ALPHA=a\nAPP_ZED=z\n"
	if got := b.String(); got != want {
		t.Errorf("sorted output = %q, want %q", got, want)
	}

	// Encoding must not have rearranged the document itself.
	if got := keysOf(e); !slices.Equal(got, []string{"APP_ZED", "APP_ALPHA"}) {
		t.Errorf("keys = %v, want [APP_ZED APP_ALPHA]", got)
	}
}

// Sorted output is written from the model: the recorded lines above a row
// describe where it used to sit, and sorting has moved everything.
func TestSortedOrderDoesNotReproduceVerbatim(t *testing.T) {
	t.Parallel()

	const src = `###   ---[ Section ]---   ###
# about zed
APP_ZED   =   "z"
APP_ALPHA=a
`

	e, err := envi.ParseString(src)
	if err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	if err := envi.NewEncoder(&b, envi.WithOrder(envi.OrderSorted)).Encode(e); err != nil {
		t.Fatal(err)
	}

	got := b.String()
	if strings.Contains(got, "APP_ZED   =") {
		t.Errorf("sorted output reproduced the original spacing:\n%s", got)
	}
	alpha := strings.Index(got, "APP_ALPHA")
	zed := strings.Index(got, "APP_ZED")
	if alpha < 0 || zed < 0 || alpha > zed {
		t.Errorf("rows are not sorted:\n%s", got)
	}
	if !strings.Contains(got, "# about zed") {
		t.Errorf("the comment was dropped:\n%s", got)
	}
}
