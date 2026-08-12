package bind_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	envi "github.com/efureev/envi/v2"
	"github.com/efureev/envi/v2/bind"
)

// mapSource is a Source built from a literal, showing that the package works
// against anything that can look a key up.
type mapSource map[string]string

func (m mapSource) Lookup(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

func TestDecodeBasicTypes(t *testing.T) {
	t.Parallel()

	type Config struct {
		Name    string
		Debug   bool
		Port    int
		Small   int8
		Count   uint16
		Ratio   float64
		TTL     time.Duration
		Started time.Time
		Raw     []byte
	}

	src := mapSource{
		"NAME":    "app",
		"DEBUG":   "true",
		"PORT":    "8080",
		"SMALL":   "-7",
		"COUNT":   "42",
		"RATIO":   "0.25",
		"TTL":     "30s",
		"STARTED": "2026-08-12T10:00:00Z",
		"RAW":     "abc",
	}

	var cfg Config
	if err := bind.Decode(src, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if cfg.Name != "app" || !cfg.Debug || cfg.Port != 8080 || cfg.Small != -7 {
		t.Errorf("scalars: %+v", cfg)
	}
	if cfg.Count != 42 || cfg.Ratio != 0.25 {
		t.Errorf("numbers: %+v", cfg)
	}
	if cfg.TTL != 30*time.Second {
		t.Errorf("TTL = %v, want 30s", cfg.TTL)
	}
	if cfg.Started.UTC().Format(time.RFC3339) != "2026-08-12T10:00:00Z" {
		t.Errorf("Started = %v", cfg.Started)
	}
	if string(cfg.Raw) != "abc" {
		t.Errorf("Raw = %q, want the text itself", cfg.Raw)
	}
}

func TestKeysDerivedFromFieldNames(t *testing.T) {
	t.Parallel()

	type Config struct {
		AppName  string
		DBHost   string
		HTTPPort int
		ID       string
	}

	src := mapSource{
		"APP_NAME":  "app",
		"DB_HOST":   "localhost",
		"HTTP_PORT": "80",
		"ID":        "x1",
	}

	var cfg Config
	if err := bind.Decode(src, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.AppName != "app" || cfg.DBHost != "localhost" || cfg.HTTPPort != 80 || cfg.ID != "x1" {
		t.Errorf("derived keys did not all resolve: %+v", cfg)
	}
}

func TestDefaultsAndRequired(t *testing.T) {
	t.Parallel()

	type Config struct {
		Host string `env:"HOST,default=localhost"`
		Port int    `env:"PORT,required"`
		TTL  string `env:"TTL,default=1s,2s"` // a default may contain commas
	}

	t.Run("default applies when absent", func(t *testing.T) {
		t.Parallel()
		var cfg Config
		if err := bind.Decode(mapSource{"PORT": "1"}, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Host != "localhost" {
			t.Errorf("Host = %q, want the default", cfg.Host)
		}
		if cfg.TTL != "1s,2s" {
			t.Errorf("TTL = %q, want the comma-bearing default", cfg.TTL)
		}
	})

	t.Run("empty counts as absent", func(t *testing.T) {
		t.Parallel()
		var cfg Config
		if err := bind.Decode(mapSource{"PORT": "1", "HOST": ""}, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Host != "localhost" {
			t.Errorf("Host = %q, want the default", cfg.Host)
		}
	})

	t.Run("required missing is an error", func(t *testing.T) {
		t.Parallel()
		var cfg Config
		err := bind.Decode(mapSource{}, &cfg)
		if err == nil {
			t.Fatal("Decode succeeded without the required value")
		}
		if !errors.Is(err, bind.ErrRequired) {
			t.Errorf("error %v does not wrap ErrRequired", err)
		}
	})
}

func TestRequiredByDefault(t *testing.T) {
	t.Parallel()

	type Config struct {
		A string
		B string `env:"B,default=x"`
	}

	var cfg Config
	err := bind.Decode(mapSource{}, &cfg, bind.WithRequiredByDefault())
	if err == nil {
		t.Fatal("Decode succeeded with A unset")
	}

	var bindErr *bind.Error
	if !errors.As(err, &bindErr) {
		t.Fatalf("error %v is not a *bind.Error", err)
	}
	if len(bindErr.Fields) != 1 || bindErr.Fields[0].Field != "A" {
		t.Errorf("failures = %+v, want only A: B has a default", bindErr.Fields)
	}
}

func TestSlicesAndMaps(t *testing.T) {
	t.Parallel()

	type Config struct {
		Hosts   []string       `env:"HOSTS"`
		Ports   []int          `env:"PORTS,separator=;"`
		Weights map[string]int `env:"WEIGHTS"`
	}

	src := mapSource{
		"HOSTS":   "a, b ,c",
		"PORTS":   "1;2;3",
		"WEIGHTS": "x:1,y:2",
	}

	var cfg Config
	if err := bind.Decode(src, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if strings.Join(cfg.Hosts, "|") != "a|b|c" {
		t.Errorf("Hosts = %q, want elements trimmed", cfg.Hosts)
	}
	if len(cfg.Ports) != 3 || cfg.Ports[2] != 3 {
		t.Errorf("Ports = %v", cfg.Ports)
	}
	if cfg.Weights["x"] != 1 || cfg.Weights["y"] != 2 {
		t.Errorf("Weights = %v", cfg.Weights)
	}
}

func TestPointersAndTextUnmarshaler(t *testing.T) {
	t.Parallel()

	type Config struct {
		Port *int    `env:"PORT"`
		Name *string `env:"NAME"`
		Addr net.IP  `env:"ADDR"`
		Gone *int    `env:"GONE"`
	}

	var cfg Config
	if err := bind.Decode(mapSource{"PORT": "80", "NAME": "x", "ADDR": "10.0.0.1"}, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if cfg.Port == nil || *cfg.Port != 80 {
		t.Errorf("Port = %v", cfg.Port)
	}
	if cfg.Name == nil || *cfg.Name != "x" {
		t.Errorf("Name = %v", cfg.Name)
	}
	if cfg.Addr.String() != "10.0.0.1" {
		t.Errorf("Addr = %v", cfg.Addr)
	}
	if cfg.Gone != nil {
		t.Errorf("Gone = %v, want nil for an absent key", cfg.Gone)
	}
}

func TestNestedStructs(t *testing.T) {
	t.Parallel()

	type Database struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	type Cache struct {
		Host string `env:"CACHE_HOST"`
	}
	type Config struct {
		DB      Database  `env:"DB"`
		Cache   Cache     // no tag: children keep their own keys
		Replica *Database `env:"REPLICA"`
		Unused  *Database `env:"UNUSED"`
	}

	src := mapSource{
		"DB_HOST":      "primary",
		"DB_PORT":      "5432",
		"CACHE_HOST":   "redis",
		"REPLICA_HOST": "secondary",
	}

	var cfg Config
	if err := bind.Decode(src, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if cfg.DB.Host != "primary" || cfg.DB.Port != 5432 {
		t.Errorf("DB = %+v", cfg.DB)
	}
	if cfg.Cache.Host != "redis" {
		t.Errorf("Cache = %+v, want keys unprefixed", cfg.Cache)
	}
	if cfg.Replica == nil || cfg.Replica.Host != "secondary" {
		t.Errorf("Replica = %+v, want it allocated", cfg.Replica)
	}
	if cfg.Unused != nil {
		t.Errorf("Unused = %+v, want it left nil when nothing is set", cfg.Unused)
	}
}

func TestPrefixAndTagName(t *testing.T) {
	t.Parallel()

	type Config struct {
		Port int `cfg:"PORT"`
	}

	var c1 Config
	if err := bind.Decode(mapSource{"APP_PORT": "80"}, &c1, bind.WithPrefix("APP"), bind.WithTagName("cfg")); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c1.Port != 80 {
		t.Errorf("Port = %d", c1.Port)
	}

	// Without the matching tag name the field falls back to its Go name.
	var c2 Config
	if err := bind.Decode(mapSource{"PORT": "81"}, &c2); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c2.Port != 81 {
		t.Errorf("Port = %d, want the name-derived key to be used", c2.Port)
	}
}

func TestSkippedAndUnexportedFields(t *testing.T) {
	t.Parallel()

	type Config struct {
		Kept    string `env:"KEPT"`
		Ignored string `env:"-"`
		hidden  string //nolint:unused // present to prove it is skipped
	}

	var cfg Config
	if err := bind.Decode(mapSource{"KEPT": "y", "IGNORED": "n"}, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Kept != "y" {
		t.Errorf("Kept = %q", cfg.Kept)
	}
	if cfg.Ignored != "" {
		t.Errorf("Ignored = %q, want it untouched", cfg.Ignored)
	}
}

func TestErrorsAreCollected(t *testing.T) {
	t.Parallel()

	type Config struct {
		A int    `env:"A"`
		B bool   `env:"B"`
		C string `env:"C,required"`
	}

	var cfg Config
	err := bind.Decode(mapSource{"A": "notanint", "B": "maybe"}, &cfg)
	if err == nil {
		t.Fatal("Decode succeeded on bad input")
	}

	var bindErr *bind.Error
	if !errors.As(err, &bindErr) {
		t.Fatalf("error %v is not a *bind.Error", err)
	}
	if len(bindErr.Fields) != 3 {
		t.Errorf("collected %d failures, want all 3: %v", len(bindErr.Fields), err)
	}
	if !errors.Is(err, bind.ErrRequired) {
		t.Error("errors.Is does not see ErrRequired through the aggregate")
	}
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not mention %s: %s", want, err)
		}
	}
}

func TestBadDestination(t *testing.T) {
	t.Parallel()

	var notAPointer struct{ A string }
	if err := bind.Decode(mapSource{}, notAPointer); !errors.Is(err, bind.ErrNotPointer) {
		t.Errorf("error = %v, want ErrNotPointer", err)
	}

	var nilPtr *struct{ A string }
	if err := bind.Decode(mapSource{}, nilPtr); !errors.Is(err, bind.ErrNotPointer) {
		t.Errorf("error = %v, want ErrNotPointer", err)
	}

	n := 0
	if err := bind.Decode(mapSource{}, &n); !errors.Is(err, bind.ErrNotStruct) {
		t.Errorf("error = %v, want ErrNotStruct", err)
	}
}

func TestUnsupportedType(t *testing.T) {
	t.Parallel()

	type Config struct {
		Fn func() `env:"FN"`
	}

	var cfg Config
	err := bind.Decode(mapSource{}, &cfg)
	if !errors.Is(err, bind.ErrUnsupportedType) {
		t.Errorf("error = %v, want ErrUnsupportedType", err)
	}
}

// A cyclic type must not send the plan builder into infinite recursion.
func TestCyclicTypeTerminates(t *testing.T) {
	t.Parallel()

	type Node struct {
		Name string `env:"NAME"`
		Next *Node
	}

	var cfg Node
	if err := bind.Decode(mapSource{"NAME": "root"}, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Name != "root" {
		t.Errorf("Name = %q", cfg.Name)
	}
}

// The plan cache is shared, so concurrent decoding of one type must be safe.
// Run under -race.
func TestConcurrentDecode(t *testing.T) {
	t.Parallel()

	type Config struct {
		Port int    `env:"PORT"`
		Name string `env:"NAME"`
	}

	src := mapSource{"PORT": "8080", "NAME": "app"}

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				var cfg Config
				if err := bind.Decode(src, &cfg); err != nil {
					t.Error(err)
					return
				}
				if cfg.Port != 8080 || cfg.Name != "app" {
					t.Errorf("bad decode: %+v", cfg)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestDecodeFromParsedEnv(t *testing.T) {
	t.Parallel()

	type Config struct {
		Name  string `env:"APP_NAME"`
		Debug bool   `env:"APP_DEBUG"`
	}

	env, err := envi.ParseString("###   ---[ App ]---   ###\nAPP_NAME=\"My App\"\nAPP_DEBUG=true\n")
	if err != nil {
		t.Fatal(err)
	}

	var cfg Config
	if err := bind.Decode(env, &cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Name != "My App" || !cfg.Debug {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadFilesAndEnviron(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".env")
	local := filepath.Join(dir, ".env.local")

	if err := os.WriteFile(base, []byte("APP_NAME=base\nAPP_PORT=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("APP_PORT=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	type Config struct {
		Name string `env:"APP_NAME"`
		Port int    `env:"APP_PORT"`
		Tier string `env:"APP_TIER,default=dev"`
	}

	t.Run("later file wins", func(t *testing.T) {
		var cfg Config
		if err := bind.Load(&cfg, bind.WithFiles(base, local)); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Name != "base" || cfg.Port != 2 || cfg.Tier != "dev" {
			t.Errorf("cfg = %+v", cfg)
		}
	})

	t.Run("environ wins over files", func(t *testing.T) {
		t.Setenv("APP_PORT", "3")

		var cfg Config
		if err := bind.Load(&cfg, bind.WithFiles(base, local), bind.WithEnviron()); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Port != 3 {
			t.Errorf("Port = %d, want the environment to win", cfg.Port)
		}
	})

	t.Run("missing required file is an error", func(t *testing.T) {
		var cfg Config
		err := bind.Load(&cfg, bind.WithFiles(filepath.Join(dir, "nope.env")))
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("error = %v, want it to wrap os.ErrNotExist", err)
		}
	})

	t.Run("missing optional file is fine", func(t *testing.T) {
		t.Setenv("APP_NAME", "from-env")

		var cfg Config
		err := bind.Load(&cfg,
			bind.WithOptionalFiles(filepath.Join(dir, "nope.env")),
			bind.WithEnviron(),
		)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Name != "from-env" {
			t.Errorf("Name = %q", cfg.Name)
		}
	})
}
