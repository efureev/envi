package envi

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
)

// lineKind classifies one physical line of input.
type lineKind int

const (
	lineBlank     lineKind = iota // nothing but whitespace
	lineComment                   // a plain comment
	lineHeader                    // a comment matching the block header template
	lineCommented                 // a commented-out assignment: "# KEY=value"
	lineAssign                    // a live assignment
)

// lineInfo is what the scanner reports for one line. Its string fields are
// materialised, so it stays valid after the scanner moves on.
type lineInfo struct {
	kind lineKind

	// raw is the line as it appeared, without its terminator.
	raw string

	// key and value are set for lineAssign and lineCommented; key is
	// normalised, value has quoting and escapes resolved.
	key, value string

	// text is the comment text: the body of a lineComment or lineHeader, or
	// the trailing comment of a lineAssign.
	text string
}

// A scanner turns a byte stream into classified lines without regular
// expressions and without a line-length limit.
//
// It reads through a bufio.Reader directly rather than through bufio.Scanner,
// which caps a token at 64 KiB — a real constraint for .env files carrying
// long secrets or certificates.
type scanner struct {
	r      *bufio.Reader
	lineNo int

	// lineBuf accumulates a line too long for the reader's buffer; valBuf is
	// scratch for unescaping. Both are reused across lines.
	lineBuf   []byte
	valBuf    []byte
	renderBuf []byte

	// eol is the terminator the first complete line used, so that a document
	// written on Windows is not silently converted to LF — which would show
	// up as a diff on every line.
	eol string

	headerBefore, headerAfter []byte
}

func newScanner(r io.Reader, cfg config) *scanner {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &scanner{
		r:            br,
		headerBefore: []byte(strings.TrimSpace(cfg.blockCommentBefore)),
		headerAfter:  []byte(strings.TrimSpace(cfg.blockCommentAfter)),
	}
}

// scan reads and classifies the next line. It reports false at end of input.
func (s *scanner) scan(out *lineInfo) (bool, error) {
	line, err := s.readLine()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	s.lineNo++
	if err := s.classify(line, out); err != nil {
		return false, err
	}
	return true, nil
}

// readLine returns the next line without its terminator. The result is only
// valid until the next call.
func (s *scanner) readLine() ([]byte, error) {
	s.lineBuf = s.lineBuf[:0]
	for {
		chunk, err := s.r.ReadSlice('\n')
		switch {
		case err == nil:
			if len(s.lineBuf) == 0 {
				// Whole line already contiguous: hand it over without copying.
				return s.trimEOL(chunk), nil
			}
			s.lineBuf = append(s.lineBuf, chunk...)
			return s.trimEOL(s.lineBuf), nil

		case errors.Is(err, bufio.ErrBufferFull):
			s.lineBuf = append(s.lineBuf, chunk...)

		case errors.Is(err, io.EOF):
			if len(chunk) == 0 && len(s.lineBuf) == 0 {
				return nil, io.EOF
			}
			s.lineBuf = append(s.lineBuf, chunk...)
			return s.trimEOL(s.lineBuf), nil

		default:
			return nil, err
		}
	}
}

// classify decides what the line is and fills out.
func (s *scanner) classify(line []byte, out *lineInfo) error {
	*out = lineInfo{}

	trimmed := trimSpace(line)
	if len(trimmed) == 0 {
		out.kind = lineBlank
		out.raw = string(line)
		return nil
	}

	if trimmed[0] == '#' {
		if body, ok := s.headerBody(trimmed); ok {
			out.kind = lineHeader
			out.raw = string(line)
			out.text = string(trimSpace(body))
			return nil
		}
		// A commented-out assignment is a shadow or an inert row; anything
		// else is prose. Prose keeps only its raw form — the text is a
		// substring of it, taken without allocating when a row asks for it.
		body := stripCommentMarker(trimmed)
		if key, value, _, perr := s.parseAssign(body); perr == nil {
			out.kind = lineCommented
			out.raw = string(line)
			out.key = NormalizeKey(string(key))
			out.value = string(value)
			return nil
		}
		out.kind = lineComment
		out.raw = string(line)
		return nil
	}

	key, value, comment, perr := s.parseAssign(trimmed)
	if perr != nil {
		return &SyntaxError{Line: s.lineNo, Col: perr.col, Msg: perr.msg, Src: string(line)}
	}
	out.kind = lineAssign
	out.key = NormalizeKey(string(key))
	out.value = string(value)
	out.text = string(comment)

	// A line already in the form the encoder produces needs no verbatim copy:
	// re-rendering reproduces it byte for byte. Most lines of a real .env are
	// in that form, so this removes an allocation per row.
	if !s.assignIsCanonical(line, out) {
		out.raw = string(line)
	}
	return nil
}

// assignIsCanonical reports whether re-rendering the parsed assignment would
// give back exactly the line that was read.
func (s *scanner) assignIsCanonical(line []byte, out *lineInfo) bool {
	s.renderBuf = append(s.renderBuf[:0], out.key...)
	s.renderBuf = append(s.renderBuf, '=')
	s.renderBuf = appendMinimal(s.renderBuf, out.value)
	if out.text != "" {
		s.renderBuf = append(s.renderBuf, " # "...)
		s.renderBuf = append(s.renderBuf, out.text...)
	}
	return bytes.Equal(s.renderBuf, line)
}

// headerBody reports whether the line is a block header and returns its inner
// text.
func (s *scanner) headerBody(trimmed []byte) ([]byte, bool) {
	if len(s.headerBefore) == 0 || len(s.headerAfter) == 0 {
		return nil, false
	}
	if len(trimmed) < len(s.headerBefore)+len(s.headerAfter) {
		return nil, false
	}
	if !bytes.HasPrefix(trimmed, s.headerBefore) || !bytes.HasSuffix(trimmed, s.headerAfter) {
		return nil, false
	}
	return trimmed[len(s.headerBefore) : len(trimmed)-len(s.headerAfter)], true
}

// parseErr describes why a line is not a well-formed assignment. A soft fault
// means the line simply is not one, which is unremarkable for a comment.
type parseErr struct {
	col  int
	msg  string
	soft bool
}

// parseAssign parses "export KEY = value # comment" in a single pass.
//
// The returned value may point into the line or into the scanner's scratch
// buffer, so the caller must materialise it before scanning on.
func (s *scanner) parseAssign(line []byte) (key, value, comment []byte, perr *parseErr) {
	n := len(line)
	i := skipSpace(line, 0)

	// An optional "export " prefix, as shell-sourced files carry.
	const export = "export"
	if i+len(export) < n && string(line[i:i+len(export)]) == export && isSpace(line[i+len(export)]) {
		i = skipSpace(line, i+len(export))
	}

	ks := i
	for i < n && isKeyByte(line[i]) {
		i++
	}
	if i == ks {
		return nil, nil, nil, &parseErr{col: i + 1, msg: "expected a key", soft: true}
	}
	key = line[ks:i]

	i = skipSpace(line, i)
	if i >= n || (line[i] != '=' && line[i] != ':') {
		return nil, nil, nil, &parseErr{col: i + 1, msg: "expected '=' after key", soft: true}
	}
	i = skipSpace(line, i+1)

	if i < n && (line[i] == '"' || line[i] == '\'') {
		value, i, perr = s.quotedValue(line, i)
		if perr != nil {
			return nil, nil, nil, perr
		}
	} else {
		vs := i
		for i < n && line[i] != '#' {
			i++
		}
		value = trimRightSpace(line[vs:i])
	}

	i = skipSpace(line, i)
	if i < n {
		if line[i] != '#' {
			return nil, nil, nil, &parseErr{col: i + 1, msg: "unexpected text after value"}
		}
		comment = trimSpace(line[i+1:])
	}
	return key, value, comment, nil
}

// quotedValue reads a quoted value starting at the opening quote and returns it
// along with the index just past the closing quote.
//
// Single quotes are literal; double quotes honour escapes.
func (s *scanner) quotedValue(line []byte, i int) (value []byte, next int, perr *parseErr) {
	n := len(line)
	quote := line[i]
	open := i
	i++

	start := i
	escaped := false
	for i < n {
		c := line[i]
		if c == '\\' && quote == '"' {
			if i+1 >= n {
				return nil, i, &parseErr{col: i + 1, msg: "dangling escape in quoted value"}
			}
			escaped = true
			i += 2
			continue
		}
		if c == quote {
			break
		}
		i++
	}
	if i >= n {
		return nil, i, &parseErr{col: open + 1, msg: "unterminated quoted value"}
	}

	body := line[start:i]
	i++ // past the closing quote

	if escaped {
		s.valBuf = unescape(s.valBuf[:0], body)
		return s.valBuf, i, nil
	}
	return body, i, nil
}

// unescape resolves the escape sequences the encoder produces. An unknown
// escape yields the escaped byte itself, which is what a shell would do.
func unescape(dst, src []byte) []byte {
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c != '\\' || i+1 >= len(src) {
			dst = append(dst, c)
			continue
		}
		i++
		switch src[i] {
		case 'n':
			dst = append(dst, '\n')
		case 'r':
			dst = append(dst, '\r')
		case 't':
			dst = append(dst, '\t')
		default:
			dst = append(dst, src[i])
		}
	}
	return dst
}

// stripCommentMarker removes the leading hashes of a comment and one space
// after them, leaving the text.
func stripCommentMarker(b []byte) []byte {
	i := 0
	for i < len(b) && b[i] == '#' {
		i++
	}
	if i < len(b) && b[i] == ' ' {
		i++
	}
	return b[i:]
}

// isSpace reports whether c is whitespace for trimming purposes.
//
// A carriage return counts. One can only reach this point stranded inside a
// line — a well-formed terminator was already removed — and leaving it in a
// value or a comment would produce output whose trailing CR is read back as
// part of the next line's terminator, quietly losing it. A carriage return
// that genuinely belongs to a value survives, because such a value is quoted
// and the CR written as an escape.
func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' }

// isKeyByte reports whether c may appear in a key as written. Keys are
// normalised afterwards, so a hyphen is accepted here and folded later.
func isKeyByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '_' || c == '.' || c == '-'
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && isSpace(b[i]) {
		i++
	}
	return i
}

func trimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && isSpace(b[i]) {
		i++
	}
	for j > i && isSpace(b[j-1]) {
		j--
	}
	return b[i:j]
}

func trimRightSpace(b []byte) []byte {
	j := len(b)
	for j > 0 && isSpace(b[j-1]) {
		j--
	}
	return b[:j]
}

// trimEOL removes the line terminator and records which one the document uses.
//
// A carriage return is dropped even without the newline that should follow it,
// which happens at the end of a file that was cut short. Leaving it in would
// make the line encode to something that reads back differently.
func (s *scanner) trimEOL(b []byte) []byte {
	var hadLF, hadCR bool
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
		hadLF = true
	}
	// Every trailing carriage return goes, not just the one belonging to a
	// CRLF pair: a value cannot legitimately end in a bare CR, since that form
	// is written escaped, and leaving one in makes the line encode to
	// something that reads back differently.
	for n := len(b); n > 0 && b[n-1] == '\r'; n = len(b) {
		b = b[:n-1]
		hadCR = true
	}

	if s.eol == "" && hadLF {
		if hadCR {
			s.eol = "\r\n"
		} else {
			s.eol = "\n"
		}
	}
	return b
}
