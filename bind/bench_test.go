package bind_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	envi "github.com/efureev/envi/v2"
	"github.com/efureev/envi/v2/bind"
)

// wideConfig has fifty fields across the types a real configuration uses, which
// is the shape docs/UPGRADE-SPEC.md §6 sets a budget for.
type wideConfig struct {
	S00, S01, S02, S03, S04, S05, S06, S07, S08, S09 string
	S10, S11, S12, S13, S14, S15, S16, S17, S18, S19 string
	I20, I21, I22, I23, I24, I25, I26, I27, I28, I29 int
	B30, B31, B32, B33, B34, B35, B36, B37, B38, B39 bool
	D40, D41, D42, D43, D44                          time.Duration
	F45, F46, F47, F48, F49                          float64
}

// wideSource builds the values every field of wideConfig asks for.
func wideSource() *envi.Env {
	var b strings.Builder
	for i := range 20 {
		fmt.Fprintf(&b, "S%02d=value-%d\n", i, i)
	}
	for i := 20; i < 30; i++ {
		fmt.Fprintf(&b, "I%02d=%d\n", i, i)
	}
	for i := 30; i < 40; i++ {
		fmt.Fprintf(&b, "B%02d=true\n", i)
	}
	for i := 40; i < 45; i++ {
		fmt.Fprintf(&b, "D%02d=%ds\n", i, i)
	}
	for i := 45; i < 50; i++ {
		fmt.Fprintf(&b, "F%02d=%d.5\n", i, i)
	}
	env, err := envi.ParseString(b.String())
	if err != nil {
		panic(err)
	}
	return env
}

// BenchmarkDecode measures binding with the type's plan already cached, which
// is the steady state: reflection is paid once per type, not per value.
func BenchmarkDecode(b *testing.B) {
	src := wideSource()

	var warm wideConfig
	if err := bind.Decode(src, &warm); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		var cfg wideConfig
		if err := bind.Decode(src, &cfg); err != nil {
			b.Fatal(err)
		}
	}
}

// There is deliberately no cold-start benchmark: the plan cache is global and
// keyed by type, so the first call cannot be repeated within one run. What it
// costs is one pass of reflection over the struct, paid once per type per
// process.
