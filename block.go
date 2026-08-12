package envi

import (
	"fmt"
	"iter"
	"slices"
)

// A Block groups rows that share a prefix up to the first underscore, and may
// carry a header comment introducing them.
//
// Membership never rewrites a row's key: a block holding APP_NAME stores a row
// whose key is APP_NAME, not NAME. Adding a row whose key does not carry the
// block's prefix is an error rather than a silent drop.
//
// The zero Block is not usable; construct one with [NewBlock].
type Block struct {
	prefix  string
	comment string
	rows    []*Row

	// index maps a full row key to its position in rows. It stays nil while
	// the block is small, where scanning a handful of rows beats hashing and
	// costs no allocation — and most blocks in a real .env are small.
	index map[string]int

	// blanksAfter is how many blank lines followed the block in the source,
	// or -1 for a block built in memory, which takes the configured indent
	// instead. Recording it is what lets a parsed document round-trip byte
	// for byte.
	blanksAfter int

	// rawHeader is the header comment line exactly as it appeared in the
	// input, and rawPrefix the verbatim lines above it. Both are empty for a
	// block built in memory, and rawHeader is dropped when the comment
	// changes.
	rawHeader string
	rawPrefix []string
}

// blockIndexThreshold is the row count above which a block starts hashing its
// keys instead of scanning them.
const blockIndexThreshold = 8

// NewBlock returns an empty block with the given prefix, normalised (see
// [NormalizeKey]).
func NewBlock(prefix string) *Block {
	return &Block{
		prefix:      NormalizeKey(prefix),
		blanksAfter: -1,
	}
}

// find returns the position of key in rows, or -1.
func (b *Block) find(key string) int {
	if b.index != nil {
		if i, ok := b.index[key]; ok {
			return i
		}
		return -1
	}
	for i, r := range b.rows {
		if r.key == key {
			return i
		}
	}
	return -1
}

// buildIndex switches the block over to hashed lookup.
func (b *Block) buildIndex() {
	b.index = make(map[string]int, len(b.rows)*2)
	for i, r := range b.rows {
		b.index[r.key] = i
	}
}

// clone returns an independent copy of the block and of every row in it.
func (b *Block) clone() *Block {
	c := &Block{
		prefix:      b.prefix,
		comment:     b.comment,
		rows:        make([]*Row, len(b.rows)),
		blanksAfter: b.blanksAfter,
		rawHeader:   b.rawHeader,
		rawPrefix:   slices.Clone(b.rawPrefix),
	}
	for i, r := range b.rows {
		c.rows[i] = r.clone()
	}
	if len(c.rows) > blockIndexThreshold {
		c.buildIndex()
	}
	return c
}

// Key returns the block's prefix, satisfying [Item].
func (b *Block) Key() string { return b.prefix }

// Prefix returns the prefix shared by every row in the block.
func (b *Block) Prefix() string { return b.prefix }

// Comment returns the block's header comment, without its decoration.
func (b *Block) Comment() string { return b.comment }

// SetComment replaces the block's header comment and returns b for chaining.
func (b *Block) SetComment(s string) *Block {
	b.comment = s
	b.rawHeader = ""
	return b
}

// Len returns the number of rows in the block.
func (b *Block) Len() int { return len(b.rows) }

// Rows iterates the block's rows in order.
func (b *Block) Rows() iter.Seq[*Row] {
	return slices.Values(b.rows)
}

// qualify accepts either a full key or one relative to the block's prefix, and
// returns the full normalised form.
//
//	block "APP": qualify("NAME") == qualify("APP_NAME") == "APP_NAME"
func (b *Block) qualify(key string) string {
	k := NormalizeKey(key)
	if prefix, _ := splitKey(k); prefix == b.prefix {
		return k
	}
	return b.prefix + string(blockSeparator) + k
}

// Get returns the row stored under key, or nil if there is none. The key may be
// given in full or relative to the block's prefix.
func (b *Block) Get(key string) *Row {
	if i := b.find(b.qualify(key)); i >= 0 {
		return b.rows[i]
	}
	return nil
}

// Has reports whether the block holds a row under key.
func (b *Block) Has(key string) bool {
	return b.find(b.qualify(key)) >= 0
}

// Add appends rows to the block.
//
// Every row must carry the block's prefix; the first that does not stops the
// call and is reported as an error wrapping [ErrPrefixMismatch], leaving
// earlier rows added. A row whose key is already present is merged into the
// existing one rather than duplicated.
func (b *Block) Add(rows ...*Row) error {
	for _, r := range rows {
		if r == nil {
			continue
		}
		prefix, _ := splitKey(r.key)
		if prefix != b.prefix {
			return fmt.Errorf("%w: key %q is not in block %q", ErrPrefixMismatch, r.key, b.prefix)
		}
		if i := b.find(r.key); i >= 0 {
			b.rows[i].merge(r)
			continue
		}
		b.rows = append(b.rows, r)
		b.noteAppended()
	}
	return nil
}

// noteAppended keeps the index in step after a row lands at the end, building
// it the moment the block outgrows a linear scan.
func (b *Block) noteAppended() {
	switch {
	case b.index != nil:
		b.index[b.rows[len(b.rows)-1].key] = len(b.rows) - 1
	case len(b.rows) > blockIndexThreshold:
		b.buildIndex()
	}
}

// Delete removes the row stored under key and reports whether one was present.
// The key may be given in full or relative to the block's prefix.
func (b *Block) Delete(key string) bool {
	i := b.find(b.qualify(key))
	if i < 0 {
		return false
	}
	b.rows = slices.Delete(b.rows, i, i+1)
	b.reindex()
	return true
}

// reindex rebuilds the key index after positions have shifted, dropping it
// entirely once the block is small enough to scan.
func (b *Block) reindex() {
	if len(b.rows) <= blockIndexThreshold {
		b.index = nil
		return
	}
	b.buildIndex()
}

// merge folds other into b, adding rows absent here and merging those present.
func (b *Block) merge(other *Block) {
	if b.comment == "" {
		b.comment = other.comment
	}
	for _, r := range other.rows {
		if i := b.find(r.key); i >= 0 {
			b.rows[i].merge(r)
			continue
		}
		b.rows = append(b.rows, r.clone())
		b.noteAppended()
	}
}

// sortRows orders the block's rows by key.
func (b *Block) sortRows() {
	slices.SortStableFunc(b.rows, func(x, y *Row) int {
		switch {
		case x.key < y.key:
			return -1
		case x.key > y.key:
			return 1
		default:
			return 0
		}
	})
	b.reindex()
}

func (b *Block) sealed() {}
