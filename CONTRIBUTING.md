# Contributing

Thanks for taking the time. This is a small library with a narrow purpose, so the bar is less about
process and more about not breaking the two properties it sells: fidelity and speed.

## Layout

The library is at the repository root; `bind/` is the struct-binding subpackage. The import path
keeps its `/v2` suffix — that is how Go names a major version, not a directory.

`v1` is frozen and off the default branch, reachable only by its tags. Please do not resurrect it.

## Before opening a pull request

```shell
make check    # fmt, vet, lint, tests — the same order CI uses
```

or by hand:

```shell
go build ./...
go vet ./...
gofmt -l .                 # must print nothing
golangci-lint run ./...    # must report 0 issues
go test -race -shuffle=on ./...
```

`-shuffle=on` is not optional. It is what proves the package holds no order-dependent state — the
single worst defect of `v1`.

If you touched the parser or the encoder, also run the fuzzers for a minute:

```shell
go test -run Fuzz -fuzz FuzzParse -fuzztime 60s
go test -run Fuzz -fuzz FuzzRoundTrip -fuzztime 60s
```

Any input a fuzzer rejects is written to `testdata/fuzz/`. Commit it: it becomes a permanent
regression test.

## Things that will be asked of you

**Every exported symbol needs a doc comment.** The linter enforces it. `v1` shipped with an entirely
empty godoc page and we are not doing that again.

**Round-trip fidelity is a feature, not an optimisation.** A parsed document must come back byte for
byte under the default options. If a change cannot preserve that, say so explicitly in the pull
request rather than letting a golden test be updated quietly.

**No dependencies.** Neither at runtime nor in tests. If a test needs a helper, write the helper.

**Performance changes need numbers.** Parsing a 1000-line file costs 237 µs and 4 405 allocations;
writing it costs 22.6 µs and 5; a lookup costs 27.8 ns and none. Those are measured, not aspirational,
and they should not get worse. Run the benchmarks before and after and put `benchstat` output in the
pull request:

```shell
go test -run '^$' -bench . -benchmem -count 8 > new.txt
go run golang.org/x/perf/cmd/benchstat@latest old.txt new.txt
```

## Reporting a bug

A failing input is worth more than a description. If you can reduce it to a few lines of `.env` and
the output you expected, that is enough — it usually becomes a test verbatim.
