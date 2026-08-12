// Package bind fills a configuration struct from .env files and the process
// environment.
//
// It is the piece most programs actually want from a .env library: describe the
// configuration once as a struct, and let the values arrive typed, defaulted and
// checked.
//
//	type Config struct {
//	    Name  string        `env:"APP_NAME"`
//	    Debug bool          `env:"APP_DEBUG,default=false"`
//	    Port  int           `env:"APP_PORT,required"`
//	    TTL   time.Duration `env:"CACHE_TTL,default=30s"`
//	    Hosts []string      `env:"HOSTS,separator=;"`
//	}
//
//	var cfg Config
//	if err := bind.Load(&cfg, bind.WithOptionalFiles(".env"), bind.WithEnviron()); err != nil {
//	    log.Fatal(err)
//	}
//
// The package stands on its own: it needs nothing from the parent package
// beyond reading values, so it can be the only part of envi a program imports.
//
// # Keys
//
// A field's key comes from its tag, or from its name when the tag gives none —
// AppName becomes APP_NAME, HTTPPort becomes HTTP_PORT. Keys are normalised the
// same way the parent package normalises them, so case and punctuation in a tag
// do not matter.
//
// Nested structs are descended into. A tag on the nested field prefixes every
// key below it, so a Database field tagged `env:"DB"` gives its Host field the
// key DB_HOST.
//
// # Tag options
//
//   - required — a missing value is an error rather than a zero field
//   - default=… — the value to use when nothing is set; it may contain commas,
//     so it must be the last option in the tag
//   - separator=… — what divides the elements of a slice or map, "," by default
//   - a tag of "-" skips the field
//
// # Types
//
// Strings, booleans, every sized integer and float, [time.Duration], slices and
// maps of those, pointers to those, and any type implementing
// [encoding.TextUnmarshaler] — which covers [time.Time], [net.IP] and most
// identifier types. A []byte field receives the raw text. Map entries are
// written key:value.
//
// # Errors
//
// Every field that fails is collected, so one run reports everything wrong with
// a configuration instead of stopping at the first problem. The returned error
// is an [*Error]; [errors.Is] and [errors.As] see through it to the individual
// causes, including [ErrRequired].
//
// # Precedence and absence
//
// Files are read in the order given, each overriding the one before, and the
// process environment overrides them all when [WithEnviron] is used. A key that
// is present but empty counts as absent: it states that nothing was configured,
// not that the field should be blanked.
package bind
