package bind_test

import (
	"errors"
	"fmt"
	"log"
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
