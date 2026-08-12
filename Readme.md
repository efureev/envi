# envi

[![Go package](https://github.com/efureev/envi/actions/workflows/go.yml/badge.svg)](https://github.com/efureev/envi/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/efureev/envi/v2.svg)](https://pkg.go.dev/github.com/efureev/envi/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/envi/v2)](https://goreportcard.com/report/github.com/efureev/envi/v2)
![Go version](https://img.shields.io/github/go-mod/go-version/efureev/envi)
![Zero dependencies](https://img.shields.io/badge/dependencies-none-success)
![License](https://img.shields.io/github/license/efureev/envi)

### The `.env` library that gives you your file back.

Every `.env` library can read a file. Almost none can **write one back**. Load a config into a map,
dump it to disk, and your carefully organised file comes back sorted, unquoted, uncommented — a
stranger. So nobody edits `.env` files programmatically. They edit them by hand, forever.

`envi` models the document, not just its values. Comments, sections, blank lines, quote style, even
CRLF endings survive the trip. Change one key, get a one-line diff.

---

## See the difference

Given `.env`:

```dotenv
###   ---[ Application ]---   ###
# Human readable name
APP_NAME="My App"
APP_DEBUG=false
```

Flip one flag:

```go
env, _ := envi.Load(".env")
env.Set("APP_DEBUG", "true")
envi.Save(env, ".env")
```

**With `envi`** — the diff is what you actually changed:

```diff
  ###   ---[ Application ]---   ###
  # Human readable name
  APP_NAME="My App"
- APP_DEBUG=false
+ APP_DEBUG=true
```

**With a load-into-a-map round trip** — the diff is the whole file:

```diff
- ###   ---[ Application ]---   ###
- # Human readable name
- APP_NAME="My App"
- APP_DEBUG=false
+ APP_DEBUG="true"
+ APP_NAME="My App"
```

That difference is the entire point of this library. It is guaranteed by a property test: parse any
document, write it back, and the bytes match. Continuously fuzzed.

---

## Install

```shell
go get github.com/efureev/envi/v2
```

```go
import envi "github.com/efureev/envi/v2"
```

Requires Go 1.26. **Zero dependencies** — in the library and in its test suite.

---

## Two ways to use it

### 1. Just give me my config, typed

Most programs want this and nothing else. The `bind` subpackage is self-contained: it can be the
only part of `envi` you import.

```go
import "github.com/efureev/envi/v2/bind"

type Config struct {
    Name  string        `env:"APP_NAME"`
    Port  int           `env:"APP_PORT,required"`
    TTL   time.Duration `env:"CACHE_TTL,default=30s"`
    Hosts []string      `env:"HOSTS,separator=;"`
    DB    struct {
        Host string `env:"HOST,required"`
        Port int    `env:"PORT,default=5432"`
    } `env:"DB"`
}

var cfg Config
if err := bind.Load(&cfg, bind.WithOptionalFiles(".env"), bind.WithEnviron()); err != nil {
    log.Fatal(err)
}
```

Files load in order, each overriding the last; the process environment overrides them all. Missing
optional files are fine — the container case, where everything arrives through the environment.

**Errors arrive all at once**, not one per run:

```
bind: 2 fields failed:
  - Port (APP_PORT): strconv.ParseInt: parsing "eighty": invalid syntax
  - DB.Host (DB_HOST): bind: required value is missing
```

Out of the box: every sized integer and float, `bool`, `string`, `time.Duration`, slices, maps,
pointers, nested structs, `[]byte`, and anything implementing `encoding.TextUnmarshaler` — which
quietly covers `time.Time`, `net.IP` and most ID types. No tag? The key comes from the field name:
`AppName` → `APP_NAME`, `HTTPPort` → `HTTP_PORT`.

### 2. Treat the file as a document

```go
env, err := envi.Load(".env", ".env.local")   // merge, later wins

env.Set("APP_PORT", "8080")                   // create or update
env.Delete("APP_DEBUG")
env.Get("APP_URL").SetComment("staging only")

envi.Save(env, ".env")                        // atomic write
```

Useful for the things nobody writes tooling for today: installers that seed a `.env`, CLIs that
toggle a feature flag, CI checks that diff `.env` against `.env.example`, migrations that rename a
key across a fleet of repositories.

---

## What you get that a map doesn't

| | |
|---|---|
| **Comments** | Section headers, per-row comments and trailing comments, each kept where it was |
| **Shadows** | `# REDIS_HOST=redis` next to the live value is a first-class concept, not a lost line |
| **Order** | Source order preserved; sorting is something you ask for, never a side effect |
| **Blocks** | Rows sharing a prefix group into a `Block` you can address as a unit |
| **Formatting** | Quote style, spacing, blank lines, CRLF vs LF — all reproduced |
| **Positions** | A malformed line reports its line *and* column, not "invalid format" |
| **No limits** | No 64 KiB line cap; a base64 certificate in one value parses fine |

```go
row := env.Get("REDIS_HOST")
row.Value()                     // "127.0.0.1"
row.Comment()                   // "for docker"
for alt := range row.Shadows() {
    fmt.Println(alt)            // redis
}
```

---

## Fast, and measured

The parser is a hand-written single-pass scanner. No regular expressions anywhere in the module.

1000-line file, Go 1.26, Apple M5 Pro, `benchstat`, `-count 8`:

| Operation | Time | Allocations |
|---|---:|---:|
| Parse | **237 µs** (134 MiB/s) | 4 405 |
| Write | **22.6 µs** | 5 |
| Lookup | **27.8 ns** | **0** |
| Bind 50 struct fields | **1.0 µs** | 2 |

Lookups are O(1) and allocation-free. Binding resolves each type's layout once and caches it, so the
thousandth config costs the same as the second.

For scale: those numbers are 25–67× better than this library's own v1, whose parser recompiled a
regular expression on every line. The full teardown, with the measurements that motivated the
rewrite, is in [docs/AUDIT.md](docs/AUDIT.md).

---

## Built to be trusted with your config

- **Zero dependencies.** Nothing to audit, nothing to update, no supply chain.
- **Continuously fuzzed.** Three fuzz targets run in CI: the parser never panics, and its own output
  always parses back to the same document. Every input a fuzzer ever rejected is committed as a
  permanent regression test.
- **Race-tested and order-shuffled.** `go test -race -shuffle=on` on Linux, macOS and Windows. The
  package holds no mutable global state, so two libraries in one process cannot fight over settings.
- **Atomic writes.** `Save` writes a temporary file and renames it. An interrupted run cannot leave
  you with half a `.env`.
- **Documented.** Every exported symbol carries a doc comment, enforced by the linter.

---

## API tour

```go
// Read
env, err := envi.Parse(r)                       // any io.Reader
env, err := envi.ParseString(s)
env, err := envi.ParseBytes(b)
env, err := envi.Load(".env", ".env.local")

// Write
env.WriteTo(w)                                  // io.WriterTo
env.MarshalText()                               // encoding.TextMarshaler
envi.Save(env, ".env")

// Look up — O(1), keys normalised, so spelling does not matter
env.Lookup("app-port")                          // "8080", true
env.Get("APP_PORT")                             // *Row
env.Block("APP")                                // *Block

// Iterate — no intermediate slices
for key, value := range env.All() { }
for row := range env.Rows() { }
for item := range env.Items() { }               // *Row or *Block

// Edit
env.Set("K", "v")
env.Add(envi.NewRow("HYPE", "false"))
env.Delete("K")
env.Merge(other)
env.SortByKey()
env.Export(true)                                // into os.Environ
```

Formatting is chosen per operation, never globally:

```go
envi.NewEncoder(w,
    envi.WithQuoting(envi.QuoteAlways),
    envi.WithOrder(envi.OrderSorted),
    envi.WithComments(false),
).Encode(env)
```

Errors carry position:

```go
var syntaxErr *envi.SyntaxError
if errors.As(err, &syntaxErr) {
    fmt.Println(syntaxErr.Line, syntaxErr.Col, syntaxErr.Msg)
}
```

Full reference and runnable examples: [pkg.go.dev](https://pkg.go.dev/github.com/efureev/envi/v2).

---

## Concurrency

An `*Env` is a mutable document — guard it like any other. `Decoder`, `Encoder` and `bind` share no
state between instances, so parsing, writing and binding from many goroutines at once is safe.

---

## Version 1

`v1` is frozen. It stays fetchable by tag (`go get github.com/efureev/envi@v1.3.1`) but receives no
fixes and has known crashes in `Merge` and `RemoveRow`, catalogued in [docs/AUDIT.md](docs/AUDIT.md).

New code imports `github.com/efureev/envi/v2`. The `/v2` suffix stays in the import path even though
the code sits at the repository root — that is simply how Go names a major version.

---

## Contributing

Bug reports with a failing `.env` fragment are worth their weight; they usually become a test
verbatim. See [CONTRIBUTING.md](CONTRIBUTING.md), and [docs/UPGRADE-SPEC.md](docs/UPGRADE-SPEC.md)
for what is open and where this is heading.

## License

[MIT](LICENSE)
