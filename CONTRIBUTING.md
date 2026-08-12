# Contributing

Thanks for taking the time. This is a small library with a narrow purpose, so the bar is less about
process and more about not breaking the two properties it sells: fidelity and speed.

## Layout

The library is at the repository root; `bind/` is the struct-binding subpackage. The import path
keeps its `/v2` suffix — that is how Go names a major version, not a directory.

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

`-shuffle=on` is not optional. It is what proves the package holds no order-dependent state. Two
callers in one process must never be able to disturb each other's output.

If you touched the parser or the encoder, also run the fuzzers for a minute each. Note the `$` — a
`-fuzz` pattern must match exactly one target, and `FuzzRoundTrip` unanchored also matches
`FuzzRoundTripRewritten`, which makes the command fail rather than fuzz:

```shell
go test -run Fuzz -fuzz FuzzParse -fuzztime 60s
go test -run Fuzz -fuzz 'FuzzRoundTrip$' -fuzztime 60s
go test -run Fuzz -fuzz 'FuzzModelSurvivesEncoding$' -fuzztime 60s
```

`make fuzz` runs all six targets for 30 s each, which is what CI does. Thirty seconds is a smoke
test: two of the four bugs the fuzzer has found here surfaced after the first minute.

Any input a fuzzer rejects is written to `testdata/fuzz/`. Commit it: it becomes a permanent
regression test.

## Things that will be asked of you

**Every exported symbol needs a doc comment.** The linter enforces it, and the godoc page is the
first thing anyone evaluating the library reads.

**Round-trip fidelity is a feature, not an optimisation.** A parsed document must come back byte for
byte under the default options, apart from the two cases the README names — a key given a live value
twice, and mixed line endings. If a change cannot preserve that, say so explicitly in the pull
request rather than letting a golden test be updated quietly.

**A green `FuzzRoundTrip` does not mean nothing was dropped.** It compares our output against our
output, so anything the encoder discards *consistently* looks like a fixed point. That is how a
repeated commented key silently lost a line for two releases. `FuzzModelSurvivesEncoding` is the
target that catches it: it reparses the output and compares the model — rows, values, commented
flags, shadows. If you add a field that affects output, ask whether that target would see it go
missing.

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
