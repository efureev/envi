package bind_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/efureev/envi/v2/bind"
)

// Every conversion failure must be reported against the field it happened in,
// rather than aborting the whole decode with a bare error.
func TestConversionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dst  any
		src  mapSource
	}{
		{"bool", &struct {
			V bool `env:"V"`
		}{}, mapSource{"V": "maybe"}},

		{"int", &struct {
			V int `env:"V"`
		}{}, mapSource{"V": "twelve"}},

		{"int overflow", &struct {
			V int8 `env:"V"`
		}{}, mapSource{"V": "999"}},

		{"uint", &struct {
			V uint `env:"V"`
		}{}, mapSource{"V": "-1"}},

		{"float", &struct {
			V float64 `env:"V"`
		}{}, mapSource{"V": "one point five"}},

		{"duration", &struct {
			V time.Duration `env:"V"`
		}{}, mapSource{"V": "soon"}},

		{"time via TextUnmarshaler", &struct {
			V time.Time `env:"V"`
		}{}, mapSource{"V": "yesterday"}},

		{"ip via TextUnmarshaler", &struct {
			V net.IP `env:"V"`
		}{}, mapSource{"V": "999.999.999.999"}},

		{"pointer to int", &struct {
			V *int `env:"V"`
		}{}, mapSource{"V": "nope"}},

		{"slice element", &struct {
			V []int `env:"V"`
		}{}, mapSource{"V": "1,two,3"}},

		{"map entry without colon", &struct {
			V map[string]int `env:"V"`
		}{}, mapSource{"V": "justakey"}},

		{"map key", &struct {
			V map[int]string `env:"V"`
		}{}, mapSource{"V": "x:1"}},

		{"map value", &struct {
			V map[string]int `env:"V"`
		}{}, mapSource{"V": "k:notanint"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := bind.Decode(tc.src, tc.dst)
			if err == nil {
				t.Fatal("Decode accepted an unconvertible value")
			}

			var bindErr *bind.Error
			if !errors.As(err, &bindErr) {
				t.Fatalf("error %v is not a *bind.Error", err)
			}
			if len(bindErr.Fields) != 1 {
				t.Fatalf("collected %d failures, want 1: %v", len(bindErr.Fields), err)
			}
			if bindErr.Fields[0].Field != "V" || bindErr.Fields[0].Key != "V" {
				t.Errorf("failure names %q/%q, want V/V", bindErr.Fields[0].Field, bindErr.Fields[0].Key)
			}
		})
	}
}

// Values that do convert must land intact, including the container types.
func TestConversionSuccess(t *testing.T) {
	t.Parallel()

	var cfg struct {
		Signed   int8              `env:"SIGNED"`
		Unsigned uint32            `env:"UNSIGNED"`
		Float    float32           `env:"FLOAT"`
		Flag     bool              `env:"FLAG"`
		IPs      []net.IP          `env:"IPS"`
		Limits   map[string]int    `env:"LIMITS"`
		Labels   map[string]string `env:"LABELS,separator=;"`
		Wait     *time.Duration    `env:"WAIT"`
	}

	src := mapSource{
		"SIGNED":   "-8",
		"UNSIGNED": "4000000000",
		"FLOAT":    "1.5",
		"FLAG":     "1",
		"IPS":      "10.0.0.1,10.0.0.2",
		"LIMITS":   "a:1,b:2",
		"LABELS":   "x:one;y:two",
		"WAIT":     "250ms",
	}

	if err := bind.Decode(src, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	switch {
	case cfg.Signed != -8 || cfg.Unsigned != 4000000000 || cfg.Float != 1.5 || !cfg.Flag:
		t.Errorf("scalars: %+v", cfg)
	case len(cfg.IPs) != 2 || cfg.IPs[1].String() != "10.0.0.2":
		t.Errorf("IPs = %v", cfg.IPs)
	case cfg.Limits["a"] != 1 || cfg.Limits["b"] != 2:
		t.Errorf("Limits = %v", cfg.Limits)
	case cfg.Labels["x"] != "one" || cfg.Labels["y"] != "two":
		t.Errorf("Labels = %v", cfg.Labels)
	case cfg.Wait == nil || *cfg.Wait != 250*time.Millisecond:
		t.Errorf("Wait = %v", cfg.Wait)
	}
}

// An unsupported type is rejected while the plan is built, so it fails the same
// way every time — including on the second call, which is served from cache.
func TestUnsupportedTypeIsStableAcrossCalls(t *testing.T) {
	t.Parallel()

	type Config struct {
		Ch chan int `env:"CH"`
	}

	for i := range 2 {
		var cfg Config
		err := bind.Decode(mapSource{}, &cfg)
		if !errors.Is(err, bind.ErrUnsupportedType) {
			t.Fatalf("call %d: error = %v, want ErrUnsupportedType", i+1, err)
		}
	}
}

func TestUnsupportedTypeInsideContainers(t *testing.T) {
	t.Parallel()

	t.Run("slice element", func(t *testing.T) {
		t.Parallel()
		var cfg struct {
			V []chan int `env:"V"`
		}
		if err := bind.Decode(mapSource{}, &cfg); !errors.Is(err, bind.ErrUnsupportedType) {
			t.Errorf("error = %v, want ErrUnsupportedType", err)
		}
	})

	t.Run("map value", func(t *testing.T) {
		t.Parallel()
		var cfg struct {
			V map[string]chan int `env:"V"`
		}
		if err := bind.Decode(mapSource{}, &cfg); !errors.Is(err, bind.ErrUnsupportedType) {
			t.Errorf("error = %v, want ErrUnsupportedType", err)
		}
	})
}

func TestLoadFileErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.env")
	if err := os.WriteFile(broken, []byte("A=1\nnot an assignment\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		A string `env:"A"`
	}

	t.Run("required file is malformed", func(t *testing.T) {
		t.Parallel()
		if err := bind.Load(&cfg, bind.WithFiles(broken)); err == nil {
			t.Error("Load accepted a malformed file")
		}
	})

	t.Run("optional file is malformed", func(t *testing.T) {
		t.Parallel()
		// Optional means "may be absent", not "may be broken".
		if err := bind.Load(&cfg, bind.WithOptionalFiles(broken)); err == nil {
			t.Error("Load accepted a malformed optional file")
		}
	})
}

// Binding with nothing to read leaves defaults in place rather than failing.
func TestLoadWithoutSources(t *testing.T) {
	t.Parallel()

	var cfg struct {
		Tier string `env:"TIER,default=dev"`
	}
	if err := bind.Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tier != "dev" {
		t.Errorf("Tier = %q, want the default", cfg.Tier)
	}
}

// Decode tolerates a nil source: every field simply falls back to its default.
func TestDecodeNilSource(t *testing.T) {
	t.Parallel()

	var cfg struct {
		Tier string `env:"TIER,default=dev"`
		Must string `env:"MUST,required"`
	}
	err := bind.Decode(nil, &cfg)

	if cfg.Tier != "dev" {
		t.Errorf("Tier = %q, want the default", cfg.Tier)
	}
	if !errors.Is(err, bind.ErrRequired) {
		t.Errorf("error = %v, want ErrRequired for the missing field", err)
	}
}

func TestFieldErrorUnwraps(t *testing.T) {
	t.Parallel()

	var cfg struct {
		V string `env:"V,required"`
	}
	err := bind.Decode(mapSource{}, &cfg)

	var fieldErr *bind.FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("error %v does not unwrap to *FieldError", err)
	}
	if !errors.Is(fieldErr, bind.ErrRequired) {
		t.Errorf("FieldError does not wrap ErrRequired")
	}
	if got := fieldErr.Error(); got == "" {
		t.Error("FieldError.Error() is empty")
	}
}
