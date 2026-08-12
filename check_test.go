package envi_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// rulesOf lists the rules a report fired, in order, so a test can name what it
// expects without pinning down the wording of every message.
func rulesOf(r *envi.Report) []envi.Rule {
	var got []envi.Rule
	for p := range r.All() {
		got = append(got, p.Rule)
	}
	return got
}

// linesOf lists the line each finding sits on, in order.
func linesOf(r *envi.Report) []int {
	var got []int
	for p := range r.All() {
		got = append(got, p.Line)
	}
	return got
}

func TestCheckAcceptsAValidDocument(t *testing.T) {
	t.Parallel()

	env, rep, err := envi.CheckString(readmeExample)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.OK() {
		t.Errorf("OK = false, want true: %v", rep)
	}
	if rep.Len() != 0 {
		t.Errorf("Len = %d, want 0:\n%s", rep.Len(), rep)
	}
	if err := rep.Err(); err != nil {
		t.Errorf("Err = %v, want nil", err)
	}
	if env.Len() == 0 {
		t.Error("Check returned an empty document")
	}
}

// The whole point of checking rather than parsing: one pass reports everything
// wrong with the file, not just the first thing.
func TestCheckCollectsEverySyntaxError(t *testing.T) {
	t.Parallel()

	const src = `GOOD=1
K="unterminated
ALSO_GOOD=2
nonsense
STILL_GOOD=3
L="a" trailing
`

	_, rep, err := envi.CheckString(src)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	want := []envi.Rule{envi.RuleSyntax, envi.RuleSyntax, envi.RuleSyntax}
	if got := rulesOf(rep); !slices.Equal(got, want) {
		t.Fatalf("rules = %v, want %v:\n%s", got, want, rep)
	}
	if got := linesOf(rep); !slices.Equal(got, []int{2, 4, 6}) {
		t.Errorf("lines = %v, want [2 4 6]:\n%s", got, rep)
	}
	if rep.OK() {
		t.Error("OK = true, want false")
	}
}

// Parsing keeps its old contract: the first malformed line ends the read.
// Only checking recovers.
func TestParseStillStopsAtTheFirstSyntaxError(t *testing.T) {
	t.Parallel()

	_, err := envi.ParseString("GOOD=1\nK=\"unterminated\nALSO=2\n")
	if err == nil {
		t.Fatal("Parse = nil error, want a *SyntaxError")
	}
	var se *envi.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("error %v (%T) is not a *SyntaxError", err, err)
	}
	if se.Line != 2 {
		t.Errorf("Line = %d, want 2", se.Line)
	}
}

// A line that could not be parsed stays in the document, so that checking a
// file and writing it back does not quietly delete what it did not understand.
func TestCheckKeepsUnparsableLines(t *testing.T) {
	t.Parallel()

	const src = `GOOD=1
nonsense
ALSO_GOOD=2
`

	env, _, err := envi.CheckString(src)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := env.String(); got != src {
		t.Errorf("writing back changed the file.\n--- got ---\n%s\n--- want ---\n%s", got, src)
	}
}

func TestCheckReportsDuplicateKey(t *testing.T) {
	t.Parallel()

	const src = `APP_URL=https://a.example
OTHER=1
APP_URL=https://b.example
`

	env, rep, err := envi.CheckString(src)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	want := []envi.Rule{envi.RuleDuplicateKey}
	if got := rulesOf(rep); !slices.Equal(got, want) {
		t.Fatalf("rules = %v, want %v:\n%s", got, want, rep)
	}
	var p envi.Problem
	for q := range rep.All() {
		p = q
	}
	if p.Line != 3 {
		t.Errorf("Line = %d, want 3", p.Line)
	}
	if p.Key != "APP_URL" {
		t.Errorf("Key = %q, want APP_URL", p.Key)
	}
	if p.Severity != envi.SeverityError {
		t.Errorf("Severity = %v, want error", p.Severity)
	}
	if !strings.Contains(p.Msg, "line 1") {
		t.Errorf("Msg = %q, want it to name line 1", p.Msg)
	}
	// The later definition still wins, as it always has.
	if got, _ := env.Lookup("APP_URL"); got != "https://b.example" {
		t.Errorf("APP_URL = %q, want https://b.example", got)
	}
}

// A commented-out alternative beside a live value is a shadow: an idiom the
// format is built around, not a mistake.
func TestCheckDoesNotCallAShadowADuplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{"adjacent", "# REDIS_HOST=redis\nREDIS_HOST=127.0.0.1\n"},
		{"several", "# REDIS_HOST=redis\n# REDIS_HOST=10.0.0.1\nREDIS_HOST=127.0.0.1\n"},
		{"apart", "# REDIS_HOST=redis\nOTHER=1\nREDIS_HOST=127.0.0.1\n"},
		{"after", "REDIS_HOST=127.0.0.1\nOTHER=1\n# REDIS_HOST=redis\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, rep, err := envi.CheckString(tt.src)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if rep.Len() != 0 {
				t.Errorf("Len = %d, want 0:\n%s", rep.Len(), rep)
			}
		})
	}
}

func TestCheckRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []envi.Rule
	}{
		{"canonical key", "APP_NAME=x\n", nil},
		{"lower case key", "app_name=x\n", []envi.Rule{envi.RuleKeyNotCanonical}},
		{"hyphenated key", "app-name=x\n", []envi.Rule{envi.RuleKeyNotCanonical}},
		{"leading digit", "1BAD=x\n", []envi.Rule{envi.RuleKeyInvalid}},
		{"leading dot", ".BAD=x\n", []envi.Rule{envi.RuleKeyInvalid}},
		{"empty value", "EMPTY=\n", []envi.Rule{envi.RuleEmptyValue}},
		{"empty commented value", "# EMPTY=\n", nil},
		{"bare dollar", "PASS=p$ss\n", []envi.Rule{envi.RuleUnquotedValue}},
		{"bare backtick", "CMD=`date`\n", []envi.Rule{envi.RuleUnquotedValue}},
		{"bare backslash", `P=C:\tmp` + "\n", []envi.Rule{envi.RuleUnquotedValue}},
		{"bare quote", "SAY=it's\n", []envi.Rule{envi.RuleUnquotedValue}},
		{"quoted dollar", `PASS="p$ss"` + "\n", nil},
		{"single-quoted dollar", `PASS='p$ss'` + "\n", nil},
		{"plain value", "HOST=localhost\n", nil},
		{"url with hash", "K=http://x/y#frag\n", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, rep, err := envi.CheckString(tt.src)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got := rulesOf(rep); !slices.Equal(got, tt.want) {
				t.Errorf("rules = %v, want %v:\n%s", got, tt.want, rep)
			}
		})
	}
}

func TestCheckSeverities(t *testing.T) {
	t.Parallel()

	const src = `1BAD=x
EMPTY=
app-name=y
`

	_, rep, err := envi.CheckString(src)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := rep.Count(envi.SeverityError); got != 1 {
		t.Errorf("errors = %d, want 1:\n%s", got, rep)
	}
	if got := rep.Count(envi.SeverityWarning); got != 2 {
		t.Errorf("warnings = %d, want 2:\n%s", got, rep)
	}
	if rep.OK() {
		t.Error("OK = true, want false")
	}
	if err := rep.Err(); err == nil {
		t.Error("Err = nil, want an error")
	} else if !strings.Contains(err.Error(), "key-invalid") {
		t.Errorf("Err = %q, want it to name the failing rule", err)
	}
}

// Warnings alone leave the document valid.
func TestCheckWarningsDoNotFailTheDocument(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckString("app-name=x\nEMPTY=\n")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.OK() {
		t.Errorf("OK = false, want true:\n%s", rep)
	}
	if rep.Err() != nil {
		t.Errorf("Err = %v, want nil", rep.Err())
	}
	if rep.Len() != 2 {
		t.Errorf("Len = %d, want 2:\n%s", rep.Len(), rep)
	}
}

func TestWithoutRules(t *testing.T) {
	t.Parallel()

	const src = `app-name=x
EMPTY=
1BAD=y
`

	tests := []struct {
		name string
		off  []envi.Rule
		want []envi.Rule
	}{
		{
			"none off",
			nil,
			[]envi.Rule{envi.RuleKeyNotCanonical, envi.RuleEmptyValue, envi.RuleKeyInvalid},
		},
		{
			"one off",
			[]envi.Rule{envi.RuleEmptyValue},
			[]envi.Rule{envi.RuleKeyNotCanonical, envi.RuleKeyInvalid},
		},
		{
			"several off",
			[]envi.Rule{envi.RuleEmptyValue, envi.RuleKeyNotCanonical},
			[]envi.Rule{envi.RuleKeyInvalid},
		},
		{
			"unknown rule off",
			[]envi.Rule{envi.Rule("no-such-rule")},
			[]envi.Rule{envi.RuleKeyNotCanonical, envi.RuleEmptyValue, envi.RuleKeyInvalid},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, rep, err := envi.CheckString(src, envi.WithoutRules(tt.off...))
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got := rulesOf(rep); !slices.Equal(got, tt.want) {
				t.Errorf("rules = %v, want %v:\n%s", got, tt.want, rep)
			}
		})
	}
}

func TestWithoutRulesSilencesSyntax(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckString("K=\"unterminated\n", envi.WithoutRules(envi.RuleSyntax))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.Len() != 0 {
		t.Errorf("Len = %d, want 0:\n%s", rep.Len(), rep)
	}
	if !rep.OK() {
		t.Error("OK = false, want true once the rule is off")
	}
}

// A document in memory has no source, so only the rules about content run.
func TestEnvCheck(t *testing.T) {
	t.Parallel()

	e := envi.New(
		envi.NewRow("APP_NAME", "one"),
		envi.NewRow("APP_EMPTY", ""),
		envi.NewRow("1BAD", "x"),
	)

	rep := e.Check()

	want := []envi.Rule{envi.RuleEmptyValue, envi.RuleKeyInvalid}
	if got := rulesOf(rep); !slices.Equal(got, want) {
		t.Fatalf("rules = %v, want %v:\n%s", got, want, rep)
	}
	for p := range rep.All() {
		if p.Line != 0 {
			t.Errorf("Line = %d, want 0 for a document with no source", p.Line)
		}
		if p.Key == "" {
			t.Error("Key is empty; a finding about a row must name it")
		}
	}
	if rep.OK() {
		t.Error("OK = true, want false")
	}
}

func TestEnvCheckSkipsCommentedRows(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("EMPTY", "").SetCommented(true))
	if rep := e.Check(); rep.Len() != 0 {
		t.Errorf("Len = %d, want 0:\n%s", rep.Len(), rep)
	}
}

func TestEnvCheckHonoursWithoutRules(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("EMPTY", ""))
	if rep := e.Check(envi.WithoutRules(envi.RuleEmptyValue)); rep.Len() != 0 {
		t.Errorf("Len = %d, want 0:\n%s", rep.Len(), rep)
	}
}

func TestZeroReportIsUsable(t *testing.T) {
	t.Parallel()

	var rep envi.Report
	if !rep.OK() {
		t.Error("OK = false, want true")
	}
	if rep.Len() != 0 {
		t.Errorf("Len = %d, want 0", rep.Len())
	}
	if rep.Err() != nil {
		t.Errorf("Err = %v, want nil", rep.Err())
	}
	if got := rep.String(); got != "" {
		t.Errorf("String = %q, want empty", got)
	}
	for range rep.All() {
		t.Error("All yielded a finding from an empty report")
	}
}

func TestCheckFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("app-name=x\nDUP=1\nDUP=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, rep, err := envi.CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if env == nil {
		t.Fatal("CheckFile returned no document")
	}
	want := []envi.Rule{envi.RuleKeyNotCanonical, envi.RuleDuplicateKey}
	if got := rulesOf(rep); !slices.Equal(got, want) {
		t.Errorf("rules = %v, want %v:\n%s", got, want, rep)
	}
}

func TestCheckFileMissing(t *testing.T) {
	t.Parallel()

	_, _, err := envi.CheckFile(filepath.Join(t.TempDir(), "absent"))
	if err == nil {
		t.Fatal("CheckFile on a missing path = nil error")
	}
	if !os.IsNotExist(err) {
		t.Errorf("error = %v, want a not-exist error", err)
	}
}

func TestCheckBytes(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckBytes([]byte("EMPTY=\n"))
	if err != nil {
		t.Fatalf("CheckBytes: %v", err)
	}
	if got := rulesOf(rep); !slices.Equal(got, []envi.Rule{envi.RuleEmptyValue}) {
		t.Errorf("rules = %v, want [empty-value]", got)
	}
}

func TestSeverityString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   envi.Severity
		want string
	}{
		{envi.SeverityError, "error"},
		{envi.SeverityWarning, "warning"},
		{envi.Severity(42), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// failingReader lets a fixed prefix through and then fails, so that Check's
// only error path — a broken input stream — is exercised.
type failingReader struct {
	data []byte
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestCheckReportsReaderFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("disk on fire")
	env, rep, err := envi.Check(&failingReader{data: []byte("K=v\n"), err: want})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if env != nil || rep != nil {
		t.Errorf("Check returned %v, %v alongside an error, want nil, nil", env, rep)
	}
}

func TestCheckErrorMessageCountsProblems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{"one", "1BAD=x\n", "envi: 1 problem: "},
		{"several", "1BAD=x\n2BAD=y\n", "envi: 2 problems: "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, rep, err := envi.CheckString(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			got := rep.Err()
			if got == nil {
				t.Fatal("Err = nil, want an error")
			}
			if !strings.HasPrefix(got.Error(), tt.want) {
				t.Errorf("Err = %q, want it to start with %q", got, tt.want)
			}
		})
	}
}

// A key that normalises to nothing cannot come out of the parser, which needs
// at least one key byte, but a caller can build one.
func TestEnvCheckRejectsAnEmptyKey(t *testing.T) {
	t.Parallel()

	e := envi.New(envi.NewRow("", "x"))
	rep := e.Check()

	if got := rulesOf(rep); !slices.Equal(got, []envi.Rule{envi.RuleKeyInvalid}) {
		t.Errorf("rules = %v, want [key-invalid]:\n%s", got, rep)
	}
}
