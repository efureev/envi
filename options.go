package envi

// QuoteStyle selects how values are quoted when a document is written.
type QuoteStyle int

const (
	// QuotePreserve writes each value exactly as it appeared in the input,
	// including whether it was quoted and with which quote character. Values
	// created programmatically are quoted only where quoting is required.
	// This is the default and the only style under which a parsed document is
	// written back byte for byte identical.
	QuotePreserve QuoteStyle = iota

	// QuoteMinimal drops quotes wherever the value survives without them.
	QuoteMinimal

	// QuoteAlways wraps every value in double quotes except the literals true
	// and false and decimal integers.
	QuoteAlways
)

// String implements [fmt.Stringer].
func (q QuoteStyle) String() string {
	switch q {
	case QuotePreserve:
		return "preserve"
	case QuoteMinimal:
		return "minimal"
	case QuoteAlways:
		return "always"
	default:
		return "unknown"
	}
}

// Order selects the order in which items are written.
type Order int

const (
	// OrderSource keeps the order in which items were read or added. This is
	// the default: it is what makes a read-modify-write cycle produce a diff
	// confined to the lines that actually changed.
	OrderSource Order = iota

	// OrderSorted writes items sorted by key.
	OrderSorted
)

// String implements [fmt.Stringer].
func (o Order) String() string {
	switch o {
	case OrderSource:
		return "source"
	case OrderSorted:
		return "sorted"
	default:
		return "unknown"
	}
}

// Default formatting, matching what the package writes when given no options.
const (
	defaultIndent             = 1
	defaultGroupThreshold     = 1
	defaultBlockCommentBefore = "###   ---[ "
	defaultBlockCommentAfter  = " ]---   ###"
)

// config is the resolved set of options for one operation. It is copied into
// each [Decoder] and [Encoder] at construction and never mutated afterwards,
// which is what makes concurrent use of separate instances safe.
type config struct {
	indent             int
	blockCommentBefore string
	blockCommentAfter  string
	groupThreshold     int
	shadows            bool
	comments           bool
	commentedRows      bool
	quoting            QuoteStyle
	order              Order
}

// newConfig resolves opts over the defaults.
func newConfig(opts []Option) config {
	c := config{
		indent:             defaultIndent,
		blockCommentBefore: defaultBlockCommentBefore,
		blockCommentAfter:  defaultBlockCommentAfter,
		groupThreshold:     defaultGroupThreshold,
		shadows:            true,
		comments:           true,
		commentedRows:      true,
		quoting:            QuotePreserve,
		order:              OrderSource,
	}
	for _, o := range opts {
		if o != nil {
			o.apply(&c)
		}
	}
	return c
}

// An Option configures parsing, encoding, or both. Options are values, so the
// same slice can be reused across calls and across goroutines.
type Option interface {
	apply(*config)
}

// optionFunc adapts a function to [Option].
type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// WithIndent sets how many blank lines follow each block on output. Negative
// values are treated as zero. Encoding only.
func WithIndent(n int) Option {
	if n < 0 {
		n = 0
	}
	return optionFunc(func(c *config) { c.indent = n })
}

// WithBlockComment sets the strings wrapping a block's header comment. They are
// used both to recognise a header while parsing and to write one while
// encoding, so a document round-trips only if the same values are used in both
// directions.
func WithBlockComment(before, after string) Option {
	return optionFunc(func(c *config) {
		c.blockCommentBefore = before
		c.blockCommentAfter = after
	})
}

// WithGroupThreshold sets how many rows must share a prefix before they are
// grouped into a [Block]. The default is 1, meaning every prefix forms a block.
// Values below 1 are treated as 1. Parsing only.
func WithGroupThreshold(n int) Option {
	if n < 1 {
		n = 1
	}
	return optionFunc(func(c *config) { c.groupThreshold = n })
}

// WithShadows controls whether shadows — commented-out alternatives of a value —
// are written. Encoding only.
func WithShadows(enabled bool) Option {
	return optionFunc(func(c *config) { c.shadows = enabled })
}

// WithComments controls whether comments attached to rows and blocks are
// written. Encoding only.
func WithComments(enabled bool) Option {
	return optionFunc(func(c *config) { c.comments = enabled })
}

// WithCommentedRows controls whether rows that are themselves commented out are
// written. Encoding only.
func WithCommentedRows(enabled bool) Option {
	return optionFunc(func(c *config) { c.commentedRows = enabled })
}

// WithQuoting selects the quoting style used on output. Encoding only.
func WithQuoting(q QuoteStyle) Option {
	return optionFunc(func(c *config) { c.quoting = q })
}

// WithOrder selects the order in which items are written. Encoding only;
// parsing always records source order.
func WithOrder(o Order) Option {
	return optionFunc(func(c *config) { c.order = o })
}
