package envi

import (
	"encoding/json"
	"errors"
	"io"
	"iter"
	"slices"
	"strconv"
	"strings"
)

// A ChangeKind says how one key differs between two documents.
type ChangeKind int

const (
	// ChangeAdded marks a key the other document configures and this one does
	// not.
	ChangeAdded ChangeKind = iota

	// ChangeRemoved marks a key this document configures and the other does
	// not.
	ChangeRemoved

	// ChangeChanged marks a key both configure, with different values.
	ChangeChanged
)

// String implements [fmt.Stringer].
func (k ChangeKind) String() string {
	switch k {
	case ChangeAdded:
		return "added"
	case ChangeRemoved:
		return "removed"
	case ChangeChanged:
		return "changed"
	default:
		return "unknown"
	}
}

// MarshalJSON writes the kind as its name, so that a comparison stays readable
// once it has left the process.
func (k ChangeKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// UnmarshalJSON reads a kind written as its name.
func (k *ChangeKind) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	switch name {
	case "added":
		*k = ChangeAdded
	case "removed":
		*k = ChangeRemoved
	case "changed":
		*k = ChangeChanged
	default:
		return errors.New("envi: unknown change kind " + strconv.Quote(name))
	}
	return nil
}

// A Change is one key that differs between two documents.
type Change struct {
	// Kind says which way the key differs.
	Kind ChangeKind `json:"kind"`

	// Key is the key, in the normalised form [NormalizeKey] produces.
	Key string `json:"key"`

	// Old is the value in the document [Env.Diff] was called on, and New the
	// value in the one it was passed. Old is empty for [ChangeAdded] and New
	// for [ChangeRemoved], where there is nothing to report.
	Old string `json:"old,omitempty"`
	New string `json:"new,omitempty"`
}

// String renders the change the way a diff reads: a sigil, then the key, then
// the values. An addition is marked "+", a removal "-" and a changed value
// "~", which shows both sides separated by an arrow.
//
// Values are quoted. A configuration value may be empty or carry meaningful
// spaces, and unquoted there is no telling "" from a value that did not
// change. The exact shape is in the [Env.Diff] example.
func (c Change) String() string {
	var b []byte
	switch c.Kind {
	case ChangeAdded:
		b = append(b, "+ "...)
		b = append(b, c.Key...)
		b = append(b, '=')
		b = strconv.AppendQuote(b, c.New)
	case ChangeRemoved:
		b = append(b, "- "...)
		b = append(b, c.Key...)
		b = append(b, '=')
		b = strconv.AppendQuote(b, c.Old)
	case ChangeChanged:
		b = append(b, "~ "...)
		b = append(b, c.Key...)
		b = append(b, ": "...)
		b = strconv.AppendQuote(b, c.Old)
		b = append(b, " -> "...)
		b = strconv.AppendQuote(b, c.New)
	default:
		b = append(b, c.Kind.String()...)
		b = append(b, ' ')
		b = append(b, c.Key...)
	}
	return string(b)
}

// A Delta is the set of differences between two documents, in the order
// [Env.Diff] found them.
//
// The zero Delta is a valid empty one.
type Delta struct {
	changes []Change
}

// Empty reports whether the two documents configure the same keys with the same
// values.
func (d *Delta) Empty() bool { return len(d.changes) == 0 }

// Len returns the number of differences.
func (d *Delta) Len() int { return len(d.changes) }

// Count returns how many differences are of the given kind.
func (d *Delta) Count(k ChangeKind) int {
	n := 0
	for _, c := range d.changes {
		if c.Kind == k {
			n++
		}
	}
	return n
}

// All iterates the differences in order.
func (d *Delta) All() iter.Seq[Change] { return slices.Values(d.changes) }

// Text writes the comparison to w, one change per line — see [Change.String].
// An empty Delta writes nothing.
func (d *Delta) Text(w io.Writer) error {
	for _, c := range d.changes {
		if _, err := io.WriteString(w, c.String()); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

// JSON writes the comparison to w as an array of changes, one object each, with
// the fields of [Change]. An empty Delta writes an empty array, not null, so
// that the output is the same shape however the comparison went.
func (d *Delta) JSON(w io.Writer) error {
	changes := d.changes
	if changes == nil {
		changes = []Change{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(changes)
}

// String returns what [Delta.Text] writes.
func (d *Delta) String() string {
	var b strings.Builder
	// Writing to a strings.Builder cannot fail.
	_ = d.Text(&b)
	return b.String()
}

// Diff reports what it would take to turn e into other: which keys other adds,
// which it drops, and which it gives a different value.
//
// Only values are compared. Comments, trailing comments, shadows, block
// membership and order are how a document is written rather than what it
// configures, and none of them produce a [Change]. For those, read the file
// with a text diff.
//
// A commented-out row does not configure anything, so it counts as absent —
// the same view [Env.Export] takes. This is deliberately not the view
// [Env.Lookup] takes, which hands back a commented row's value; comparing
// configurations is the question Diff answers. So a key that is live here and
// commented out in other is [ChangeRemoved], and two documents that comment the
// same key out with different values do not differ at all.
//
// Changes come in the order of this document, followed by the additions in the
// order of the other, which makes the result stable across runs.
//
// Either side may be nil: comparing against nil reports every configured key as
// removed.
func (e *Env) Diff(other *Env) *Delta {
	d := &Delta{}

	var configured map[string]string
	if other != nil {
		configured = make(map[string]string, other.Len())
		for r := range other.Rows() {
			if !r.commented {
				configured[r.key] = r.value
			}
		}
	}

	if e != nil {
		for r := range e.Rows() {
			if r.commented {
				continue
			}
			value, ok := configured[r.key]
			switch {
			case !ok:
				d.changes = append(d.changes, Change{Kind: ChangeRemoved, Key: r.key, Old: r.value})
			case value != r.value:
				d.changes = append(d.changes, Change{
					Kind: ChangeChanged, Key: r.key, Old: r.value, New: value,
				})
			}
		}
	}

	if other != nil {
		for r := range other.Rows() {
			if r.commented || e.configures(r.key) {
				continue
			}
			d.changes = append(d.changes, Change{Kind: ChangeAdded, Key: r.key, New: r.value})
		}
	}
	return d
}

// configures reports whether the document holds a live row under key.
//
// [Env.Get] answers for commented rows too, so the flag has to be checked here:
// without it a commented "# K=v" would hide the addition of a live one.
func (e *Env) configures(key string) bool {
	if e == nil {
		return false
	}
	r := e.Get(key)
	return r != nil && !r.commented
}
