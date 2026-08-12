package envi

import (
	"fmt"
	"iter"
	"os"
	"slices"
	"strings"
)

// An Env is a parsed .env document: an ordered sequence of [Item] values with
// constant-time lookup by key.
//
// Order is the order of the source, or of insertion for documents built in
// memory. It is never rearranged implicitly; call [Env.SortByKey] to sort.
//
// An Env must not be mutated concurrently.
//
// The zero Env is ready to use.
type Env struct {
	// items holds document order. Removed entries are left as nil and swept
	// by compact, so deletion does not shift every following index.
	items []Item

	// rowIndex maps every row key — top level or inside a block — to the
	// position of the item holding it. blockIndex maps a prefix to the first
	// block carrying it, which is both how [Env.Block] answers and the
	// fallback for a row added straight to a block after it joined the
	// document. Every lookup is O(1) either way.
	rowIndex   map[string]int
	blockIndex map[string]int

	// dirty records that items contains holes.
	dirty bool

	// eol is the line terminator the document was read with, empty for one
	// built in memory, which is written with "\n".
	eol string

	// trailer holds the verbatim lines that followed the last assignment in
	// the input — trailing comments and blank lines — so that reproducing the
	// document does not truncate its tail.
	trailer []string
}

// New returns a document containing the given items.
//
// It panics if an item cannot be placed — a block whose prefix repeats, or a
// row rejected by its block. Use [Env.Add] on an empty Env to handle that as an
// error instead.
func New(items ...Item) *Env {
	e := &Env{}
	if err := e.Add(items...); err != nil {
		panic("envi: New: " + err.Error())
	}
	return e
}

// init makes the indexes usable on a zero Env.
func (e *Env) init() {
	if e.rowIndex == nil {
		e.rowIndex = make(map[string]int)
		e.blockIndex = make(map[string]int)
	}
}

// compact removes holes left by deletion and rebuilds the indexes. It is called
// before anything that walks items in order, so the cost is paid once per batch
// of removals rather than once per removal.
func (e *Env) compact() {
	if !e.dirty {
		return
	}
	kept := e.items[:0]
	for _, it := range e.items {
		if it != nil {
			kept = append(kept, it)
		}
	}
	clear(e.items[len(kept):])
	e.items = kept
	e.reindex()
	e.dirty = false
}

// reindex rebuilds both indexes from items.
func (e *Env) reindex() {
	e.init()
	clear(e.rowIndex)
	clear(e.blockIndex)
	for i, it := range e.items {
		switch v := it.(type) {
		case *Row:
			e.rowIndex[v.key] = i
		case *Block:
			if _, seen := e.blockIndex[v.prefix]; !seen {
				e.blockIndex[v.prefix] = i
			}
			for _, r := range v.rows {
				e.rowIndex[r.key] = i
			}
		}
	}
}

// NumItems returns the number of top-level items: rows plus blocks.
func (e *Env) NumItems() int {
	e.compact()
	return len(e.items)
}

// NumBlocks returns the number of blocks in the document.
func (e *Env) NumBlocks() int {
	e.compact()
	return len(e.blockIndex)
}

// Len returns the total number of rows, counting those inside blocks.
func (e *Env) Len() int {
	e.compact()
	n := 0
	for _, it := range e.items {
		switch v := it.(type) {
		case *Row:
			n++
		case *Block:
			n += len(v.rows)
		}
	}
	return n
}

// Get returns the row stored under key, or nil if the document has none.
//
// A top-level row takes precedence over a block member of the same name, though
// the two cannot both arise from documents built through this package's API:
// adding a block adopts any top-level rows carrying its prefix.
func (e *Env) Get(key string) *Row {
	k := NormalizeKey(key)
	if i, ok := e.rowIndex[k]; ok {
		switch v := e.items[i].(type) {
		case *Row:
			return v
		case *Block:
			return v.Get(k)
		}
	}
	// Fallback for a row put straight into a block after that block joined the
	// document, which the index above has not been told about.
	if prefix, _ := splitKey(k); prefix != "" {
		if i, ok := e.blockIndex[prefix]; ok {
			return e.items[i].(*Block).Get(k)
		}
	}
	return nil
}

// Lookup returns the value stored under key and whether it was present. It is
// the accessor other packages should build on.
func (e *Env) Lookup(key string) (string, bool) {
	if r := e.Get(key); r != nil {
		return r.value, true
	}
	return "", false
}

// Has reports whether the document holds a row under key.
func (e *Env) Has(key string) bool { return e.Get(key) != nil }

// Block returns the block with the given prefix, or nil if there is none.
func (e *Env) Block(prefix string) *Block {
	if i, ok := e.blockIndex[NormalizeKey(prefix)]; ok {
		return e.items[i].(*Block)
	}
	return nil
}

// Set stores value under key and returns the affected row, creating it if the
// document had none. A new row joins the block matching its prefix when one
// exists, and sits at top level otherwise.
func (e *Env) Set(key, value string) *Row {
	e.init()
	k := NormalizeKey(key)
	if r := e.Get(k); r != nil {
		return r.SetValue(value)
	}
	r := &Row{key: k, value: value}
	e.place(r)
	return r
}

// place files an unseen row into its block, or at top level.
func (e *Env) place(r *Row) {
	if prefix, _ := splitKey(r.key); prefix != "" {
		if i, ok := e.blockIndex[prefix]; ok {
			// The prefix matches by construction, so Add cannot fail.
			_ = e.items[i].(*Block).Add(r)
			e.rowIndex[r.key] = i
			return
		}
	}
	e.rowIndex[r.key] = len(e.items)
	e.items = append(e.items, r)
}

// appendBlock adds nb as a new item even when a block of the same prefix is
// already present.
//
// The parser needs this: a prefix may reappear further down a document, and
// folding the two together would move rows across the file. [Env.Add] keeps the
// merging behaviour, which is what a caller building a document by hand wants.
func (e *Env) appendBlock(nb *Block) {
	e.init()
	pos := len(e.items)
	if _, seen := e.blockIndex[nb.prefix]; !seen {
		e.blockIndex[nb.prefix] = pos
	}
	e.items = append(e.items, nb)
	for _, r := range nb.rows {
		e.rowIndex[r.key] = pos
	}
}

// Add inserts items into the document.
//
// A row whose key is already present is merged into the existing one. A block
// whose prefix is already present is merged into the existing block; otherwise
// it adopts any top-level rows that carry its prefix, so a key is never
// reachable by two different paths.
func (e *Env) Add(items ...Item) error {
	e.init()
	for _, it := range items {
		switch v := it.(type) {
		case nil:
			continue
		case *Row:
			if v == nil {
				continue
			}
			if r := e.Get(v.key); r != nil {
				r.merge(v)
				continue
			}
			e.place(v)
		case *Block:
			if v == nil {
				continue
			}
			if err := e.addBlock(v); err != nil {
				return err
			}
		default:
			return fmt.Errorf("envi: unsupported item type %T", it)
		}
	}
	return nil
}

// addBlock inserts nb, merging into an existing block of the same prefix and
// otherwise adopting matching top-level rows.
func (e *Env) addBlock(nb *Block) error {
	if i, ok := e.blockIndex[nb.prefix]; ok {
		blk := e.items[i].(*Block)
		blk.merge(nb)
		for _, r := range blk.rows {
			e.rowIndex[r.key] = i
		}
		return nil
	}

	for key, i := range e.rowIndex {
		row, isRow := e.items[i].(*Row)
		if !isRow {
			continue
		}
		if prefix, _ := splitKey(key); prefix != nb.prefix {
			continue
		}
		if err := nb.Add(row); err != nil {
			return err
		}
		e.items[i] = nil
		delete(e.rowIndex, key)
		e.dirty = true
	}

	pos := len(e.items)
	e.blockIndex[nb.prefix] = pos
	e.items = append(e.items, nb)
	for _, r := range nb.rows {
		e.rowIndex[r.key] = pos
	}
	return nil
}

// Delete removes the row stored under key and reports whether one was present.
// Removing an absent key is not an error.
func (e *Env) Delete(key string) bool {
	k := NormalizeKey(key)
	if i, ok := e.rowIndex[k]; ok {
		switch v := e.items[i].(type) {
		case *Row:
			e.items[i] = nil
			delete(e.rowIndex, k)
			e.dirty = true
			return true
		case *Block:
			if v.Delete(k) {
				delete(e.rowIndex, k)
				return true
			}
			return false
		}
	}
	if prefix, _ := splitKey(k); prefix != "" {
		if i, ok := e.blockIndex[prefix]; ok && e.items[i].(*Block).Delete(k) {
			delete(e.rowIndex, k)
			return true
		}
	}
	return false
}

// DeleteBlock removes the block with the given prefix, and every row in it, and
// reports whether one was present.
func (e *Env) DeleteBlock(prefix string) bool {
	p := NormalizeKey(prefix)
	i, ok := e.blockIndex[p]
	if !ok {
		return false
	}
	for _, r := range e.items[i].(*Block).rows {
		delete(e.rowIndex, r.key)
	}
	e.items[i] = nil
	delete(e.blockIndex, p)
	e.dirty = true
	return true
}

// Merge folds other into e, treating other as the overriding definition.
//
// Rows absent here are copied; rows present are merged, which keeps an existing
// comment and does not let an empty incoming value erase a set one. Items are
// copied, so the two documents share no mutable state afterwards.
func (e *Env) Merge(other *Env) error {
	if other == nil {
		return nil
	}
	e.init()
	for it := range other.Items() {
		switch v := it.(type) {
		case *Row:
			if r := e.Get(v.key); r != nil {
				r.merge(v)
				continue
			}
			e.place(v.clone())
		case *Block:
			if i, ok := e.blockIndex[v.prefix]; ok {
				e.items[i].(*Block).merge(v)
				continue
			}
			if err := e.addBlock(v.clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

// SortByKey orders the document by key, including the rows inside each block.
//
// Sorting is explicit: reading a document preserves its order, so that writing
// it back produces a diff limited to what actually changed.
func (e *Env) SortByKey() {
	e.compact()
	slices.SortStableFunc(e.items, func(x, y Item) int {
		return strings.Compare(x.Key(), y.Key())
	})
	for _, it := range e.items {
		if b, ok := it.(*Block); ok {
			b.sortRows()
		}
	}
	e.reindex()
}

// Items iterates the document's top-level items in order.
func (e *Env) Items() iter.Seq[Item] {
	e.compact()
	return slices.Values(e.items)
}

// Rows iterates every row in the document, including those inside blocks, in
// document order.
func (e *Env) Rows() iter.Seq[*Row] {
	e.compact()
	return func(yield func(*Row) bool) {
		for _, it := range e.items {
			switch v := it.(type) {
			case *Row:
				if !yield(v) {
					return
				}
			case *Block:
				for _, r := range v.rows {
					if !yield(r) {
						return
					}
				}
			}
		}
	}
}

// All iterates every row as a key/value pair, in document order.
func (e *Env) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for r := range e.Rows() {
			if !yield(r.key, r.value) {
				return
			}
		}
	}
}

// Export writes the document into the process environment.
//
// With override false an existing variable is left alone. The first failure
// stops the walk and is returned.
func (e *Env) Export(override bool) error {
	for r := range e.Rows() {
		if r.commented {
			continue
		}
		if !override {
			if _, present := os.LookupEnv(r.key); present {
				continue
			}
		}
		if err := os.Setenv(r.key, r.value); err != nil {
			return fmt.Errorf("envi: setting %s: %w", r.key, err)
		}
	}
	return nil
}
