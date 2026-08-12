package envi

// An Item is one element of a document at top level: either a [*Row] or a
// [*Block].
//
// The interface is closed. Its unexported method cannot be implemented outside
// this package, so a type switch over an Item covers every case that will ever
// exist and needs no default branch beyond a defensive one.
type Item interface {
	// Key returns the identity of the item within the document: a row's full
	// key, or a block's prefix. Keys are normalised (see [NormalizeKey]) and
	// unique within one [Env].
	Key() string

	// sealed prevents implementations outside this package.
	sealed()
}
