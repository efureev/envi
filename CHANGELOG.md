# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] — v2

`v2` is a rewrite. It shares the domain model of `v1` — blocks, rows and shadows — and nothing else.
The import path is `github.com/efureev/envi/v2`, and the code lives at the repository root; the
`/v2` suffix is what identifies the major version, not a directory.

`v1` is frozen and no longer on the default branch. It stays fetchable by its tags, up to `v1.3.1`.

There is no migration path by design: every entry point has a different name or signature. See
[docs/AUDIT.md](docs/AUDIT.md) for why, and [docs/UPGRADE-SPEC.md](docs/UPGRADE-SPEC.md) for what is
still open and where this is going.

### Added

- `bind` subpackage: fills a configuration struct from `.env` files and the environment, with tags
  for defaults, required values and separators. Reports every failing field at once.
- Byte-for-byte round trip: a parsed document is written back exactly as it was read, including
  comment spacing, quote style, blank lines and CRLF endings.
- `SyntaxError` carrying the line and column of a malformed line.
- Iterators `Env.Items`, `Env.Rows`, `Env.All`, `Block.Rows` and `Row.Shadows`.
- `Env.WriteTo` (`io.WriterTo`) and `Env.MarshalText` (`encoding.TextMarshaler`).
- `Row.InlineComment`: a comment trailing an assignment is kept where it was, because moving it to a
  line of its own would turn it into a shadow.
- Fuzz targets for parsing and for round-trip idempotence.

### Changed

- **Configuration is per-operation, not global.** Every knob is an `Option` passed to `NewDecoder`
  or `NewEncoder`. The package holds no mutable global state, so two callers in one process no
  longer disturb each other, and encoding is safe from several goroutines.
- **Source order is preserved.** Reading a document no longer sorts it; `Env.SortByKey` does that
  when asked. A read-modify-write cycle now produces a diff limited to what changed.
- **`Row` is exported.** `v1` returned an unexported `*row` from exported functions, which callers
  could not name.
- **Parsing is a hand-written scanner.** No regular expressions, and no 64 KiB line limit.
- **Lookups are O(1)**, and allocate nothing.
- **`Save` is atomic**: it writes a temporary file and renames it, so an interrupted run cannot
  leave a truncated `.env` behind.
- Minimum Go version is 1.26.

### Fixed

Every defect below is catalogued in [docs/AUDIT.md](docs/AUDIT.md) and covered by a regression test
named after it.

- **C1** — `Merge` panicked on any key absent from the receiver.
- **C2** — `RemoveRow` panicked when no block carried the key's prefix.
- **C3** — merging files whose keys differed in case silently dropped the earlier comment.
- **C4** — a row whose key did not match a block's prefix vanished without an error; it is now
  `ErrPrefixMismatch`.
- **H1** — a top-level row became unreachable once a block of the same prefix existed.
- **H2** — an empty incoming value erased a value that was already set.
- **H3** — reading a document reordered it.
- **H4** — values were requoted on write, so a round trip was never byte-identical.
- **H5** — configuration lived in package globals, so concurrent use was a data race.

### Performance

Measured against `v1` on the same machine, 1000-line document, Go 1.26.5, Apple M5 Pro:

| | v1 | v2 |
|---|---:|---:|
| Parse | 11.27 ms | 240.8 µs |
| Parse allocations | 295 687 | 4 405 |
| Write | 568.9 µs | 22.6 µs |
| Write allocations | 10 914 | 5 |
| Lookup | 1.362 µs | 30.7 ns |

---

## v1

`v1` is frozen. Its history is in the repository's git log; releases `v1.0.0` through `v1.3.1` were
published without a changelog.
