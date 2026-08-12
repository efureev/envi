package envi_test

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// genEnv builds the same document shape the v1 baseline measures, so the two
// sets of numbers compare directly. See docs/AUDIT.md §3.
func genEnv(n int) string {
	var b strings.Builder
	for i := range n {
		if i%10 == 0 {
			fmt.Fprintf(&b, "###   ---[ Section %d ]---   ###\n", i/10)
		}
		if i%3 == 0 {
			fmt.Fprintf(&b, "# comment for key %d\n", i)
		}
		fmt.Fprintf(&b, "GRP%d_KEY%d=value-%d\n", i/10, i, i)
	}
	return b.String()
}

var benchSizes = []int{10, 100, 1000}

func BenchmarkParse(b *testing.B) {
	for _, n := range benchSizes {
		src := genEnv(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))
			for b.Loop() {
				if _, err := envi.ParseString(src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMarshal(b *testing.B) {
	for _, n := range benchSizes {
		e, err := envi.ParseString(genEnv(n))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := e.WriteTo(io.Discard); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkGet(b *testing.B) {
	for _, n := range benchSizes {
		e, err := envi.ParseString(genEnv(n))
		if err != nil {
			b.Fatal(err)
		}
		key := fmt.Sprintf("GRP%d_KEY%d", (n-1)/10, n-1)
		if e.Get(key) == nil {
			b.Fatalf("precondition: %s must be present", key)
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if e.Get(key) == nil {
					b.Fatal("not found")
				}
			}
		})
	}
}

func BenchmarkLookup(b *testing.B) {
	e, err := envi.ParseString(genEnv(1000))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := e.Lookup("GRP99_KEY999"); !ok {
			b.Fatal("not found")
		}
	}
}

func BenchmarkNewRow(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = envi.NewRow("app-section_name.sub", "value")
	}
}
