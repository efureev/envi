package envi

import (
	"iter"
	"slices"
)

// A Row is one KEY=value entry of a document, together with the comment above
// it and any shadows attached to it.
//
// A row always holds its full key, whether or not it sits inside a [Block].
// Block membership affects only how the row is grouped and introduced on
// output, never its identity — which is what keeps adding a row to a block from
// being able to rename or lose it.
//
// The zero Row is not usable; construct one with [NewRow].
type Row struct {
	key     string
	value   string
	comment string

	// inline is the comment trailing the assignment on its own line, kept
	// apart from comment because moving one to the other changes meaning:
	// "# KEY=v" on a line of its own reads back as a shadow, not as prose.
	inline string

	// rawLine is the assignment exactly as it appeared in the input, and
	// rawPrefix the verbatim lines above it that belong to this row: its
	// comments, its shadows and any blank lines among them.
	//
	// Together they are what makes a parsed document round-trip byte for
	// byte. Both are empty for rows built in memory, and both are dropped as
	// soon as the row changes, so [QuotePreserve] can never reproduce a
	// rendering that no longer matches the value.
	rawLine   string
	rawPrefix []string

	// parsed marks a row that came from input and has not changed since, so
	// rawPrefix is authoritative for the lines above it. rawLine may still be
	// empty for such a row: an assignment written in canonical form is
	// reproduced exactly by re-rendering it, and storing the text again would
	// cost an allocation per line for nothing.
	parsed bool

	shadows   []string
	commented bool
}

// NewRow returns a row with the given key and value. The key is normalised (see
// [NormalizeKey]).
func NewRow(key, value string) *Row {
	return &Row{key: NormalizeKey(key), value: value}
}

// Key returns the row's full, normalised key.
func (r *Row) Key() string { return r.key }

// Value returns the row's value with quoting and escaping already resolved.
func (r *Row) Value() string { return r.value }

// SetValue replaces the value and returns r for chaining.
//
// It also discards the recorded original rendering: once the value differs from
// what was read, reproducing the input verbatim would be wrong.
func (r *Row) SetValue(v string) *Row {
	r.value = v
	r.dropRaw()
	return r
}

// dropRaw forgets the verbatim rendering after the row's content changes.
func (r *Row) dropRaw() {
	r.rawLine = ""
	r.rawPrefix = nil
	r.parsed = false
}

// dropLine forgets the recorded rendering of the assignment while keeping the
// lines above it.
//
// It is for a change that invalidates what the row says but not what precedes
// it. The lines above belong to no other row, so discarding them would delete
// input outright — a blank line or a comment that nothing else records.
func (r *Row) dropLine() {
	r.rawLine = ""
	r.parsed = false
}

// Comment returns the comment attached above the row, without the leading "# "
// of each line. A multi-line comment is joined with newlines.
func (r *Row) Comment() string { return r.comment }

// SetComment replaces the row's comment and returns r for chaining.
func (r *Row) SetComment(s string) *Row {
	r.comment = s
	r.dropRaw()
	return r
}

// InlineComment returns the comment trailing the assignment, without its "#".
func (r *Row) InlineComment() string { return r.inline }

// SetInlineComment replaces the comment trailing the assignment and returns r
// for chaining.
func (r *Row) SetInlineComment(s string) *Row {
	r.inline = s
	r.dropRaw()
	return r
}

// IsCommented reports whether the row itself is commented out, that is written
// as "# KEY=value" and inert for a consumer of the file.
func (r *Row) IsCommented() bool { return r.commented }

// SetCommented marks the row as commented out, or restores it, and returns r
// for chaining.
func (r *Row) SetCommented(b bool) *Row {
	if r.commented != b {
		r.dropRaw()
	}
	r.commented = b
	return r
}

// NumShadows returns how many shadows the row carries.
func (r *Row) NumShadows() int { return len(r.shadows) }

// Shadows iterates the row's shadows — commented-out alternatives of the value,
// kept in the order they were seen.
func (r *Row) Shadows() iter.Seq[string] {
	return slices.Values(r.shadows)
}

// HasShadow reports whether the row already carries the given shadow.
func (r *Row) HasShadow(s string) bool {
	return slices.Contains(r.shadows, s)
}

// AddShadow records an alternative value for the row, ignoring duplicates, and
// returns r for chaining.
func (r *Row) AddShadow(s string) *Row {
	if !r.HasShadow(s) {
		r.shadows = append(r.shadows, s)
		r.dropRaw()
	}
	return r
}

// addParsedShadow records a shadow that came from the input, where the verbatim
// lines already account for it and must not be discarded.
func (r *Row) addParsedShadow(s string) {
	if !r.HasShadow(s) {
		r.shadows = append(r.shadows, s)
	}
}

// merge folds other into r, treating r as the earlier and other as the
// overriding definition.
//
// An empty incoming value does not erase an existing one: a later file that
// mentions a key without giving it a value is stating that the key exists, not
// that it is now blank.
func (r *Row) merge(other *Row) {
	if other.value != "" && other.value != r.value {
		r.value = other.value
		r.rawLine = other.rawLine
		r.rawPrefix = slices.Clone(other.rawPrefix)
		r.parsed = other.parsed
	}
	if r.inline == "" && other.inline != "" {
		r.inline = other.inline
		r.dropRaw()
	}
	if r.comment == "" && other.comment != "" {
		r.comment = other.comment
		// The recorded rendering no longer accounts for the comment above.
		r.dropRaw()
	}
	for _, s := range other.shadows {
		if !r.HasShadow(s) {
			r.shadows = append(r.shadows, s)
			r.dropRaw()
		}
	}
}

// clone returns an independent copy, so that merging one document into another
// cannot leave the two sharing mutable state.
func (r *Row) clone() *Row {
	c := *r
	c.shadows = slices.Clone(r.shadows)
	c.rawPrefix = slices.Clone(r.rawPrefix)
	return &c
}

func (r *Row) sealed() {}
