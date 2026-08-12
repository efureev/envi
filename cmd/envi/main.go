// Command envi reads, checks and edits .env files from the shell.
//
// Install it with:
//
//	go install github.com/efureev/envi/v2/cmd/envi@latest
//
// It exists because .env files are edited from CI and shell scripts, where the
// usual tools are wrong for the job: sed does not know about quoting or
// comments, and "source .env" is not valid shell for a value holding a space or
// a hash. Every command here goes through the same document model the library
// uses, so a one-key edit produces a one-line diff.
//
// # Commands
//
//	fmt      canonicalise a file: group keys sharing a prefix into blocks
//	check    report everything wrong with a file, in one pass
//	diff     compare what two files configure
//	get      print one configured value
//	set      set keys in place, leaving the rest of the file alone
//	unset    remove keys in place
//	export   print shell statements for eval "$(envi export .env)"
//	json     print the configuration as a JSON object
//
// With no file argument a command reads ".env", the same default [envi.Load]
// takes. A file argument of "-" means standard input.
//
// # What "configured" means
//
// A commented-out row configures nothing, so get, export, json and diff pass
// over it. That is the view [envi.Env.Export] takes, and deliberately not the
// one [envi.Env.Lookup] takes, which hands back a commented row's value: a
// script asking for a key must not be given a value the process will never see.
//
// # Exit codes
//
//	0  nothing to report
//	1  found what it was asked to look for: check found an error, diff found a
//	   difference, fmt -check found an unformatted file, get found no value
//	2  the command could not run: bad usage, missing file, unreadable input
package main

import (
	"io"
	"os"
	"runtime/debug"
)

// Exit codes. Anything a caller scripts against lives here rather than being
// spelled out at each return.
const (
	exitOK      = 0
	exitFound   = 1
	exitFailure = 2
)

func main() {
	s := ioStreams{in: os.Stdin, out: &outWriter{w: os.Stdout}, err: os.Stderr}
	os.Exit(run(os.Args[1:], s))
}

// run is the whole program. Keeping it out of main, with the streams passed in,
// is what lets the tests drive every command in process rather than building a
// binary and shelling out to it.
func run(args []string, s ioStreams) int {
	code := dispatch(args, s)

	// One place to notice that the output never arrived — a closed pipe or a
	// full disk must not look like success.
	if s.out.err != nil {
		warnf(s.err, "envi: %v\n", s.out.err)
		return exitFailure
	}
	return code
}

func dispatch(args []string, s ioStreams) int {
	if len(args) == 0 {
		usage(s.err)
		return exitFailure
	}

	name, rest := args[0], args[1:]
	switch name {
	case "fmt":
		return cmdFmt(rest, s)
	case "check":
		return cmdCheck(rest, s)
	case "diff":
		return cmdDiff(rest, s)
	case "get":
		return cmdGet(rest, s)
	case "set":
		return cmdSet(rest, s)
	case "unset":
		return cmdUnset(rest, s)
	case "export":
		return cmdExport(rest, s)
	case "json":
		return cmdJSON(rest, s)
	case "help", "-h", "--help":
		usage(s.out)
		return exitOK
	case "version", "-version", "--version":
		s.out.println(version())
		return exitOK
	default:
		warnf(s.err, "envi: unknown command %q\n\n", name)
		usage(s.err)
		return exitFailure
	}
}

// version reports the module version the binary was built from. A binary built
// with "go build" from a checkout has none, which is what "(devel)" means.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "envi (devel)"
	}
	return "envi " + info.Main.Version
}

const usageText = `envi reads, checks and edits .env files.

usage: envi <command> [flags] [arguments]

commands:
  fmt      canonicalise a file: group keys sharing a prefix into blocks
  check    report everything wrong with a file, in one pass
  diff     compare what two files configure
  get      print one configured value
  set      set keys in place, leaving the rest of the file alone
  unset    remove keys in place
  export   print shell statements for eval "$(envi export .env)"
  json     print the configuration as a JSON object
  version  print the version

With no file argument a command reads ".env". A file of "-" means stdin.
A commented-out row configures nothing, so get, export, json and diff skip it.

exit codes:
  0  nothing to report
  1  found what it was asked to look for
  2  the command could not run

Run "envi <command> -h" for the flags of one command.
`

func usage(w io.Writer) {
	_, _ = io.WriteString(w, usageText)
}

// fail reports an error the way every command reports one, so that the prefix
// and the exit code cannot drift apart.
func fail(stderr io.Writer, err error) int {
	warnf(stderr, "envi: %v\n", err)
	return exitFailure
}
