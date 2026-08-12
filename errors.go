package envi

import (
	"errors"
	"strconv"
)

// ErrPrefixMismatch reports an attempt to put a row into a block whose prefix
// the row's key does not carry. Compare with [errors.Is].
//
// An absent key is not an error: [Env.Get] returns nil and [Env.Lookup] reports
// false, which is what callers of a lookup expect.
var ErrPrefixMismatch = errors.New("envi: row key does not match block prefix")

// A SyntaxError reports a construct that could not be parsed, along with the
// position at which the parser gave up.
//
// Parsing stops at the first SyntaxError; the returned document is nil.
type SyntaxError struct {
	// Line is the 1-based line number of the offending input line.
	Line int

	// Col is the 1-based byte offset within that line. It is 0 when the
	// problem concerns the line as a whole rather than one position in it.
	Col int

	// Msg describes the problem in lower case, without position or package
	// prefix: "unterminated quoted value".
	Msg string

	// Src is the offending line as it appeared in the input, with the trailing
	// newline removed. It may be empty if the input was not retained.
	Src string
}

// Error implements the error interface.
func (e *SyntaxError) Error() string {
	// Built by hand rather than with fmt to keep the error path allocation-light.
	var b []byte
	b = append(b, "envi: line "...)
	b = strconv.AppendInt(b, int64(e.Line), 10)
	if e.Col > 0 {
		b = append(b, ", column "...)
		b = strconv.AppendInt(b, int64(e.Col), 10)
	}
	b = append(b, ": "...)
	b = append(b, e.Msg...)
	if e.Src != "" {
		b = append(b, " in "...)
		b = strconv.AppendQuote(b, truncate(e.Src, maxSrcInError))
	}
	return string(b)
}

// maxSrcInError bounds how much of the offending line an error message quotes,
// so that a pathological input cannot produce a megabyte-long error string.
const maxSrcInError = 64

// truncate shortens s to at most n bytes, appending an ellipsis when it cuts.
// It never splits a UTF-8 rune.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Back off to a rune boundary: continuation bytes are 0b10xxxxxx.
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}
