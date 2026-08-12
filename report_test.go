package envi_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

// failingWriter reports an error on every write, so that the renderers'
// error paths are exercised rather than assumed.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestProblemString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   envi.Problem
		want string
	}{
		{
			"line and column",
			envi.Problem{Rule: envi.RuleSyntax, Severity: envi.SeverityError, Line: 4, Col: 12, Msg: "unterminated quoted value"},
			"4:12: error: syntax: unterminated quoted value",
		},
		{
			"line only",
			envi.Problem{Rule: envi.RuleEmptyValue, Severity: envi.SeverityWarning, Line: 7, Key: "APP_NAME", Msg: "value is empty"},
			"7: warning: empty-value: value is empty (APP_NAME)",
		},
		{
			"key only",
			envi.Problem{Rule: envi.RuleEmptyValue, Severity: envi.SeverityWarning, Key: "APP_NAME", Msg: "value is empty"},
			"APP_NAME: warning: empty-value: value is empty",
		},
		{
			"neither",
			envi.Problem{Rule: envi.RuleSyntax, Severity: envi.SeverityError, Msg: "something went wrong"},
			"error: syntax: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.in.String(); got != tt.want {
				t.Errorf("String = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReportText(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckString("app-name=x\nDUP=1\nDUP=2\n")
	if err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	if err := rep.Text(&b); err != nil {
		t.Fatalf("Text: %v", err)
	}

	got := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	want := []string{
		`1: warning: key-not-canonical: key is written as "app-name" (APP_NAME)`,
		"3: error: duplicate-key: key is already defined on line 2, and that value is discarded (DUP)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Text =\n%v\nwant\n%v", got, want)
	}
	if b.String() != rep.String() {
		t.Errorf("String = %q, want it to match Text", rep.String())
	}
}

func TestReportTextOnEmptyReportWritesNothing(t *testing.T) {
	t.Parallel()

	var rep envi.Report
	var b strings.Builder
	if err := rep.Text(&b); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if b.Len() != 0 {
		t.Errorf("Text wrote %q, want nothing", b.String())
	}
}

func TestReportTextReportsWriteErrors(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckString("EMPTY=\n")
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("disk on fire")
	if got := rep.Text(failingWriter{err: want}); !errors.Is(got, want) {
		t.Errorf("Text = %v, want %v", got, want)
	}
}

func TestReportJSON(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckString("app-name=x\nDUP=1\nDUP=2\n")
	if err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	if err := rep.JSON(&b); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got []envi.Problem
	if err := json.Unmarshal([]byte(b.String()), &got); err != nil {
		t.Fatalf("the output does not parse back: %v\n%s", err, b.String())
	}
	if len(got) != rep.Len() {
		t.Fatalf("len = %d, want %d", len(got), rep.Len())
	}

	want := slices.Collect(rep.All())
	if !slices.Equal(got, want) {
		t.Errorf("round trip changed the findings:\ngot  %+v\nwant %+v", got, want)
	}

	// Severity travels as its name, so a report stays readable outside Go.
	if !strings.Contains(b.String(), `"severity": "warning"`) {
		t.Errorf("severity is not written as a name:\n%s", b.String())
	}
}

func TestReportJSONOnEmptyReportWritesAnArray(t *testing.T) {
	t.Parallel()

	var rep envi.Report
	var b strings.Builder
	if err := rep.JSON(&b); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got := strings.TrimSpace(b.String()); got != "[]" {
		t.Errorf("JSON = %q, want []", got)
	}
}

func TestReportJSONReportsWriteErrors(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckString("EMPTY=\n")
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("disk on fire")
	if got := rep.JSON(failingWriter{err: want}); !errors.Is(got, want) {
		t.Errorf("JSON = %v, want %v", got, want)
	}
}

func TestSeverityJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   envi.Severity
		want string
	}{
		{"error", envi.SeverityError, `"error"`},
		{"warning", envi.SeverityWarning, `"warning"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("Marshal = %s, want %s", data, tt.want)
			}

			var back envi.Severity
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if back != tt.in {
				t.Errorf("round trip = %v, want %v", back, tt.in)
			}
		})
	}
}

func TestSeverityJSONRejectsNonsense(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{"unknown name", `"catastrophe"`},
		{"not a string", `42`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var s envi.Severity
			if err := json.Unmarshal([]byte(tt.in), &s); err == nil {
				t.Errorf("Unmarshal(%s) = nil error, want one", tt.in)
			}
		})
	}
}

// flakyWriter accepts n writes and fails afterwards, so that the renderers'
// second write — the line terminator — has its error path covered too.
type flakyWriter struct {
	ok  int
	err error
}

func (w *flakyWriter) Write(p []byte) (int, error) {
	if w.ok == 0 {
		return 0, w.err
	}
	w.ok--
	return len(p), nil
}

func TestReportTextReportsTerminatorWriteErrors(t *testing.T) {
	t.Parallel()

	_, rep, err := envi.CheckString("EMPTY=\n")
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("disk on fire")
	if got := rep.Text(&flakyWriter{ok: 1, err: want}); !errors.Is(got, want) {
		t.Errorf("Text = %v, want %v", got, want)
	}
}
