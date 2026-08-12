package envi

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

// MarshalJSON writes the severity as its name, so that a report stays readable
// once it has left the process.
func (s Severity) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON reads a severity written as its name.
func (s *Severity) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	switch name {
	case "error":
		*s = SeverityError
	case "warning":
		*s = SeverityWarning
	default:
		return errors.New("envi: unknown severity " + strconv.Quote(name))
	}
	return nil
}

// Text writes the report to w, one finding per line, in the form compilers and
// linters use — see [Problem.String]. An empty report writes nothing.
func (r *Report) Text(w io.Writer) error {
	for _, p := range r.problems {
		if _, err := io.WriteString(w, p.String()); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

// JSON writes the report to w as an array of findings, one object each, with
// the fields of [Problem]. An empty report writes an empty array, not null, so
// that the output is the same shape however the check went.
func (r *Report) JSON(w io.Writer) error {
	problems := r.problems
	if problems == nil {
		problems = []Problem{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(problems)
}

// String returns what [Report.Text] writes.
func (r *Report) String() string {
	var b strings.Builder
	// Writing to a strings.Builder cannot fail.
	_ = r.Text(&b)
	return b.String()
}
