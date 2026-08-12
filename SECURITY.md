# Security Policy

## Supported versions

| Version | Supported |
|---|---|
| `v2.x` (`github.com/efureev/envi/v2`) | yes |
| `v1.x` | no — frozen, with known defects listed in [docs/AUDIT.md](docs/AUDIT.md) |

## Reporting a vulnerability

Please report privately through
[GitHub's security advisory form](https://github.com/efureev/envi/security/advisories/new) rather
than opening a public issue.

Include the input that triggers the problem and the version you used. A minimal `.env` fragment is
usually enough.

## Scope

This library parses untrusted text, so the interesting cases are:

- input that makes the parser panic, hang, or consume memory out of proportion to its size;
- input that round-trips into something with a different meaning — a value that reads back as a
  different value, a comment that reads back as a live assignment, or the reverse.

The parser is fuzzed continuously against the first class and for round-trip idempotence against the
second. A reproducing input for either is a genuine finding.

## Out of scope

`.env` files hold secrets, but this library only reads and writes them. Protecting the file itself —
permissions, encryption, keeping it out of version control — is the responsibility of the program
using it. `envi` never logs values and never sends them anywhere.
