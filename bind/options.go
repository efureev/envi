package bind

import "reflect"

// defaultTagName is the struct tag this package reads.
const defaultTagName = "env"

// defaultSeparator splits the elements of a slice or map value.
const defaultSeparator = ","

// config is the resolved set of options for one call.
//
// Fields are ordered widest first so that the two flags share one word. Without
// that, adding the converter map pushed the struct into the next size class and
// cost 16 bytes on every call, including calls that register no converter.
type config struct {
	files         []string
	optionalFiles []string
	prefix        string
	tagName       string

	// converters holds the setters registered with [WithConverter]. It stays
	// nil until one is, which is what keeps the feature free for callers who
	// do not use it: a nil map is what tells planFor it may use the cache.
	converters map[reflect.Type]setter

	environ    bool
	requireAll bool
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

// WithConverter registers fn as the way to read a value of type T, for types
// this package cannot fill on its own.
//
//	bind.Load(&cfg, bind.WithConverter(url.Parse))
//
// The usual reason to need one is a type from another package that does not
// implement [encoding.TextUnmarshaler] and cannot be given the method — for
// example [net/url.URL].
//
// A registered type wins over everything else, including its own
// [encoding.TextUnmarshaler], so a type whose text form means something other
// than what a configuration file should say can be read differently here
// without changing the type.
//
// Registering the value type covers a field holding a pointer to it as well,
// because a pointer field is filled by reading the value it points at.
// Registering a pointer type does not cover the value.
//
// Elements are covered too: a converter for T fills fields of type []T and
// map[K]T without further registration.
//
// Calls accumulate, one per type, and registering the same type twice keeps the
// last. A nil fn registers nothing. Registering three types costs no more than
// registering one: what a converter suspends — caching the type's plan — is
// suspended by the first and not again by the rest.
//
// fn must be func(string) (T, error) exactly. A function that returns more, as
// [net.ParseCIDR] does, is adapted by wrapping it:
//
//	parseCIDR := func(s string) (*net.IPNet, error) {
//	    _, network, err := net.ParseCIDR(s)
//	    return network, err
//	}
//
// The reflection this package normally pays once per type is paid once per
// call while any converter is registered, because what a converter changes is
// the plan itself. Binding a configuration at startup will not notice; binding
// in a loop should hold the result rather than the options.
func WithConverter[T any](fn func(string) (T, error)) Option {
	if fn == nil {
		return nil
	}

	t := reflect.TypeFor[T]()
	set := func(v reflect.Value, s string) error {
		out, err := fn(s)
		if err != nil {
			return err
		}
		// Taken through a pointer rather than reflect.ValueOf(out): for an
		// interface T holding nil, the direct form loses the static type and
		// Set would panic.
		v.Set(reflect.ValueOf(&out).Elem())
		return nil
	}

	return optionFunc(func(c *config) {
		if c.converters == nil {
			c.converters = make(map[reflect.Type]setter, 4)
		}
		c.converters[t] = set
	})
}
