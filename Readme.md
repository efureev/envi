# envi

[![Go package](https://github.com/efureev/envi/actions/workflows/go.yml/badge.svg)](https://github.com/efureev/envi/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/efureev/envi/v2.svg)](https://pkg.go.dev/github.com/efureev/envi/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/envi/v2)](https://goreportcard.com/report/github.com/efureev/envi/v2)
![Go version](https://img.shields.io/github/go-mod/go-version/efureev/envi?filename=v2%2Fgo.mod)
![License](https://img.shields.io/github/license/efureev/envi)

Read, edit and write `.env` files **without losing their shape**.

Most `.env` libraries hand you a `map[string]string` and forget the file. `envi` models the document
itself — comments, sections, blank lines and commented-out alternatives — so a read-modify-write
cycle changes only the line you meant to change. With the default options, a parsed file is written
back **byte for byte identical**.

```go
env, _ := envi.ParseString(src)
env.Set("APP_DEBUG", "true")
envi.Save(env, ".env")   // only APP_DEBUG's line differs
```

Zero dependencies, in the library and in its tests.

## Install

```shell
go get github.com/efureev/envi/v2
```

```go
import envi "github.com/efureev/envi/v2"
```

## Binding a config struct

If all you want is your configuration typed and validated, use the `bind` subpackage. It stands on
its own and can be the only part of `envi` you import.

```go
import "github.com/efureev/envi/v2/bind"

type Config struct {
    Name  string        `env:"APP_NAME"`
    Port  int           `env:"APP_PORT,required"`
    TTL   time.Duration `env:"CACHE_TTL,default=30s"`
    Hosts []string      `env:"HOSTS,separator=;"`
}

var cfg Config
err := bind.Load(&cfg, bind.WithOptionalFiles(".env"), bind.WithEnviron())
```

Files are read in order, each overriding the one before, and the process environment overrides them
all. Every field that fails is reported at once, not just the first:

```
bind: 2 fields failed:
  - Port (APP_PORT): strconv.ParseInt: parsing "eighty": invalid syntax
  - Name (APP_NAME): bind: required value is missing
```

Supported: strings, all sized integers and floats, booleans, `time.Duration`, slices, maps
(`key:value`), pointers, nested structs, `[]byte`, and any type implementing
`encoding.TextUnmarshaler` — which covers `time.Time`, `net.IP` and most identifier types. A field
without a tag takes its key from its name: `AppName` → `APP_NAME`, `HTTPPort` → `HTTP_PORT`.

## The document model

Given this file:

```dotenv
###   ---[ Application section ]---   ###
# Application name
APP_NAME="App name"
APP_DEBUG=false

# Default dev host
# APP_URL=http://dev.example.com
APP_URL=https://example.com

#HYPE=false
```

`envi` sees one **block** `APP` introduced by a header comment and holding three **rows**; a
commented-out row `HYPE`; and on `APP_URL` a **shadow** — the commented alternative kept beside the
live value.

```go
env, err := envi.ParseString(src)

env.Lookup("app-url")          // "https://example.com", true — keys are normalised
env.Get("APP_URL").Comment()   // "Default dev host"
env.Block("APP").Len()         // 3

for shadow := range env.Get("APP_URL").Shadows() {
    fmt.Println(shadow)        // http://dev.example.com
}
```

## Reading and writing

```go
env, err := envi.Parse(r)                        // any io.Reader
env, err := envi.ParseString(s)
env, err := envi.ParseBytes(b)
env, err := envi.Load(".env", ".env.local")      // later files override earlier ones

env.WriteTo(w)                                   // io.WriterTo
env.MarshalText()                                // encoding.TextMarshaler
envi.Save(env, ".env")                           // atomic: temp file, then rename
```

`Decoder` and `Encoder` take the same options and share no state, so separate instances are safe to
use from different goroutines.

```go
err := envi.NewEncoder(w, envi.WithQuoting(envi.QuoteAlways), envi.WithIndent(2)).Encode(env)
```

| Option | Effect |
|---|---|
| `WithQuoting` | `QuotePreserve` (default), `QuoteMinimal`, `QuoteAlways` |
| `WithOrder` | `OrderSource` (default) or `OrderSorted` |
| `WithIndent` | blank lines after each block |
| `WithBlockComment` | the strings wrapping a section header |
| `WithGroupThreshold` | how many rows sharing a prefix form a block |
| `WithComments`, `WithShadows`, `WithCommentedRows` | leave those out of the output |

## Editing

```go
env.Set("APP_PORT", "8080")                 // creates or updates; joins its block
env.Add(envi.NewRow("HYPE", "false"))
env.Delete("APP_DEBUG")                     // reports whether anything went
env.Merge(other)                            // other wins, comments survive
env.SortByKey()                             // explicit; reading never reorders

for key, value := range env.All() { ... }   // iterators, no intermediate slice
for row := range env.Rows() { ... }
for item := range env.Items() { ... }       // *Row or *Block

env.Export(true)                            // into the process environment
```

## Errors

A malformed line reports where it went wrong:

```go
var syntaxErr *envi.SyntaxError
if errors.As(err, &syntaxErr) {
    fmt.Println(syntaxErr.Line, syntaxErr.Col, syntaxErr.Msg)
}
```

## Performance

Against v1 on the same machine, parsing a 1000-line file (Go 1.26, Apple M5 Pro):

| | v1 | v2 | |
|---|---:|---:|---|
| Parse | 11.27 ms | **240.8 µs** | 47× faster |
| Parse throughput | 2.8 MiB/s | **131.8 MiB/s** | |
| Parse allocations | 295 687 | **4 405** | 67× fewer |
| Write | 568.9 µs | **22.6 µs** | 25× faster |
| Lookup | 1.36 µs | **30.7 ns** | 44× faster, zero allocations |

The parser is a hand-written single-pass scanner: no regular expressions, no line-length limit, and
positions tracked for error reporting. Benchmarks live in the repository; the reasoning is in
[docs/AUDIT.md](docs/AUDIT.md) and [docs/UPGRADE-SPEC.md](docs/UPGRADE-SPEC.md).

## Concurrency

An `*Env` must not be mutated concurrently — guard it like any other mutable value. `Decoder`,
`Encoder` and `bind` share no state between instances, so parsing, encoding and binding from several
goroutines at once are safe.

## Version 1

`v1` is frozen. It is still fetchable by tag — `go get github.com/efureev/envi@v1.3.1` — but its
code no longer lives on the default branch, and it will receive no fixes. It has known defects,
including crashes in `Merge` and `RemoveRow`; they are listed in [docs/AUDIT.md](docs/AUDIT.md).

New code imports `github.com/efureev/envi/v2`. The `/v2` suffix stays in the import path even though
the code sits at the repository root — that is how Go identifies a major version.

## License

[MIT](LICENSE)
