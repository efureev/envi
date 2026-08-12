package envi

import (
	"slices"
	"strings"
)

// Regroup rebuilds the document's block structure.
//
// Every row carrying the same prefix is gathered into one block, wherever those
// rows sat before: a document whose keys are scattered comes back grouped. A
// prefix carried by fewer rows than the grouping threshold — see
// [WithGroupThreshold] — leaves its rows at top level instead, so a block that
// has shrunk below the threshold is dissolved. Groups keep the order in which
// their prefix was first seen, and rows keep the order they had, so nothing is
// reordered beyond what grouping requires.
//
// A block already holding exactly the rows regrouping assigns it is left alone,
// header comment and all. Every other row moves, and a row that moves loses the
// verbatim rendering recorded for it, because the lines above it described
// where it used to be. A document already in order therefore comes through
// untouched and still writes back byte for byte identical; one that is not
// gives up reproduction in exchange for the tidying that was asked for.
func (e *Env) Regroup(opts ...Option) {
	e.relayout(newConfig(opts), false)
}

// Tidy puts the document in order: it regroups and then sorts by key, including
// the rows inside each block.
//
// It is [Env.Regroup] followed by [Env.SortByKey], and has the same effect on
// verbatim renderings — a row that ends up somewhere new is written from the
// model rather than reproduced.
func (e *Env) Tidy(opts ...Option) {
	e.relayout(newConfig(opts), true)
}

// rowPos records where a row sat before a relayout: the block holding it, or
// nil for top level, and its index within that container.
type rowPos struct {
	owner *Block
	idx   int
}

// relayout rebuilds the document's structure and drops the verbatim rendering
// of every row that ended up somewhere other than where it started.
//
// The rule is deliberately strict — same container object, same index, or the
// rendering goes — because a row's recorded prefix lines are the blank lines
// and comments that sat above it in the source, and those describe a position,
// not a value. A document that regrouping leaves alone keeps every rendering,
// which is what makes [Env.Regroup] free for a file that is already in order.
func (e *Env) relayout(cfg config, sort bool) {
	e.compact()

	before := make(map[*Row]rowPos, len(e.items))
	for i, it := range e.items {
		switch v := it.(type) {
		case *Row:
			before[v] = rowPos{idx: i}
		case *Block:
			for j, r := range v.rows {
				before[r] = rowPos{owner: v, idx: j}
			}
		}
	}
	was := slices.Clone(e.items)

	e.items = e.grouped(cfg)

	if sort {
		slices.SortStableFunc(e.items, func(x, y Item) int {
			return strings.Compare(x.Key(), y.Key())
		})
		for _, it := range e.items {
			if b, ok := it.(*Block); ok {
				b.sortRows()
			}
		}
	}

	e.dirty = false
	e.reindex()

	// Items hold pointers, so comparing the sequences catches a block that
	// changed place without any of its rows changing place inside it.
	changed := !slices.Equal(was, e.items)

	for i, it := range e.items {
		switch v := it.(type) {
		case *Row:
			if p, ok := before[v]; !ok || p.owner != nil || p.idx != i {
				v.dropRaw()
				changed = true
			}
		case *Block:
			for j, r := range v.rows {
				if p, ok := before[r]; !ok || p.owner != v || p.idx != j {
					r.dropRaw()
					changed = true
				}
			}
		}
	}
	if !changed {
		return
	}

	// The blank lines recorded after a block were measured against neighbours
	// the document no longer has: the parser puts the blanks that followed a
	// row into the next row's prefix, and that prefix has just been dropped.
	// Handing the spacing back to the configured indent is both what keeps a
	// rearranged block from running into what follows it and what makes the
	// result evenly spaced, which is the point of tidying.
	for _, it := range e.items {
		if b, ok := it.(*Block); ok {
			b.blanksAfter = -1
		}
	}
}

// group collects the rows sharing one prefix while the document is walked.
type group struct {
	prefix string
	rows   []*Row

	// src is the block every row of the group came from, or nil once rows have
	// been seen from more than one container. Only a group that is still whole
	// can reuse its block and the verbatim lines that block carries.
	src *Block

	// loose marks a group standing for a single row with no prefix, which never
	// becomes a block however low the threshold is set.
	loose bool
}

// grouped returns the document's items rearranged so that every prefix carried
// by at least the threshold number of rows forms exactly one block.
func (e *Env) grouped(cfg config) []Item {
	var groups []*group
	at := make(map[string]int, len(e.items))

	add := func(r *Row, from *Block) {
		prefix, _ := splitKey(r.key)
		if prefix == "" {
			groups = append(groups, &group{rows: []*Row{r}, loose: true})
			return
		}
		i, ok := at[prefix]
		if !ok {
			at[prefix] = len(groups)
			groups = append(groups, &group{prefix: prefix, src: from})
			i = len(groups) - 1
		}
		g := groups[i]
		if g.src != from {
			g.src = nil
		}
		g.rows = append(g.rows, r)
	}

	// headers keeps the comment of whichever block introduced a prefix first, so
	// that a block rebuilt from scattered rows still says what it is for.
	headers := make(map[string]string)

	for _, it := range e.items {
		switch v := it.(type) {
		case *Row:
			add(v, nil)
		case *Block:
			if v.comment != "" {
				if _, seen := headers[v.prefix]; !seen {
					headers[v.prefix] = v.comment
				}
			}
			for _, r := range v.rows {
				add(r, v)
			}
		}
	}

	items := make([]Item, 0, len(groups))
	for _, g := range groups {
		if g.loose || len(g.rows) < cfg.groupThreshold {
			// A block dissolving below the threshold takes its header comment
			// with it unless something else claims the text, so it moves onto
			// the first row it introduced. Demoted to an ordinary comment it
			// reads a little plainer, which beats disappearing.
			if h := headers[g.prefix]; h != "" && g.rows[0].comment == "" {
				g.rows[0].SetComment(h)
			}
			for _, r := range g.rows {
				items = append(items, r)
			}
			continue
		}
		if g.src != nil && blockHolds(g.src, g.rows) {
			items = append(items, g.src)
			continue
		}
		blk := NewBlock(g.prefix)
		blk.comment = headers[g.prefix]
		// Every row carries the prefix by construction, so Add cannot fail.
		_ = blk.Add(g.rows...)
		items = append(items, blk)
	}
	return items
}

// blockHolds reports whether b already holds exactly rows, in that order.
func blockHolds(b *Block, rows []*Row) bool {
	if len(b.rows) != len(rows) {
		return false
	}
	for i, r := range rows {
		if b.rows[i] != r {
			return false
		}
	}
	return true
}
