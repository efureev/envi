package main

import (
	"errors"
	"fmt"
	"strings"

	envi "github.com/efureev/envi/v2"
)

// The editing commands name their file with -f rather than taking it as a
// trailing argument.
//
// For unset there is no way to tell a key from a path by looking at it:
// "envi unset APP_NAME config.env" could mean either, and a rule based on dots
// or slashes gets "envi unset app_name .env" wrong the other way. Guessing in
// argument parsing is how a tool deletes the wrong thing, so all three editing
// commands say it the same explicit way.

// cmdGet prints one configured value.
//
// A key that is only present commented out reads as unset, and the exit status
// says so: a script must not be handed a value the process will never see.
func cmdGet(args []string, s ioStreams) int {
	fs := newFlags("get", s)
	path := fs.String("f", defaultFile, "file to read")
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	keys := fs.Args()
	if len(keys) != 1 {
		return fail(s.err, errors.New("get needs exactly one key"))
	}

	e, err := readDoc(*path, s)
	if err != nil {
		return fail(s.err, err)
	}

	value, ok := configured(e, keys[0])
	if !ok {
		return exitFound
	}
	s.out.println(value)
	return exitOK
}

// cmdSet sets keys in place, leaving everything else in the file exactly as it
// was — which is the whole reason this command exists rather than a sed line.
func cmdSet(args []string, s ioStreams) int {
	fs := newFlags("set", s)
	path := fs.String("f", defaultFile, "file to edit")
	dry := fs.Bool("n", false, "print the result instead of writing the file")
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	pairs, err := parseAssignments(fs.Args())
	if err != nil {
		return fail(s.err, err)
	}

	e, err := readOrCreate(*path, s)
	if err != nil {
		return fail(s.err, err)
	}

	for _, kv := range pairs {
		// SetCommented(false) is not decoration. Env.Set edits whatever row it
		// finds and Row.SetValue leaves the commented flag alone, so setting a
		// key whose only row is "# KEY=old" would store the value in a row that
		// still configures nothing.
		e.Set(kv[0], kv[1]).SetCommented(false)
	}

	return writeResult(*path, e, *dry, s)
}

// cmdUnset removes keys in place. Removing a key that was not there is not an
// error, the same way it is not in a shell.
func cmdUnset(args []string, s ioStreams) int {
	fs := newFlags("unset", s)
	path := fs.String("f", defaultFile, "file to edit")
	dry := fs.Bool("n", false, "print the result instead of writing the file")
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	keys := fs.Args()
	if len(keys) == 0 {
		return fail(s.err, errors.New("unset needs at least one key"))
	}

	e, err := readDoc(*path, s)
	if err != nil {
		return fail(s.err, err)
	}
	for _, k := range keys {
		e.Delete(k)
	}

	return writeResult(*path, e, *dry, s)
}

// writeResult saves an edited document, or prints it when the caller asked not
// to touch the file. Editing what came from standard input has nowhere to write
// back to, so it prints too.
func writeResult(path string, e *envi.Env, dry bool, s ioStreams) int {
	if dry || path == stdinPath {
		s.out.print(e)
		return exitOK
	}
	if err := writeInPlace(path, e); err != nil {
		return fail(s.err, err)
	}
	return exitOK
}

// parseAssignments reads "KEY=VALUE" arguments.
func parseAssignments(args []string) ([][2]string, error) {
	if len(args) == 0 {
		return nil, errors.New("set needs at least one KEY=VALUE")
	}
	pairs := make([][2]string, 0, len(args))
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", arg)
		}
		if key == "" {
			return nil, fmt.Errorf("assignment with no key: %q", arg)
		}
		pairs = append(pairs, [2]string{key, value})
	}
	return pairs, nil
}
