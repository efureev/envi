package envi

import (
	"bytes"
	"io"
	"iter"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Severity ranks a [Problem].
type Severity int

const (
	// SeverityError marks a problem that makes the document invalid: the file
	// does not say what it appears to say, or does not parse at all.
	SeverityError Severity = iota

	// SeverityWarning marks a document that parses and means what it says but
	// carries something worth looking at.
	SeverityWarning
)

// String implements [fmt.Stringer].
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// A Rule names one check. The strings are stable across releases and are what
// [WithoutRules] takes, so they may be written down in a configuration file.
type Rule string

const (
	// RuleSyntax reports a line that could not be parsed. Source only.
	RuleSyntax Rule = "syntax"

	// RuleDuplicateKey reports a key given a live value twice, where the first
	// value is silently discarded. A commented-out alternative beside a live
	// value is a shadow, not a duplicate, and is not reported. Source only.
	RuleDuplicateKey Rule = "duplicate-key"

	// RuleKeyInvalid reports a key that no shell would accept as the name of an
	// environment variable, such as one starting with a digit.
	RuleKeyInvalid Rule = "key-invalid"

	// RuleKeyNotCanonical reports a key written in a form this package rewrites
	// on output, such as lower case or with hyphens. Source only.
	RuleKeyNotCanonical Rule = "key-not-canonical"

	// RuleEmptyValue reports a live row whose value is empty.
	RuleEmptyValue Rule = "empty-value"

	// RuleUnquotedValue reports a bare value holding a character that a shell or
	// another reader of the file may treat specially. Source only.
	RuleUnquotedValue Rule = "unquoted-value"
)

// bit returns the rule's place in a configuration's disabled-rule mask, or 0
// for a name this package does not know, which then disables nothing.
func (r Rule) bit() uint32 {
	switch r {
	case RuleSyntax:
		return 1 << 0
	case RuleDuplicateKey:
		return 1 << 1
	case RuleKeyInvalid:
		return 1 << 2
	case RuleKeyNotCanonical:
		return 1 << 3
	case RuleEmptyValue:
		return 1 << 4
	case RuleUnquotedValue:
		return 1 << 5
	default:
		return 0
	}
}

// A Problem is one finding.
type Problem struct {
	// Rule names the check that produced the finding.
	Rule Rule `json:"rule"`

	// Severity says whether the finding makes the document invalid.
	Severity Severity `json:"severity"`

	// Line is the 1-based line the finding sits on, and Col the 1-based byte
	// offset within it. Both are 0 when the finding is not tied to a position:
	// every finding from [Env.Check] is, since a document in memory has no
	// source, and only [RuleSyntax] carries a column.
	Line int `json:"line,omitempty"`
	Col  int `json:"col,omitempty"`

	// Key is the row the finding concerns, empty for findings that concern a
	// line rather than a row.
	Key string `json:"key,omitempty"`

	// Msg describes the finding in lower case, without position or severity:
	// "value is empty".
	Msg string `json:"message"`
}

// String renders the problem the way compilers and linters do, so that an
// editor or a CI log turns the position into a link.
//
//	4:12: error: syntax: unterminated quoted value
//	APP_NAME: warning: empty-value: value is empty
func (p Problem) String() string {
	var b []byte
	switch {
	case p.Line > 0 && p.Col > 0:
		b = strconv.AppendInt(b, int64(p.Line), 10)
		b = append(b, ':')
		b = strconv.AppendInt(b, int64(p.Col), 10)
		b = append(b, ": "...)
	case p.Line > 0:
		b = strconv.AppendInt(b, int64(p.Line), 10)
		b = append(b, ": "...)
	case p.Key != "":
		b = append(b, p.Key...)
		b = append(b, ": "...)
	}
	b = append(b, p.Severity.String()...)
	b = append(b, ": "...)
	b = append(b, p.Rule...)
	b = append(b, ": "...)
	b = append(b, p.Msg...)
	if p.Key != "" && p.Line > 0 {
		b = append(b, " ("...)
		b = append(b, p.Key...)
		b = append(b, ')')
	}
	return string(b)
}

// A Report is the outcome of a check: the findings, in the order they were
// made, which for a check over a source is the order of the file.
//
// The zero Report is a valid empty report, with every rule enabled.
type Report struct {
	problems []Problem

	// disabled is the mask from [WithoutRules], held here so that recording a
	// finding needs nothing else.
	disabled uint32
}

// newReport returns a report that will ignore the masked rules.
func newReport(disabled uint32) *Report { return &Report{disabled: disabled} }

// record appends p unless its rule is switched off.
func (r *Report) record(p Problem) {
	if r.disabled&p.Rule.bit() != 0 {
		return
	}
	r.problems = append(r.problems, p)
}

// OK reports whether the document is valid: no finding of severity
// [SeverityError]. Warnings do not make it false.
func (r *Report) OK() bool { return r.Count(SeverityError) == 0 }

// Len returns the number of findings.
func (r *Report) Len() int { return len(r.problems) }

// Count returns how many findings carry the given severity.
func (r *Report) Count(s Severity) int {
	n := 0
	for _, p := range r.problems {
		if p.Severity == s {
			n++
		}
	}
	return n
}

// All iterates the findings in the order they were made.
func (r *Report) All() iter.Seq[Problem] { return slices.Values(r.problems) }

// Err returns an error describing what makes the document invalid, or nil when
// [Report.OK] is true. Warnings never produce one.
//
// It is for the common shape of a caller that only wants to know whether to
// carry on; the report itself carries the detail.
func (r *Report) Err() error {
	if r.OK() {
		return nil
	}
	return &checkError{report: r}
}

// checkError carries a failed report behind the error interface.
type checkError struct{ report *Report }

func (e *checkError) Error() string {
	n := e.report.Count(SeverityError)
	var b strings.Builder
	b.WriteString("envi: ")
	b.WriteString(strconv.Itoa(n))
	if n == 1 {
		b.WriteString(" problem: ")
	} else {
		b.WriteString(" problems: ")
	}
	first := true
	for _, p := range e.report.problems {
		if p.Severity != SeverityError {
			continue
		}
		if !first {
			b.WriteString("; ")
		}
		first = false
		b.WriteString(p.String())
	}
	return b.String()
}

// Check reads a document and checks it in one pass.
//
// Unlike [Parse] it does not stop at the first malformed line: a syntax error
// becomes a [Problem] and the read carries on, so one call reports everything
// wrong with the file. The document is returned as far as it could be built,
// with unparsable lines kept verbatim so that writing it back does not delete
// them. The error is reserved for a failure of the underlying reader.
//
// Rules can be switched off with [WithoutRules].
func Check(r io.Reader, opts ...Option) (*Env, *Report, error) {
	cfg := newConfig(opts)
	rep := newReport(cfg.disabledRules)
	s := newScanner(r, cfg)
	s.check = true
	d := &Decoder{s: s, cfg: cfg, report: rep}

	env, err := d.Decode()
	if err != nil {
		return nil, nil, err
	}
	return env, rep, nil
}

// CheckBytes checks the document in data. See [Check].
func CheckBytes(data []byte, opts ...Option) (*Env, *Report, error) {
	return Check(bytes.NewReader(data), opts...)
}

// CheckString checks the document in s. See [Check].
func CheckString(s string, opts ...Option) (*Env, *Report, error) {
	return Check(strings.NewReader(s), opts...)
}

// CheckFile checks the named file. See [Check].
func CheckFile(path string, opts ...Option) (*Env, *Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	return Check(f, opts...)
}

// Check runs over a document already in memory the rules that need no source
// text: [RuleKeyInvalid] and [RuleEmptyValue].
//
// The rest — [RuleSyntax], [RuleDuplicateKey], [RuleKeyNotCanonical] and
// [RuleUnquotedValue] — describe how a file is written rather than what it
// holds, and a document that has been parsed no longer remembers that. Use
// [Check] on the source to run them.
func (e *Env) Check(opts ...Option) *Report {
	cfg := newConfig(opts)
	rep := newReport(cfg.disabledRules)
	for r := range e.Rows() {
		checkRow(rep, r.key, r.value, r.commented, 0)
	}
	return rep
}

// checkRow runs the content rules over one row. It is called from the parser,
// where the line number is known, and from [Env.Check], where it is not, so
// that both report the same things in the same order.
func checkRow(rep *Report, key, value string, commented bool, line int) {
	if rep == nil || commented {
		// A commented-out row is inert: nothing it says takes effect, so
		// nothing about it is worth a complaint.
		return
	}
	if !keyIsValid(key) {
		rep.record(Problem{
			Rule:     RuleKeyInvalid,
			Severity: SeverityError,
			Line:     line,
			Key:      key,
			Msg:      "key is not a usable environment variable name",
		})
	}
	if value == "" {
		rep.record(Problem{
			Rule:     RuleEmptyValue,
			Severity: SeverityWarning,
			Line:     line,
			Key:      key,
			Msg:      "value is empty",
		})
	}
}

// keyIsValid reports whether a normalised key names something a shell would
// accept as an environment variable. Normalisation has already restricted the
// bytes to [A-Z0-9._], so only the first one and the empty case are left to
// rule out.
func keyIsValid(key string) bool {
	if key == "" {
		return false
	}
	c := key[0]
	return (c < '0' || c > '9') && c != '.'
}
