package bind

import (
	"errors"
	"strings"
)

// Sentinel errors reported by this package. Compare with [errors.Is].
var (
	// ErrRequired reports a field marked required whose key was not set.
	ErrRequired = errors.New("bind: required value is missing")

	// ErrNotPointer reports a destination that is not a non-nil pointer.
	ErrNotPointer = errors.New("bind: destination must be a non-nil pointer to a struct")

	// ErrNotStruct reports a destination that does not point at a struct.
	ErrNotStruct = errors.New("bind: destination must point at a struct")

	// ErrUnsupportedType reports a field whose type this package cannot fill.
	ErrUnsupportedType = errors.New("bind: unsupported field type")
)

// A FieldError reports what went wrong with one field.
type FieldError struct {
	// Field is the path to the field in Go terms, such as "DB.Port".
	Field string

	// Key is the environment key that was consulted.
	Key string

	// Err is the underlying cause.
	Err error
}

// Error implements the error interface.
func (e *FieldError) Error() string {
	return e.Field + " (" + e.Key + "): " + e.Err.Error()
}

// Unwrap returns the underlying cause.
func (e *FieldError) Unwrap() error { return e.Err }

// An Error collects every field that failed, so that one run tells the user
// about all the problems in their configuration rather than the first.
type Error struct {
	Fields []FieldError
}

// Error implements the error interface.
func (e *Error) Error() string {
	if len(e.Fields) == 1 {
		return "bind: " + e.Fields[0].Error()
	}
	var b strings.Builder
	b.WriteString("bind: ")
	b.WriteString(itoa(len(e.Fields)))
	b.WriteString(" fields failed:")
	for i := range e.Fields {
		b.WriteString("\n  - ")
		b.WriteString(e.Fields[i].Error())
	}
	return b.String()
}

// Unwrap exposes the individual failures to [errors.Is] and [errors.As].
func (e *Error) Unwrap() []error {
	out := make([]error, len(e.Fields))
	for i := range e.Fields {
		out[i] = &e.Fields[i]
	}
	return out
}

// itoa formats a small non-negative count without pulling in strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
