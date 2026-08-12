package bind

import (
	"errors"
	"os"
	"reflect"
	"strings"

	envi "github.com/efureev/envi/v2"
)

// A Source supplies values by key. [*envi.Env] satisfies it, and so does
// anything else a caller already has.
type Source interface {
	// Lookup returns the value stored under key and whether it was present.
	// Keys arrive normalised, in the upper-case form [envi.NormalizeKey]
	// produces.
	Lookup(key string) (value string, ok bool)
}

// Load reads the configured files and environment and fills dst.
//
// dst must be a non-nil pointer to a struct. Every field that fails is
// reported; the returned error is an [*Error] holding all of them.
//
//	type Config struct {
//	    Name  string        `env:"APP_NAME"`
//	    Port  int           `env:"APP_PORT,required"`
//	    TTL   time.Duration `env:"CACHE_TTL,default=30s"`
//	}
//
//	var cfg Config
//	err := bind.Load(&cfg, bind.WithOptionalFiles(".env"), bind.WithEnviron())
//
// With neither [WithFiles] nor [WithOptionalFiles] nor [WithEnviron], there is
// nothing to read and every field falls back to its default.
func Load(dst any, opts ...Option) error {
	cfg := newConfig(opts)
	src, err := gather(&cfg)
	if err != nil {
		return err
	}
	return decode(src, dst, &cfg)
}

// Decode fills dst from src without touching the filesystem or the process
// environment.
func Decode(src Source, dst any, opts ...Option) error {
	cfg := newConfig(opts)
	return decode(src, dst, &cfg)
}

// gather assembles the source described by cfg: the files in order, then the
// process environment on top when asked for.
func gather(cfg *config) (Source, error) {
	env := envi.New()

	for _, path := range cfg.files {
		e, err := envi.Load(path)
		if err != nil {
			return nil, err
		}
		if err := env.Merge(e); err != nil {
			return nil, err
		}
	}
	for _, path := range cfg.optionalFiles {
		e, err := envi.Load(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := env.Merge(e); err != nil {
			return nil, err
		}
	}

	if cfg.environ {
		for _, kv := range os.Environ() {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env.Set(k, v)
			}
		}
	}
	return env, nil
}

func decode(src Source, dst any, cfg *config) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return ErrNotPointer
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return ErrNotStruct
	}

	p, err := planFor(rv.Type(), cfg.tagName)
	if err != nil {
		return err
	}

	var failures []FieldError
	for i := range p.fields {
		f := &p.fields[i]

		key := f.key
		if cfg.prefix != "" {
			key = envi.NormalizeKey(joinKey(cfg.prefix, key))
		}

		value, ok := "", false
		if src != nil {
			value, ok = src.Lookup(key)
		}
		// An empty value counts as absent: a key present but blank states that
		// nothing was configured, not that the field should be blanked.
		if !ok || value == "" {
			switch {
			case f.hasDef:
				value = f.def
			case f.req || cfg.requireAll:
				failures = append(failures, FieldError{Field: f.name, Key: key, Err: ErrRequired})
				continue
			default:
				continue
			}
		}

		if err := f.set(fieldByIndex(rv, f.index), value); err != nil {
			failures = append(failures, FieldError{Field: f.name, Key: key, Err: err})
		}
	}

	if len(failures) > 0 {
		return &Error{Fields: failures}
	}
	return nil
}

// fieldByIndex walks to a field, allocating the nil pointers on the way. It is
// only called once there is a value to store, so nothing is allocated for a
// nested struct that stays untouched.
func fieldByIndex(v reflect.Value, index []int) reflect.Value {
	v = v.Field(index[0])
	for _, i := range index[1:] {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}
