# envi

[![Go package](https://github.com/efureev/envi/actions/workflows/go.yml/badge.svg)](https://github.com/efureev/envi/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/efureev/envi/v2.svg)](https://pkg.go.dev/github.com/efureev/envi/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/envi/v2)](https://goreportcard.com/report/github.com/efureev/envi/v2)
![Go version](https://img.shields.io/github/go-mod/go-version/efureev/envi)
![Zero dependencies](https://img.shields.io/badge/dependencies-none-success)
![License](https://img.shields.io/github/license/efureev/envi)

> Also available in [Russian](Readme.ru.md). This English version is the canonical one.

### The `.env` library that gives you your file back.

Every `.env` library can read a file. Almost none can **write one back**. Load a config into a map, dump it to disk, and
your carefully organised file comes back sorted, unquoted, uncommented — a stranger. So nobody edits `.env` files
programmatically. They edit them by hand, forever.

`envi` models the document, not just its values. Comments, sections, blank lines, quote style, even CRLF endings survive
the trip. Change one key, get a one-line diff.

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

That difference is the entire point of this library, and it is held in place by property tests that run on every push:
our own output always reparses to the identical bytes, and reading a document back always yields the same rows, values
and shadows it held — so nothing can go quietly missing on the way out. Both are continuously fuzzed.

A well-formed file comes back byte for byte. Two inputs are rearranged rather than reproduced, and the library says so
rather than pretending otherwise: a key given a live value **twice**, where the second wins and the first becomes a
shadow above it — `Check` reports this as an error — and a file mixing **CRLF and LF**, which is normalised to the first
ending seen. Nothing is lost in either.

---

## Install

```shell
go get github.com/efureev/envi/v2
```

```go
import envi "github.com/efureev/envi/v2"
```

Requires Go 1.26. **Zero dependencies** — in the library and in its test suite.

The `/v2` suffix stays in the import path even though the code sits at the repository root. That is simply how Go names
a major version, not a directory you need to look for.

---

## Two ways to use it

### 1. Just give me my config, typed

Most programs want this and nothing else. The `bind` subpackage is self-contained: it can be the only part of `envi` you
import.

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

Files load in order, each overriding the last; the process environment overrides them all. Missing optional files are
fine — the container case, where everything arrives through the environment.

**Errors arrive all at once**, not one per run:

```
bind: 2 fields failed:
  - Port (APP_PORT): strconv.ParseInt: parsing "eighty": invalid syntax
  - DB.Host (DB_HOST): bind: required value is missing
```

Out of the box: every sized integer and float, `bool`, `string`, `time.Duration`, slices, maps, pointers, nested
structs, `[]byte`, and anything implementing `encoding.TextUnmarshaler` — which quietly covers `time.Time`, `net.IP` and
most ID types. No tag? The key comes from the field name:
`AppName` → `APP_NAME`, `HTTPPort` → `HTTP_PORT`.

Anything else takes one line. `url.URL` is the type everyone hits first: it has no `UnmarshalText`, and you cannot give
it one from outside its package.

```go
bind.Load(&cfg, bind.WithConverter(url.Parse))
```

That covers `*url.URL` fields, and `[]*url.URL`, and `map[string]*url.URL`, with no further registration. A registered
type wins over its own `UnmarshalText` too, so a type whose text form is not what a config file should carry can be read
differently here without being changed.

Need several? Pass one per type. Three cost no more than one — what a converter suspends, caching the type's plan, is
suspended by the first and not again by the rest.

```go
// net.ParseCIDR returns three values, so it does not fit func(string) (T, error).
// Wrapping it is the whole adaptation.
parseCIDR := func (s string) (*net.IPNet, error) {
_, network, err := net.ParseCIDR(s)
return network, err
}

err := bind.Decode(src, &cfg,
bind.WithConverter(url.Parse),
bind.WithConverter(parseCIDR),
bind.WithConverter(parseSeverity), // a type of your own, no method needed
)
```

Each failing converter still reports its own field, so one run shows the whole picture:

```
bind: 2 fields failed:
  - Endpoint (ENDPOINT): parse "://nope": missing protocol scheme
  - LogLevel (LOG_LEVEL): unknown severity "shout"
```

Check before you write one, though — plenty of types already read themselves. `regexp.Regexp` does;
`url.URL`, `net.IPNet` and `time.Location` do not.

Worth knowing what it saves you from: **without** a converter such a field is not left empty — it is taken apart, and
its own key ignored. A `*url.URL` field named `ENDPOINT` gets assembled from
`ENDPOINT_SCHEME` and `ENDPOINT_HOST`, quietly, with no error.

### 2. Treat the file as a document

```go
env, err := envi.Load(".env", ".env.local") // merge, later wins

env.Set("APP_PORT", "8080")                   // create or update
env.Delete("APP_DEBUG")
env.Get("APP_URL").SetComment("staging only")

envi.Save(env, ".env") // atomic write
```

Useful for the things nobody writes tooling for today: installers that seed a `.env`, CLIs that toggle a feature flag,
CI checks that diff `.env` against `.env.example`, migrations that rename a key across a fleet of repositories.

---

## Tidy a file that got away from you

Real `.env` files drift. Keys land wherever the last hurried edit put them, prefixes end up scattered, sections lose
their members. `Regroup` gathers every key sharing a prefix into one block, wherever those keys sat; `Tidy` does that
and sorts.

```dotenv
###   ---[ Application ]---   ###
APP_NAME=one
DB_HOST=localhost
APP_DEBUG=false
DB_PORT=5432
LONE=x
```

```go
env, _ := envi.Load(".env")
env.Tidy()
envi.Save(env, ".env")
```

```dotenv
###   ---[ Application ]---   ###
APP_DEBUG=false
APP_NAME=one

DB_HOST=localhost
DB_PORT=5432

LONE=x
```

Section headers survive the move, and so do row comments and shadows. How many rows it takes to form a block is yours to
choose: `env.Regroup(envi.WithGroupThreshold(3))` dissolves both blocks above, since neither has three rows, and demotes
the header to an ordinary comment on the first row it introduced rather than dropping it.

A row that moves gives up its byte-for-byte rendering: the blank lines and comments recorded above it described where it
used to be. A document already in order is left untouched and still writes back identical, so calling `Regroup` before a
save costs nothing when there is nothing to do.

---

## Check a file before you trust it

`Check` reads and validates in one pass. Unlike `Parse` it does not stop at the first malformed line — one call tells
you everything wrong with the file.

```go
env, report, err := envi.CheckFile(".env") // err is only for I/O
if !report.OK() {
report.Text(os.Stderr)
os.Exit(1)
}
```

```
1: warning: key-not-canonical: key is written as "app-name" (APP_NAME)
2:9: error: syntax: unterminated quoted value
4: error: duplicate-key: key is already defined on line 3, and that value is discarded (APP_URL)
5: warning: empty-value: value is empty (EMPTY)
6: warning: unquoted-value: bare value holds '$' (PASS)
7: error: key-invalid: key is not a usable environment variable name (1BAD)
```

Line and column, in the form editors and CI logs turn into links. `report.JSON(w)` writes the same findings as objects;
`report.Err()` collapses them into one error for callers that only want to know whether to carry on.

| Rule                | Severity | What it catches                                                 |
|---------------------|----------|-----------------------------------------------------------------|
| `syntax`            | error    | a line that does not parse — all of them, not just the first    |
| `duplicate-key`     | error    | a key given a live value twice, silently discarding the first   |
| `key-invalid`       | error    | a name no shell would accept, such as one starting with a digit |
| `key-not-canonical` | warning  | lower case, hyphens — anything rewritten on output              |
| `empty-value`       | warning  | a live row with nothing on the right of the `=`                 |
| `unquoted-value`    | warning  | a bare value holding `$`, `` ` ``, a quote or a backslash       |

A commented-out alternative beside a live value is a *shadow*, an idiom this format is built around, and is never
reported as a duplicate. Rules switch off by name: `envi.WithoutRules(envi.RuleEmptyValue)`.

The document comes back too, unparsable lines and all, so checking a file and writing it back never deletes what it
could not understand. For a document already in memory, `env.Check()` runs the rules that do not need the source text.

---

## Compare two files

`Diff` answers what changed in the **configuration** — not what changed in the file. For the second question you already
have `git diff`.

```go
before, _ := envi.Load(".env.bak")
after, _ := envi.Load(".env")

fmt.Print(before.Diff(after))
```

```
- APP_DEBUG="false"
~ APP_URL: "https://a.example" -> "https://b.example"
+ APP_PORT="8080"
```

Comments, shadows, quote style, block membership and order produce nothing: they are how a document is written, not what
it configures. Values are quoted, because an empty value is otherwise indistinguishable from an unchanged one.

The result is a `*Delta`, the same shape `Check` returns — `All()`, `Len()`, `Count(kind)`,
`Empty()`, `Text(w)`, `JSON(w)`. Which is what makes the CI check the feature was built for a handful of lines:

```go
for c := range example.Diff(actual).All() {
switch c.Kind {
case envi.ChangeRemoved:
fmt.Println("missing from .env:", c.Key) // documented but not set
case envi.ChangeAdded:
fmt.Println("undocumented:", c.Key) // set but not in the example
case envi.ChangeChanged:
// A real value differing from the placeholder is the point.
}
}
```

One thing worth knowing: **a commented-out row configures nothing**, so commenting a key out reads as a removal. That is
the view `Export` takes. It is deliberately not the view `Lookup` takes, which still hands back a commented row's
value — comparing configurations is the question `Diff` answers.

---

## Use it from the shell

```shell
go install github.com/efureev/envi/v2/cmd/envi@latest
```

Same document model, same guarantees, from CI and shell scripts — where the usual tools are wrong for the job. `sed`
knows nothing about quoting or comments; `source .env` is not valid shell the moment a value holds a space or a hash.

| Command           | What it does                                                                                                        |
|-------------------|---------------------------------------------------------------------------------------------------------------------|
| `envi fmt`        | Canonicalise. `-w` in place, `-l` list what would change, `-check` exit 1 if anything would, `-sort` order keys too |
| `envi check`      | Report every problem in one pass. `-json`, `-strict`, `-off rule,rule`                                              |
| `envi diff a b`   | Compare what two files configure. Exit 1 if they differ. `-json`                                                    |
| `envi get KEY`    | Print one configured value. Exit 1 if it is not set                                                                 |
| `envi set K=V…`   | Edit in place, leaving the rest of the file alone. `-n` to preview                                                  |
| `envi unset KEY…` | Remove keys in place                                                                                                |
| `envi export`     | Shell statements for `eval "$(envi export .env)"`                                                                   |
| `envi json`       | The configuration as a JSON object, for `jq`                                                                        |

With no file a command reads `.env`; `-` means stdin. Editing commands name their file with `-f`, because in
`envi unset APP_NAME config.env` there is no telling a key from a path by looking at it.

A CI gate is two lines:

```shell
envi fmt -check .env.example || { echo "run: envi fmt -w .env.example"; exit 1; }
envi diff .env.example .env   # exit 1 lists the keys that drifted
```

And the thing `source .env` cannot do:

```shell
eval "$(envi export .env)"     # values with spaces, hashes and quotes all survive
```

Exit codes are the unix ones: `0` nothing to report, `1` found what it was asked to look for, `2`
could not run — so CI can tell a bad config from a mistyped path.

One rule runs through every command: a commented-out row configures nothing. `envi get K` on a file holding `# K=1`
exits 1, because handing a script a value the process will never see is worse than saying nothing.

## What you get that a map doesn't

|                |                                                                                       |
|----------------|---------------------------------------------------------------------------------------|
| **Comments**   | Section headers, per-row comments and trailing comments, each kept where it was       |
| **Shadows**    | `# REDIS_HOST=redis` next to the live value is a first-class concept, not a lost line |
| **Order**      | Source order preserved; sorting is something you ask for, never a side effect         |
| **Blocks**     | Rows sharing a prefix group into a `Block` you can address as a unit                  |
| **Formatting** | Quote style, spacing, blank lines, CRLF vs LF — all reproduced                        |
| **Tidying**    | `Regroup` and `Tidy` put a drifted file back in order, comments and all               |
| **Validation** | `Check` reports every problem in one pass, with line and column                       |
| **Positions**  | A malformed line reports its line *and* column, not "invalid format"                  |
| **No limits**  | No 64 KiB line cap; a base64 certificate in one value parses fine                     |

```go
row := env.Get("REDIS_HOST")
row.Value()   // "127.0.0.1"
row.Comment() // "for docker"
for alt := range row.Shadows() {
fmt.Println(alt) // redis
}
```

---

## Fast, and measured

The parser is a hand-written single-pass scanner. No regular expressions anywhere in the module.

1000-line file, Go 1.26, Apple M5 Pro, `benchstat`, `-count 8`:

| Operation             |                   Time | Allocations |
|-----------------------|-----------------------:|------------:|
| Parse                 | **237 µs** (134 MiB/s) |       4 405 |
| Write                 |            **22.6 µs** |           5 |
| Lookup                |            **27.8 ns** |       **0** |
| Bind 50 struct fields |             **1.0 µs** |           2 |

Lookups are O (1) and allocation-free. Binding resolves each type's layout once and caches it, so the thousandth config
costs the same as the second.

The absence of `regexp` is a measured decision, not a stylistic one: compiling a pattern to classify a single line costs
5.5 µs and 146 allocations, against 4.5 ns and none for scanning the same line by hand. That is the whole gap, and it is
why the scanner is written the way it is.

---

## Built to be trusted with your config

- **Zero dependencies.** Nothing to audit, nothing to update, no supply chain.
- **Continuously fuzzed.** Seven fuzz targets run in CI: the parser never panics, its own output always parses back to
  the same document, **writing a document never drops anything it holds**, tidying never changes what a document says,
  and the checker agrees with the parser about what the format allows. Every input a fuzzer ever rejected is committed
  as a permanent regression test.
- **Race-tested and order-shuffled.** `go test -race -shuffle=on` on Linux, macOS and Windows. The package holds no
  mutable global state, so two libraries in one process cannot fight over settings.
- **Atomic writes.** `Save` writes a temporary file and renames it. An interrupted run cannot leave you with half a
  `.env`.
- **Documented.** Every exported symbol carries a doc comment, enforced by the linter.

---

## API tour

```go
// Read
env, err := envi.Parse(r) // any io.Reader
env, err := envi.ParseString(s)
env, err := envi.ParseBytes(b)
env, err := envi.Load(".env", ".env.local")

// Write
env.WriteTo(w)    // io.WriterTo
env.MarshalText() // encoding.TextMarshaler
envi.Save(env, ".env")

// Look up — O(1), keys normalised, so spelling does not matter
env.Lookup("app-port") // "8080", true
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
env.DeleteBlock("APP")
env.Merge(other)
env.Export(true) // into os.Environ

// Arrange
env.SortByKey() // sort, leave grouping alone
env.Regroup() // gather scattered prefixes into blocks
env.Tidy()    // regroup, then sort

// Check
env, report, err := envi.CheckFile(".env") // every problem, not just the first
report.OK()                                // no errors, warnings allowed
report.Text(os.Stderr)
report.JSON(w)
report.Err()                                    // nil, or one error naming them all
env.Check()                                     // rules that need no source text

// Compare — what changed in the configuration, not in the file
d := before.Diff(after) // *Delta
d.Empty() // the two configure the same thing
d.Count(envi.ChangeAdded)
d.Text(os.Stderr)
for c := range d.All() { } // Kind, Key, Old, New
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

An `*Env` is a mutable document — guard it like any other. `Decoder`, `Encoder` and `bind` share no state between
instances, so parsing, writing and binding from many goroutines at once is safe.

---

## Contributing

Bug reports with a failing `.env` fragment are worth their weight; they usually become a test verbatim.
See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
