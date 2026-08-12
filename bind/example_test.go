package bind_test

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	envi "github.com/efureev/envi/v2"
	"github.com/efureev/envi/v2/bind"
)

// A configuration is described once and arrives typed, defaulted and checked.
func Example() {
	type Config struct {
		Name  string        `env:"APP_NAME"`
		Port  int           `env:"APP_PORT,required"`
		TTL   time.Duration `env:"CACHE_TTL,default=30s"`
		Hosts []string      `env:"HOSTS"`
	}

	env, err := envi.ParseString("APP_NAME=api\nAPP_PORT=8080\nHOSTS=alpha,beta\n")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	if err := bind.Decode(env, &cfg); err != nil {
		log.Fatal(err)
	}

	fmt.Println(cfg.Name, cfg.Port, cfg.TTL, cfg.Hosts)
	// Output: api 8080 30s [alpha beta]
}

// Load reads files and the process environment in one call, the environment
// winning over the files.
func ExampleLoad() {
	dir, err := os.MkdirTemp("", "envi")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Errors are printed rather than fatal, so the deferred cleanup still runs
	// — and an unexpected one fails the example through its output.
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("APP_NAME=from-file\nAPP_PORT=1\n"), 0o600); err != nil {
		fmt.Println("write:", err)
		return
	}
	if err := os.Setenv("APP_PORT", "9090"); err != nil {
		fmt.Println("setenv:", err)
		return
	}
	defer os.Unsetenv("APP_PORT")

	type Config struct {
		Name string `env:"APP_NAME"`
		Port int    `env:"APP_PORT"`
	}

	var cfg Config
	if err := bind.Load(&cfg, bind.WithOptionalFiles(path), bind.WithEnviron()); err != nil {
		fmt.Println("load:", err)
		return
	}

	fmt.Println(cfg.Name, cfg.Port)
	// Output: from-file 9090
}

// Every field that fails is reported, not just the first.
func ExampleError() {
	type Config struct {
		Port int    `env:"PORT"`
		Name string `env:"NAME,required"`
	}

	src, err := envi.ParseString("PORT=not-a-number\n")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	err = bind.Decode(src, &cfg)

	var bindErr *bind.Error
	if errors.As(err, &bindErr) {
		for _, f := range bindErr.Fields {
			fmt.Println(f.Field, "<-", f.Key)
		}
	}
	fmt.Println("something required is missing:", errors.Is(err, bind.ErrRequired))
	// Output:
	// Port <- PORT
	// Name <- NAME
	// something required is missing: true
}

// Nested structs group related settings, and a tag on the group prefixes
// everything below it.
func ExampleWithPrefix() {
	type Database struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	type Config struct {
		DB Database `env:"DB"`
	}

	src, err := envi.ParseString("SVC_DB_HOST=primary\nSVC_DB_PORT=5432\n")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	if err := bind.Decode(src, &cfg, bind.WithPrefix("SVC")); err != nil {
		log.Fatal(err)
	}

	fmt.Println(cfg.DB.Host, cfg.DB.Port)
	// Output: primary 5432
}

// A type that cannot read itself from text needs a converter. url.URL is the
// usual example: it has no UnmarshalText, and the method cannot be added to it
// from outside its package.
func ExampleWithConverter() {
	type Config struct {
		Endpoint *url.URL   `env:"ENDPOINT"`
		Mirrors  []*url.URL `env:"MIRRORS,separator=;"`
	}

	src, err := envi.ParseString(
		"ENDPOINT=https://api.example.com/v1\n" +
			"MIRRORS=https://eu.example.com;https://us.example.com\n")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	if err := bind.Decode(src, &cfg, bind.WithConverter(url.Parse)); err != nil {
		log.Fatal(err)
	}

	fmt.Println(cfg.Endpoint.Host, len(cfg.Mirrors), cfg.Mirrors[1].Host)
	// Output: api.example.com 2 us.example.com
}

// severity is a type of the caller's own. Registering a converter for it keeps
// the parsing rules next to the configuration instead of forcing an
// UnmarshalText method onto a domain type that has no other reason for one.
type severity int

const (
	severityInfo severity = iota
	severityWarn
	severityError
)

func parseSeverity(s string) (severity, error) {
	switch s {
	case "info":
		return severityInfo, nil
	case "warn":
		return severityWarn, nil
	case "error":
		return severityError, nil
	}
	return 0, fmt.Errorf("unknown severity %q", s)
}

// Converters accumulate: pass one per type. Registering three costs no more
// than registering one, since what a converter suspends — caching the type's
// plan — is suspended by the first and not again by the rest.
func ExampleWithConverter_several() {
	type Config struct {
		Endpoint *url.URL   `env:"ENDPOINT"`
		Subnet   *net.IPNet `env:"SUBNET"`
		LogLevel severity   `env:"LOG_LEVEL,default=info"`
	}

	// net.ParseCIDR returns three values, so it does not fit
	// func(string) (T, error) directly. Wrapping it is the whole adaptation.
	parseCIDR := func(s string) (*net.IPNet, error) {
		_, network, err := net.ParseCIDR(s)
		return network, err
	}

	src, err := envi.ParseString(
		"ENDPOINT=https://api.example.com/v1\n" +
			"SUBNET=10.0.0.0/8\n" +
			"LOG_LEVEL=warn\n")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	err = bind.Decode(src, &cfg,
		bind.WithConverter(url.Parse),
		bind.WithConverter(parseCIDR),
		bind.WithConverter(parseSeverity),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(cfg.Endpoint.Host, cfg.Subnet, cfg.LogLevel == severityWarn)
	// Output: api.example.com 10.0.0.0/8 true
}

// Every converter that fails reports its own field, so one run still shows the
// whole picture rather than the first thing that went wrong.
func ExampleWithConverter_errors() {
	type Config struct {
		Endpoint *url.URL `env:"ENDPOINT"`
		LogLevel severity `env:"LOG_LEVEL"`
	}

	src, err := envi.ParseString("ENDPOINT=://nope\nLOG_LEVEL=shout\n")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	err = bind.Decode(src, &cfg,
		bind.WithConverter(url.Parse),
		bind.WithConverter(parseSeverity),
	)

	fmt.Println(err)
	// Output:
	// bind: 2 fields failed:
	//   - Endpoint (ENDPOINT): parse "://nope": missing protocol scheme
	//   - LogLevel (LOG_LEVEL): unknown severity "shout"
}
