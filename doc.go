// Package envi parses, edits and writes .env files without losing their shape.
//
// Unlike loaders that only expose values, envi models the document itself:
// comments, blank-line grouping and shadowed values survive a read-modify-write
// cycle. With the default options a parsed document is written back byte for
// byte identical to its input.
//
// # Document model
//
// A document is an ordered sequence of [Item] values, each of which is either a
// [Row] (one KEY=value entry) or a [Block] (rows sharing a prefix up to the
// first underscore, optionally introduced by a header comment).
//
// A row may carry a comment, may itself be commented out, and may own any
// number of shadows — commented-out alternatives of the same key kept next to
// the active value:
//
//	# for docker
//	# REDIS_HOST=redis
//	REDIS_HOST=127.0.0.1
//
// Here REDIS_HOST has one shadow, redis.
//
// # Reading and writing
//
// [Parse], [ParseBytes], [ParseString] and [Load] read documents; [Env.WriteTo],
// [Env.MarshalText] and [Save] write them. Both directions accept the same
// [Option] values, so a document can be re-encoded with different formatting
// without being re-read.
//
// # Arranging a document
//
// Reading never rearranges anything, so a read-modify-write cycle produces a
// diff confined to what changed. Rearranging is asked for: [Env.SortByKey]
// sorts, [Env.Regroup] gathers every key sharing a prefix into one block
// wherever those keys sit, and [Env.Tidy] does both. A row that moves gives up
// its verbatim rendering, since the lines recorded above it described where it
// used to be; a document already in order is left untouched.
//
// # Checking a document
//
// [Check] and its variants read and validate in one pass, collecting every
// problem into a [Report] instead of stopping at the first malformed line the
// way [Parse] does. [Env.Check] runs over a document already in memory the
// rules that need no source text. Individual rules switch off with
// [WithoutRules].
//
// # Comparing documents
//
// [Env.Diff] reports what it would take to turn one document into another: the
// keys added, the keys dropped and the values changed, as a [Delta] that can be
// iterated, counted, printed or written as JSON.
//
// It compares what a document configures, not how it is written. Comments,
// shadows, block membership and order produce no [Change], and a commented-out
// row counts as absent — the view [Env.Export] takes rather than the one
// [Env.Lookup] takes. That makes it the right tool for checking a working .env
// against the .env.example that documents it, and the wrong one for reviewing
// an edit to the file itself.
//
// # Options replace global state
//
// Every knob is an [Option] passed to the operation that uses it. The package
// keeps no mutable global state, so two independent callers in one process
// cannot disturb each other's output.
//
// # Concurrency
//
// A *Env must not be mutated concurrently; guard it like any other mutable
// value. [Decoder] and [Encoder] share no state between instances, so parsing
// and encoding from several goroutines at once is safe.
package envi
