package main

import (
	"strings"
)

// cmdExport prints shell statements that set what the file configures:
//
//	eval "$(envi export .env)"
//
// This is what "source .env" is reached for and cannot do. A .env file is not a
// shell script: an unquoted value holding a space, a hash or a quote either
// fails to parse or means something else once the shell has had it. Going
// through the parser and quoting the result properly is the difference.
func cmdExport(args []string, s ioStreams) int {
	fs := newFlags("export", s)
	noExport := fs.Bool("n", false, "write assignments without the export keyword")
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	path, err := pathArg(fs.Args())
	if err != nil {
		return fail(s.err, err)
	}
	e, err := readDoc(path, s)
	if err != nil {
		return fail(s.err, err)
	}

	keyword := "export "
	if *noExport {
		keyword = ""
	}

	for _, kv := range configuredPairs(e) {
		key, value := kv[0], kv[1]
		if !isShellName(key) {
			// Emitting it would make the whole eval a syntax error, taking the
			// valid assignments down with it. Saying so beats a silently
			// incomplete environment.
			warnf(s.err, "envi: skipping %s: not a usable shell variable name\n", key)
			continue
		}
		s.out.printf("%s%s=%s\n", keyword, key, shellQuote(value))
	}
	return exitOK
}

// cmdJSON prints what the file configures as a JSON object, for jq and for
// anything else that would rather not parse .env itself.
func cmdJSON(args []string, s ioStreams) int {
	fs := newFlags("json", s)
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	path, err := pathArg(fs.Args())
	if err != nil {
		return fail(s.err, err)
	}
	e, err := readDoc(path, s)
	if err != nil {
		return fail(s.err, err)
	}

	// A map, so that encoding/json sorts the keys and the output is the same
	// every run — which matters the moment it is committed or diffed.
	out := make(map[string]string, e.Len())
	for _, kv := range configuredPairs(e) {
		out[kv[0]] = kv[1]
	}
	if err := writeJSON(s.out, out); err != nil {
		return fail(s.err, err)
	}
	return exitOK
}

// shellQuote wraps a value in single quotes, which is the only form the shell
// leaves entirely alone: no expansion, no escapes, nothing special but the
// closing quote itself.
//
// A single quote inside therefore cannot be escaped — it has to end the string,
// contribute an escaped quote, and start a new one. That is the '\” dance, and
// it is why this is worth a function rather than a Sprintf.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// isShellName reports whether key can name a shell variable.
//
// Keys here are normalised to upper case letters, digits, dots and underscores,
// and the shell allows no dots and no leading digit — so a key like A.B is
// perfectly good in a .env file and impossible to export.
func isShellName(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '_':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
