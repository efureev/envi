package envi

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// A Decoder reads documents from an input stream.
//
// Its configuration is fixed at construction, so decoders built separately
// share no state and may be used from different goroutines at once.
type Decoder struct {
	s   *scanner
	cfg config
}

// NewDecoder returns a decoder reading from r.
func NewDecoder(r io.Reader, opts ...Option) *Decoder {
	cfg := newConfig(opts)
	return &Decoder{s: newScanner(r, cfg), cfg: cfg}
}

// Decode reads one document from the stream.
//
// A malformed line stops the read and is reported as a [*SyntaxError] carrying
// its position.
func (d *Decoder) Decode() (*Env, error) {
	b := newBuilder(d.cfg)
	var info lineInfo
	for {
		ok, err := d.s.scan(&info)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		b.feed(&info)
	}
	env, err := b.finish()
	if err != nil {
		return nil, err
	}
	env.eol = d.s.eol
	return env, nil
}

// Parse reads a document from r.
func Parse(r io.Reader, opts ...Option) (*Env, error) {
	return NewDecoder(r, opts...).Decode()
}

// ParseBytes reads a document from data.
func ParseBytes(data []byte, opts ...Option) (*Env, error) {
	return Parse(bytes.NewReader(data), opts...)
}

// ParseString reads a document from s.
func ParseString(s string, opts ...Option) (*Env, error) {
	return Parse(strings.NewReader(s), opts...)
}

// Load reads the named files in order and merges them, so that a later file
// overrides an earlier one while keeping comments the earlier one carried.
//
// With no arguments it reads ".env".
func Load(paths ...string) (*Env, error) {
	return LoadWith(nil, paths...)
}

// LoadWith is [Load] with parsing options.
func LoadWith(opts []Option, paths ...string) (*Env, error) {
	if len(paths) == 0 {
		paths = []string{".env"}
	}
	var env *Env
	for _, path := range paths {
		e, err := loadFile(path, opts)
		if err != nil {
			return nil, err
		}
		if env == nil {
			env = e
			continue
		}
		if err := env.Merge(e); err != nil {
			return nil, err
		}
	}
	return env, nil
}

func loadFile(path string, opts []Option) (*Env, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	e, err := Parse(bufio.NewReader(f), opts...)
	if err != nil {
		return nil, fmt.Errorf("envi: %s: %w", path, err)
	}
	return e, nil
}

// pendingLine is a line held until the row it belongs to appears.
type pendingLine struct {
	raw  string
	kind lineKind
}

// headerInfo is a block header comment awaiting the block it introduces.
type headerInfo struct {
	text   string
	raw    string
	before []string
}

// pendingRow is a parsed row and the header, if any, that preceded it.
type pendingRow struct {
	row    *Row
	header *headerInfo
}

// A builder assembles a document from classified lines.
//
// It buffers only the lines between one assignment and the next, so memory
// stays proportional to the longest comment block rather than to the file.
type builder struct {
	cfg config

	pending    []pendingLine
	headerIdx  int
	headerText string

	rows []pendingRow

	// seen maps a key to the row already emitted for it, so that a document
	// defining the same key twice folds instead of producing two rows.
	seen map[string]*Row

	// arena hands out rows from a shared backing array. Every row a document
	// produces outlives the parse, so carving them from chunks costs one
	// allocation per chunk instead of one per row.
	arena []Row
}

// arenaChunk is how many rows are allocated at a time.
const arenaChunk = 64

func newBuilder(cfg config) *builder {
	return &builder{cfg: cfg, headerIdx: -1, seen: make(map[string]*Row)}
}

// newRow returns storage for one row.
func (b *builder) newRow() *Row {
	if len(b.arena) == 0 {
		b.arena = make([]Row, arenaChunk)
	}
	r := &b.arena[0]
	b.arena = b.arena[1:]
	return r
}

func (b *builder) feed(info *lineInfo) {
	switch info.kind {
	case lineBlank, lineComment:
		b.pending = append(b.pending, pendingLine{raw: info.raw, kind: info.kind})
	case lineHeader:
		b.headerIdx = len(b.pending)
		b.headerText = info.text
		b.pending = append(b.pending, pendingLine{raw: info.raw, kind: info.kind})
	case lineCommented:
		b.emit(info, true)
	case lineAssign:
		b.emit(info, false)
	}
}

// emit turns an assignment into a row, attaching everything that was waiting
// above it.
func (b *builder) emit(info *lineInfo, commented bool) {
	r := b.newRow()
	*r = Row{
		key:       info.key,
		value:     info.value,
		rawLine:   info.raw,
		parsed:    true,
		commented: commented,
	}

	header, prefix, comment := b.takePending()
	r.rawPrefix = prefix
	r.comment = comment
	r.inline = info.text

	if !commented {
		header = b.absorbShadows(r, header)
	}

	// The same key stated twice in one document folds into the row already
	// emitted for it. Such a document cannot be reproduced line for line, so
	// the verbatim rendering is dropped and the row is written from the model.
	if prev, ok := b.seen[r.key]; ok {
		foldDuplicate(prev, r, commented)
		return
	}

	b.seen[r.key] = r
	b.rows = append(b.rows, pendingRow{row: r, header: header})
}

// foldDuplicate merges a repeated definition of a key into the row already
// holding it. A live definition wins the value and demotes what was there to a
// shadow; a commented one only adds a shadow.
func foldDuplicate(prev, next *Row, nextCommented bool) {
	if nextCommented {
		prev.addParsedShadow(next.value)
	} else {
		prev.addParsedShadow(prev.value)
		prev.value = next.value
		prev.commented = false
	}
	if prev.comment == "" {
		prev.comment = next.comment
	}
	if prev.inline == "" {
		prev.inline = next.inline
	}
	for _, s := range next.shadows {
		prev.addParsedShadow(s)
	}
	prev.dropRaw()
}

// absorbShadows folds the commented rows directly above r that name the same
// key into r as shadows: they are alternatives of this value, not rows of their
// own. Their verbatim lines move to r, so the document still reproduces.
func (b *builder) absorbShadows(r *Row, header *headerInfo) *headerInfo {
	var absorbed []pendingRow
	for len(b.rows) > 0 {
		last := b.rows[len(b.rows)-1]
		if !last.row.commented || last.row.key != r.key {
			break
		}
		absorbed = append(absorbed, last)
		b.rows = b.rows[:len(b.rows)-1]
		delete(b.seen, last.row.key)
	}
	if len(absorbed) == 0 {
		return header
	}
	slices.Reverse(absorbed) // back into source order

	// The merged row reproduces the input only if every line going into it
	// still has its verbatim form. A row that already folded a duplicate has
	// given that up, and then the whole group must be written from the model.
	faithful := r.parsed
	prefix := make([]string, 0, len(absorbed)*2+len(r.rawPrefix))
	var comments []string

	for _, a := range absorbed {
		if !a.row.parsed || a.row.rawLine == "" {
			faithful = false
		} else {
			prefix = append(prefix, a.row.rawPrefix...)
			prefix = append(prefix, a.row.rawLine)
		}

		r.addParsedShadow(a.row.value)
		for _, s := range a.row.shadows {
			r.addParsedShadow(s)
		}
		if a.row.comment != "" {
			comments = append(comments, a.row.comment)
		}
		if r.inline == "" {
			r.inline = a.row.inline
		}
		if header == nil && a.header != nil {
			header = a.header
		}
	}

	if r.comment != "" {
		comments = append(comments, r.comment)
	}
	r.comment = strings.Join(comments, "\n")

	if faithful {
		r.rawPrefix = append(prefix, r.rawPrefix...)
	} else {
		r.dropRaw()
	}
	return header
}

// takePending hands over the buffered lines and resets the buffer, splitting
// off a block header if one was seen.
func (b *builder) takePending() (header *headerInfo, prefix []string, comment string) {
	lines := b.pending
	if b.headerIdx >= 0 && b.headerIdx < len(lines) {
		header = &headerInfo{
			text:   b.headerText,
			raw:    lines[b.headerIdx].raw,
			before: rawsOf(lines[:b.headerIdx]),
		}
		lines = lines[b.headerIdx+1:]
	}
	prefix = rawsOf(lines)
	comment = commentTextFrom(lines)

	b.pending = b.pending[:0]
	b.headerIdx = -1
	b.headerText = ""
	return header, prefix, comment
}

// commentTextFrom joins the prose comments among lines. The single-comment
// case, which is the common one, hands back a substring and allocates nothing.
func commentTextFrom(lines []pendingLine) string {
	count, last := 0, -1
	for i, l := range lines {
		if l.kind == lineComment {
			count++
			last = i
		}
	}
	switch count {
	case 0:
		return ""
	case 1:
		return commentTextOf(lines[last].raw)
	}

	var sb strings.Builder
	first := true
	for _, l := range lines {
		if l.kind != lineComment {
			continue
		}
		if !first {
			sb.WriteByte('\n')
		}
		first = false
		sb.WriteString(commentTextOf(l.raw))
	}
	return sb.String()
}

// commentTextOf strips the indentation, hashes and one following space from a
// comment line. The result is a substring, so it costs nothing.
func commentTextOf(raw string) string {
	s := strings.TrimLeft(raw, " \t")
	i := 0
	for i < len(s) && s[i] == '#' {
		i++
	}
	if i < len(s) && s[i] == ' ' {
		i++
	}
	return s[i:]
}

// finish groups contiguous runs of rows sharing a prefix into blocks and
// returns the assembled document.
func (b *builder) finish() (*Env, error) {
	env := &Env{}

	for i := 0; i < len(b.rows); {
		prefix, _ := splitKey(b.rows[i].row.key)
		j := i
		if prefix != "" {
			for j < len(b.rows) {
				p, _ := splitKey(b.rows[j].row.key)
				if p != prefix {
					break
				}
				j++
			}
		} else {
			j = i + 1
		}

		if prefix != "" && j-i >= b.cfg.groupThreshold {
			if err := b.addBlock(env, prefix, b.rows[i:j]); err != nil {
				return nil, err
			}
		} else {
			for _, pr := range b.rows[i:j] {
				foldHeader(pr)
				if err := env.Add(pr.row); err != nil {
					return nil, err
				}
			}
		}
		i = j
	}

	env.trailer = rawsOf(b.pending)
	return env, nil
}

// addBlock builds one block from a run of rows.
func (b *builder) addBlock(env *Env, prefix string, run []pendingRow) error {
	blk := NewBlock(prefix)
	blk.blanksAfter = 0

	if h := run[0].header; h != nil {
		blk.comment = h.text
		blk.rawHeader = h.raw
		blk.rawPrefix = h.before
	}
	for k, pr := range run {
		if k > 0 {
			// A header appearing partway through a run introduces nothing the
			// block can own, so it stays verbatim above its own row.
			foldHeader(pr)
		}
		if err := blk.Add(pr.row); err != nil {
			return err
		}
	}
	env.appendBlock(blk)
	return nil
}

// foldHeader moves an unconsumed header into its row's verbatim prefix, so no
// input line is dropped.
func foldHeader(pr pendingRow) {
	if pr.header == nil {
		return
	}
	pre := make([]string, 0, len(pr.header.before)+1+len(pr.row.rawPrefix))
	pre = append(pre, pr.header.before...)
	pre = append(pre, pr.header.raw)
	pr.row.rawPrefix = append(pre, pr.row.rawPrefix...)
}

func rawsOf(lines []pendingLine) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.raw
	}
	return out
}
