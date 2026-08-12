package envi

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// An Encoder writes documents to an output stream.
//
// Its configuration is fixed at construction, so encoders built separately
// share no state and may be used from different goroutines at once.
type Encoder struct {
	w   io.Writer
	cfg config

	// buf is scratch reused across lines so that encoding a large document
	// does not allocate per row.
	buf []byte

	// eol is the terminator to write, taken from the document being encoded so
	// that a file read as CRLF is written back as CRLF.
	eol string
}

// NewEncoder returns an encoder writing to w.
func NewEncoder(w io.Writer, opts ...Option) *Encoder {
	return &Encoder{w: w, cfg: newConfig(opts)}
}

// Encode writes e to the underlying stream.
//
// Writes go through a buffer whose error is sticky, so the individual writes
// below need no checking: the first failure is reported by the final flush.
func (enc *Encoder) Encode(e *Env) error {
	if e == nil {
		return nil
	}
	enc.eol = e.eol
	if enc.eol == "" {
		enc.eol = "\n"
	}
	bw := bufio.NewWriter(enc.w)
	enc.encode(bw, e)
	return bw.Flush()
}

// encode walks the document and writes it.
func (enc *Encoder) encode(bw *bufio.Writer, e *Env) {
	items := enc.ordered(e)

	for i, it := range items {
		switch v := it.(type) {
		case *Row:
			if !enc.writeRow(bw, v) {
				continue
			}
		case *Block:
			if !enc.writeBlock(bw, v) {
				continue
			}
		}
		if i < len(items)-1 {
			enc.writeBlanks(bw, enc.blanksAfter(it))
		}
	}

	if enc.canReproduce() {
		enc.writeRawLines(bw, e.trailer)
	}
}

// ordered returns the items to write, sorted into a copy when the configuration
// asks for it. The document itself is never rearranged by encoding.
func (enc *Encoder) ordered(e *Env) []Item {
	e.compact()
	if enc.cfg.order != OrderSorted {
		return e.items
	}
	items := slices.Clone(e.items)
	slices.SortStableFunc(items, func(x, y Item) int {
		return strings.Compare(x.Key(), y.Key())
	})
	return items
}

// blanksAfter reports how many blank lines follow it, preferring what the
// source had and falling back to the configured indent for blocks.
func (enc *Encoder) blanksAfter(it Item) int {
	n := -1
	switch v := it.(type) {
	case *Row:
		n = v.blanksAfter
	case *Block:
		n = v.blanksAfter
	}
	if n >= 0 {
		return n
	}
	if _, isBlock := it.(*Block); isBlock {
		return enc.cfg.indent
	}
	return 0
}

func (enc *Encoder) writeBlanks(bw *bufio.Writer, n int) {
	for range n {
		bw.WriteString(enc.eol)
	}
}

// writeBlock writes a block's header comment and its rows, and reports whether
// anything was written. A block holding no visible rows is skipped entirely
// rather than leaving a header introducing nothing.
func (enc *Encoder) writeBlock(bw *bufio.Writer, b *Block) bool {
	if !enc.blockHasVisibleRows(b) {
		return false
	}

	if enc.canReproduce() {
		enc.writeRawLines(bw, b.rawPrefix)
	}
	switch {
	case enc.canReproduce() && b.rawHeader != "":
		bw.WriteString(b.rawHeader)
		bw.WriteString(enc.eol)
	case b.comment != "" && enc.cfg.comments:
		bw.WriteString(enc.cfg.blockCommentBefore)
		bw.WriteString(b.comment)
		bw.WriteString(enc.cfg.blockCommentAfter)
		bw.WriteString(enc.eol)
	}

	for i, r := range b.rows {
		if !enc.writeRow(bw, r) {
			continue
		}
		if i < len(b.rows)-1 && r.blanksAfter > 0 {
			enc.writeBlanks(bw, r.blanksAfter)
		}
	}
	return true
}

// blockHasVisibleRows reports whether anything in the block will be written.
// A block whose rows are all excluded — because it is empty, or because every
// row is commented out and commented rows are switched off — must be skipped
// entirely rather than leaving a header introducing nothing.
func (enc *Encoder) blockHasVisibleRows(b *Block) bool {
	for _, r := range b.rows {
		if !r.commented || enc.cfg.commentedRows {
			return true
		}
	}
	return false
}

// canReproduce reports whether the configuration permits writing a parsed
// document back verbatim. Any option that changes how a row should look rules
// it out, because the recorded lines record the other rendering.
func (enc *Encoder) canReproduce() bool {
	return enc.cfg.quoting == QuotePreserve && enc.cfg.comments && enc.cfg.shadows
}

func (enc *Encoder) writeRawLines(bw *bufio.Writer, lines []string) {
	for _, l := range lines {
		bw.WriteString(l)
		bw.WriteString(enc.eol)
	}
}

// writeRow writes one row with its comment and shadows, and reports whether
// anything was written.
func (enc *Encoder) writeRow(bw *bufio.Writer, r *Row) bool {
	if r.commented && !enc.cfg.commentedRows {
		return false
	}

	// Faithful path: a row that came from input and has not been touched is
	// written back exactly as it was read, down to spacing and quote style.
	// A canonical assignment carries no verbatim copy, because rendering it
	// afresh produces the same bytes.
	if enc.canReproduce() && r.parsed {
		enc.writeRawLines(bw, r.rawPrefix)
		if r.rawLine != "" {
			bw.WriteString(r.rawLine)
			bw.WriteString(enc.eol)
		} else {
			enc.writeAssignmentLine(bw, r)
		}
		return true
	}

	if r.comment != "" && enc.cfg.comments {
		for line := range strings.SplitSeq(r.comment, "\n") {
			bw.WriteString("# ")
			bw.WriteString(line)
			bw.WriteString(enc.eol)
		}
	}

	// Shadows are alternatives to a live value; a row that is itself commented
	// out has nothing for them to shadow.
	if !r.commented && enc.cfg.shadows {
		for _, s := range r.shadows {
			bw.WriteString("# ")
			enc.writeAssignment(bw, r.key, s)
		}
	}

	enc.writeAssignmentLine(bw, r)
	return true
}

// writeAssignmentLine writes the assignment itself, without the comments and
// shadows that belong above it.
func (enc *Encoder) writeAssignmentLine(bw *bufio.Writer, r *Row) {
	if r.commented {
		bw.WriteString("# ")
	}
	enc.buf = appendValue(enc.buf[:0], r.value, enc.cfg.quoting)
	bw.WriteString(r.key)
	bw.WriteByte('=')
	bw.Write(enc.buf)
	if r.inline != "" && enc.cfg.comments {
		bw.WriteString(" # ")
		bw.WriteString(r.inline)
	}
	bw.WriteString(enc.eol)
}

// writeAssignment writes KEY=value for a raw string value, used for shadows,
// which carry no recorded rendering of their own.
func (enc *Encoder) writeAssignment(bw *bufio.Writer, key, value string) {
	enc.buf = enc.buf[:0]
	enc.buf = appendMinimal(enc.buf, value)
	bw.WriteString(key)
	bw.WriteByte('=')
	bw.Write(enc.buf)
	bw.WriteString(enc.eol)
}

// WriteTo writes the document to w using default formatting, satisfying
// [io.WriterTo].
//
// Use [NewEncoder] to write with options.
func (e *Env) WriteTo(w io.Writer) (int64, error) {
	cw := &countingWriter{w: w}
	if err := NewEncoder(cw).Encode(e); err != nil {
		return cw.n, err
	}
	return cw.n, nil
}

// MarshalText renders the document, satisfying [encoding.TextMarshaler].
func (e *Env) MarshalText() ([]byte, error) {
	var b strings.Builder
	if _, err := e.WriteTo(&b); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// String renders the document with default formatting.
func (e *Env) String() string {
	var b strings.Builder
	// Writing to a strings.Builder cannot fail.
	_, _ = e.WriteTo(&b)
	return b.String()
}

// Save writes the document to the file at path.
//
// The write is atomic: content goes to a temporary file in the same directory
// and is renamed over the target only once it is complete, so an interrupted
// run cannot leave a truncated .env behind.
func Save(e *Env, path string, opts ...Option) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".envi-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()

	// discard removes the half-written temporary file. Its own errors are of
	// no use to the caller, who is already being handed the failure that
	// mattered.
	discard := func(closeFile bool) {
		if closeFile {
			_ = f.Close()
		}
		_ = os.Remove(tmp)
	}

	if err := NewEncoder(f, opts...).Encode(e); err != nil {
		discard(true)
		return err
	}
	if err := f.Sync(); err != nil {
		discard(true)
		return err
	}
	if err := f.Close(); err != nil {
		discard(false)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		discard(false)
		return err
	}
	return nil
}

// countingWriter tallies bytes written so WriteTo can report them.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
