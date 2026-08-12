package main

import (
	"fmt"
	"strings"

	envi "github.com/efureev/envi/v2"
)

// cmdCheck reports everything wrong with the given files.
//
// Unlike parsing, checking does not stop at the first malformed line, so one
// run tells the whole story. Findings carry the file they came from, in the
// form an editor or a CI log turns into a link.
func cmdCheck(args []string, s ioStreams) int {
	fs := newFlags("check", s)
	asJSON := fs.Bool("json", false, "write the findings as JSON")
	strict := fs.Bool("strict", false, "fail on warnings as well as errors")
	off := fs.String("off", "", "comma-separated rules to switch off")
	if err := fs.Parse(args); err != nil {
		return exitFailure
	}

	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{defaultFile}
	}

	var opts []envi.Option
	if rules := parseRules(*off); len(rules) > 0 {
		opts = append(opts, envi.WithoutRules(rules...))
	}

	found := false
	var collected []jsonFinding

	for _, path := range paths {
		in, err := openReader(path, s)
		if err != nil {
			return fail(s.err, err)
		}
		_, report, err := envi.Check(in, opts...)
		closeReader(in)
		if err != nil {
			return fail(s.err, fmt.Errorf("%s: %w", path, err))
		}

		if !report.OK() || (*strict && report.Len() > 0) {
			found = true
		}

		for p := range report.All() {
			if *asJSON {
				collected = append(collected, jsonFinding{
					File:     path,
					Rule:     string(p.Rule),
					Severity: p.Severity.String(),
					Line:     p.Line,
					Col:      p.Col,
					Key:      p.Key,
					Message:  p.Msg,
				})
				continue
			}
			s.out.printf("%s:%s\n", path, p)
		}
	}

	if *asJSON {
		if err := writeJSON(s.out, nonNil(collected)); err != nil {
			return fail(s.err, err)
		}
	}

	if found {
		return exitFound
	}
	return exitOK
}

// A jsonFinding is one finding with the file it came from, which the library's
// Problem does not carry: it checks one document at a time and does not know
// where the document came from.
type jsonFinding struct {
	File     string `json:"file"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	Key      string `json:"key,omitempty"`
	Message  string `json:"message"`
}

// parseRules splits the -off value. Unknown names are passed through: the
// library ignores what it does not recognise, so a configuration written for a
// later version stays usable.
func parseRules(list string) []envi.Rule {
	if list == "" {
		return nil
	}
	parts := strings.Split(list, ",")
	rules := make([]envi.Rule, 0, len(parts))
	for _, p := range parts {
		if name := strings.TrimSpace(p); name != "" {
			rules = append(rules, envi.Rule(name))
		}
	}
	return rules
}

func nonNil(f []jsonFinding) []jsonFinding {
	if f == nil {
		return []jsonFinding{}
	}
	return f
}
