package bind_test

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/efureev/envi/v2/bind"
)

// The case the option exists for. url.URL does not implement
// encoding.TextUnmarshaler and the method cannot be added to it, so without a
// converter the field is not merely left empty: it is taken apart and filled
// from ENDPOINT_SCHEME, ENDPOINT_HOST and friends, silently, with no error and
// no sign that ENDPOINT itself was ignored.
func TestConverterFillsATypeThatCannotReadItself(t *testing.T) {
	t.Parallel()

	type Config struct {
		Endpoint *url.URL `env:"ENDPOINT"`
	}
	src := mapSource{
		"ENDPOINT":        "https://api.example.com/v1",
		"ENDPOINT_SCHEME": "sneaky",
		"ENDPOINT_HOST":   "sneaky.example",
	}

	t.Run("without a converter the field is assembled from its parts", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		if err := bind.Decode(src, &cfg); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.Endpoint == nil {
			t.Fatal("Endpoint = nil")
		}
		if got := cfg.Endpoint.String(); got != "sneaky://sneaky.example" {
			t.Errorf("Endpoint = %q, want it built from the part keys", got)
		}
	})

	t.Run("with a converter the field is read from its own key", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		if err := bind.Decode(src, &cfg, bind.WithConverter(url.Parse)); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if got := cfg.Endpoint.String(); got != "https://api.example.com/v1" {
			t.Errorf("Endpoint = %q, want %q", got, "https://api.example.com/v1")
		}
	})
}

// shouty reads text by upper-casing it, so that a converter registered for the
// same type is visibly different.
type shouty struct{ text string }

func (s *shouty) UnmarshalText(b []byte) error {
	s.text = strings.ToUpper(string(b))
	return nil
}

func TestConverterWinsOverTextUnmarshaler(t *testing.T) {
	t.Parallel()

	type Config struct {
		V shouty `env:"V"`
	}

	t.Run("without a converter the type reads itself", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		if err := bind.Decode(mapSource{"V": "hello"}, &cfg); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.V.text != "HELLO" {
			t.Errorf("text = %q, want %q", cfg.V.text, "HELLO")
		}
	})

	t.Run("a converter overrides it", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		err := bind.Decode(mapSource{"V": "hello"}, &cfg,
			bind.WithConverter(func(s string) (shouty, error) { return shouty{text: "<" + s + ">"}, nil }))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.V.text != "<hello>" {
			t.Errorf("text = %q, want the converter's %q", cfg.V.text, "<hello>")
		}
	})
}

// point has no UnmarshalText, so without a converter it is a group of fields.
// Registering it must make it a value instead — the change a converter makes is
// structural, not merely which setter runs.
type point struct {
	X int
	Y int
}

func TestConverterStopsAStructBeingTakenApart(t *testing.T) {
	t.Parallel()

	type Config struct {
		P point `env:"P"`
	}
	src := mapSource{"P": "3x4", "P_X": "99", "P_Y": "99"}

	t.Run("without a converter it is filled field by field", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		if err := bind.Decode(src, &cfg); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.P.X != 99 || cfg.P.Y != 99 {
			t.Errorf("P = %v, want it filled from P_X and P_Y", cfg.P)
		}
	})

	t.Run("with a converter it is one value", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		err := bind.Decode(src, &cfg, bind.WithConverter(parsePoint))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.P.X != 3 || cfg.P.Y != 4 {
			t.Errorf("P = %v, want {3 4} read from the single key", cfg.P)
		}
	})
}

func parsePoint(s string) (point, error) {
	var p point
	if _, err := fmt.Sscanf(s, "%dx%d", &p.X, &p.Y); err != nil {
		return point{}, fmt.Errorf("%q is not WxH: %w", s, err)
	}
	return p, nil
}

// Registering the value type covers a pointer to it, because a pointer field is
// filled by reading the value it points at. The reverse does not hold.
func TestConverterOnValueCoversPointerField(t *testing.T) {
	t.Parallel()

	type Config struct {
		P *point `env:"P"`
	}

	var cfg Config
	if err := bind.Decode(mapSource{"P": "3x4"}, &cfg, bind.WithConverter(parsePoint)); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.P == nil {
		t.Fatal("P = nil")
	}
	if cfg.P.X != 3 || cfg.P.Y != 4 {
		t.Errorf("P = %v, want {3 4}", *cfg.P)
	}
}

// Elements are covered without registering anything further, because the
// element converter is resolved through the same path.
func TestConverterReachesElements(t *testing.T) {
	t.Parallel()

	type Config struct {
		List []point          `env:"LIST,separator=;"`
		ByID map[string]point `env:"BYID,separator=;"`
		Ptrs []*point         `env:"PTRS,separator=;"`
	}

	src := mapSource{
		"LIST": "1x2;3x4",
		"BYID": "a:1x1;b:2x2",
		"PTRS": "5x6",
	}

	var cfg Config
	if err := bind.Decode(src, &cfg, bind.WithConverter(parsePoint)); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if len(cfg.List) != 2 || cfg.List[0] != (point{1, 2}) || cfg.List[1] != (point{3, 4}) {
		t.Errorf("List = %v, want [{1 2} {3 4}]", cfg.List)
	}
	if cfg.ByID["a"] != (point{1, 1}) || cfg.ByID["b"] != (point{2, 2}) {
		t.Errorf("ByID = %v, want a:{1 1} b:{2 2}", cfg.ByID)
	}
	if len(cfg.Ptrs) != 1 || *cfg.Ptrs[0] != (point{5, 6}) {
		t.Errorf("Ptrs = %v, want [{5 6}]", cfg.Ptrs)
	}
}

// A converter's error joins the same aggregate as every other failing field, so
// one run still reports everything wrong with the configuration.
func TestConverterErrorIsCollected(t *testing.T) {
	t.Parallel()

	type Config struct {
		P    point `env:"P"`
		Port int   `env:"PORT"`
	}

	var cfg Config
	err := bind.Decode(mapSource{"P": "nonsense", "PORT": "eighty"}, &cfg, bind.WithConverter(parsePoint))
	if err == nil {
		t.Fatal("Decode = nil error, want one")
	}

	var berr *bind.Error
	if !errors.As(err, &berr) {
		t.Fatalf("error %v (%T) is not a *bind.Error", err, err)
	}
	if len(berr.Fields) != 2 {
		t.Fatalf("Fields = %d, want 2:\n%v", len(berr.Fields), err)
	}
	if !strings.Contains(err.Error(), "is not WxH") {
		t.Errorf("error = %q, want it to carry the converter's message", err)
	}
}

func TestConverterRegistrations(t *testing.T) {
	t.Parallel()

	type Config struct {
		P point    `env:"P"`
		U *url.URL `env:"U"`
	}
	src := mapSource{"P": "1x2", "U": "https://example.com"}

	t.Run("several types accumulate", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		err := bind.Decode(src, &cfg, bind.WithConverter(parsePoint), bind.WithConverter(url.Parse))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.P != (point{1, 2}) {
			t.Errorf("P = %v, want {1 2}", cfg.P)
		}
		if cfg.U == nil || cfg.U.Host != "example.com" {
			t.Errorf("U = %v, want example.com", cfg.U)
		}
	})

	t.Run("the last registration of a type wins", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		err := bind.Decode(src, &cfg,
			bind.WithConverter(parsePoint),
			bind.WithConverter(func(string) (point, error) { return point{7, 8}, nil }),
			bind.WithConverter(url.Parse),
		)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.P != (point{7, 8}) {
			t.Errorf("P = %v, want the later converter's {7 8}", cfg.P)
		}
	})

	t.Run("a nil converter registers nothing", func(t *testing.T) {
		t.Parallel()

		type plain struct {
			Port int `env:"PORT"`
		}
		var cfg plain
		if err := bind.Decode(mapSource{"PORT": "80"}, &cfg, bind.WithConverter[point](nil)); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.Port != 80 {
			t.Errorf("Port = %d, want 80", cfg.Port)
		}
	})
}

// The regression the cache bypass exists for: one struct type bound twice, once
// with converters and once without, in either order. A plan cached by type
// alone would hand the second call the first call's plan, and the difference is
// structural — whether P is a value or a pair of fields.
func TestConverterDoesNotLeakThroughThePlanCache(t *testing.T) {
	t.Parallel()

	type Config struct {
		P point `env:"P"`
	}
	src := mapSource{"P": "3x4", "P_X": "99", "P_Y": "99"}

	withConv := func() point {
		var cfg Config
		if err := bind.Decode(src, &cfg, bind.WithConverter(parsePoint)); err != nil {
			t.Fatalf("Decode with converter: %v", err)
		}
		return cfg.P
	}
	without := func() point {
		var cfg Config
		if err := bind.Decode(src, &cfg); err != nil {
			t.Fatalf("Decode without converter: %v", err)
		}
		return cfg.P
	}

	t.Run("converter first", func(t *testing.T) {
		t.Parallel()

		if got := withConv(); got != (point{3, 4}) {
			t.Errorf("with converter = %v, want {3 4}", got)
		}
		if got := without(); got != (point{99, 99}) {
			t.Errorf("without converter = %v, want {99 99}", got)
		}
	})

	t.Run("plain first", func(t *testing.T) {
		t.Parallel()

		if got := without(); got != (point{99, 99}) {
			t.Errorf("without converter = %v, want {99 99}", got)
		}
		if got := withConv(); got != (point{3, 4}) {
			t.Errorf("with converter = %v, want {3 4}", got)
		}
	})
}

// Registering a type does not exempt its fields from the rules that apply to
// every other field.
func TestConverterRespectsTagOptions(t *testing.T) {
	t.Parallel()

	type Config struct {
		Skipped  point `env:"-"`
		Fallback point `env:"FB,default=9x9"`
		Needed   point `env:"NEEDED,required"`
	}

	t.Run("default and skip", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		if err := bind.Decode(mapSource{"NEEDED": "1x1"}, &cfg, bind.WithConverter(parsePoint)); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if cfg.Fallback != (point{9, 9}) {
			t.Errorf("Fallback = %v, want the default {9 9}", cfg.Fallback)
		}
		if cfg.Skipped != (point{}) {
			t.Errorf("Skipped = %v, want the zero value", cfg.Skipped)
		}
	})

	t.Run("required", func(t *testing.T) {
		t.Parallel()

		var cfg Config
		err := bind.Decode(mapSource{}, &cfg, bind.WithConverter(parsePoint))
		if !errors.Is(err, bind.ErrRequired) {
			t.Errorf("error = %v, want ErrRequired", err)
		}
	})
}

// A type with no converter and no way to read itself is still an error, so the
// option adds a way in without loosening what it means to be unsupported.
func TestUnregisteredTypeIsStillUnsupported(t *testing.T) {
	t.Parallel()

	type Config struct {
		Ch chan int `env:"CH"`
	}

	var cfg Config
	err := bind.Decode(mapSource{"CH": "x"}, &cfg, bind.WithConverter(parsePoint))
	if !errors.Is(err, bind.ErrUnsupportedType) {
		t.Errorf("error = %v, want ErrUnsupportedType", err)
	}
}
