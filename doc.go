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
