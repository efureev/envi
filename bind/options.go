package bind

// defaultTagName is the struct tag this package reads.
const defaultTagName = "env"

// defaultSeparator splits the elements of a slice or map value.
const defaultSeparator = ","

// config is the resolved set of options for one call.
type config struct {
	files         []string
	optionalFiles []string
	environ       bool
	prefix        string
	tagName       string
	requireAll    bool
}

func newConfig(opts []Option) config {
	c := config{tagName: defaultTagName}
	for _, o := range opts {
		if o != nil {
			o.apply(&c)
		}
	}
	return c
}

// An Option configures loading and decoding.
type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// WithFiles names .env files to read, in order, so that a later file overrides
// an earlier one. A missing file is an error; see [WithOptionalFiles].
//
// Used by [Load] only.
func WithFiles(paths ...string) Option {
	return optionFunc(func(c *config) { c.files = append(c.files, paths...) })
}

// WithOptionalFiles names .env files to read if they exist, which is the usual
// arrangement for a container image where configuration arrives through the
// environment and the file is only present in development.
//
// Used by [Load] only.
func WithOptionalFiles(paths ...string) Option {
	return optionFunc(func(c *config) { c.optionalFiles = append(c.optionalFiles, paths...) })
}

// WithEnviron overlays the process environment on top of whatever the files
// supplied, so that a variable set in the environment wins.
//
// Used by [Load] only.
func WithEnviron() Option {
	return optionFunc(func(c *config) { c.environ = true })
}

// WithPrefix prepends a prefix to every key a struct asks for, letting one
// program bind several components from one namespace.
//
//	WithPrefix("APP") turns the field key PORT into APP_PORT.
func WithPrefix(prefix string) Option {
	return optionFunc(func(c *config) { c.prefix = prefix })
}

// WithTagName reads field settings from a struct tag other than "env".
func WithTagName(name string) Option {
	return optionFunc(func(c *config) {
		if name != "" {
			c.tagName = name
		}
	})
}

// WithRequiredByDefault treats every field without a default as required,
// turning a missing value into an error instead of a zero field.
func WithRequiredByDefault() Option {
	return optionFunc(func(c *config) { c.requireAll = true })
}
