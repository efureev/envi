# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`Env.Regroup` and `Env.Tidy`.** Regrouping gathers every key sharing a prefix into one block,
  wherever those keys sit in the document, and dissolves a block that has fallen below the grouping
  threshold; `Tidy` regroups and then sorts. Until now grouping happened only during parsing, and
  only for rows that were already adjacent, so a document could not be rearranged after the fact.
- **`Check`, `CheckBytes`, `CheckString`, `CheckFile` and `Env.Check`.** Reading and validating in
  one pass, reporting every problem rather than stopping at the first. Six rules — `syntax`,
  `duplicate-key`, `key-invalid`, `key-not-canonical`, `empty-value`, `unquoted-value` — carried by
  the new `Report`, `Problem`, `Rule` and `Severity` types. `Report.Text` renders findings the way
  compilers do, `Report.JSON` as objects, `Report.Err` as one error.
- `WithoutRules`, switching checks off by name. An unknown name switches nothing off, so a
  configuration written for a later version stays usable.
- Fuzz targets `FuzzCheck`, `FuzzRegroup` and `FuzzModelSurvivesEncoding`. The last asserts that
  writing a document says everything the document holds — the property `FuzzRoundTrip` cannot see,
  since it compares our output against our output and anything the encoder drops consistently looks
  like a fixed point.

### Fixed

- **`WithOrder(OrderSorted)` now sorts the rows inside each block**, not only the top-level items,
  which is what `Env.SortByKey` always did. Sorted output is also written from the model rather than
  reproduced verbatim: a row's recorded lines describe the neighbours it had before sorting moved
  everything. **Output bytes change for callers using this option.**
- **A repeated commented-out key no longer swallows the lines above it.** Folding `# K=v` into an
  earlier statement of the same key discarded that statement's recorded prefix, so a blank line or a
  comment sitting there vanished from the file — text belonging to no other row, and therefore lost
  outright.
- **A key stated twice in comments no longer loses a line.** `# K=1` followed by `# K=2`, with no
  live `K`, came back as `# K=1` alone: the second statement folded into the first as a shadow, and
  a commented row wrote none of its shadows on the reasoning that an inert row has nothing to
  shadow. A commented row's shadows are now written *below* it, which is where they were read — a
  live row absorbs shadows from the lines above it, and nothing absorbs them upwards into an inert
  row. Behaviour change for anyone who built such a row by hand with `Row.AddShadow`.

## [2.0.0] — 2026-08-12

A rewrite. The domain model — blocks, rows and shadows — is the only thing carried over. The import
path is `github.com/efureev/envi/v2`, and the code lives at the repository root; the `/v2` suffix is
what identifies the major version, not a directory.

There is no migration path by design: every entry point has a different name or signature. The
reason is in the Fixed section below — the defects that mattered could not be repaired without
breaking the API they were built into.

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
- **`Row` is exported.** Exported functions no longer hand back an unexported type that callers
  cannot name.
- **Parsing is a hand-written scanner.** No regular expressions, and no 64 KiB line limit.
- **Lookups are O(1)**, and allocate nothing.
- **`Save` is atomic**: it writes a temporary file and renames it, so an interrupted run cannot
  leave a truncated `.env` behind.
- Minimum Go version is 1.26.

### Fixed

Every defect below was reproduced through the public API before being fixed, and each is covered by
a regression test named after its identifier.

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

The budget this release established, and which later releases are held to. 1000-line document,
Go 1.26.5, Apple M5 Pro:

| | Time | Allocations |
|---|---:|---:|
| Parse | 240.8 µs | 4 405 |
| Write | 22.6 µs | 5 |
| Lookup | 30.7 ns | 0 |
