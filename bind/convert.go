package bind

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// A setter fills one field from its textual value. Setters are chosen once,
// when a type's plan is built, so binding itself does no type dispatch.
type setter func(v reflect.Value, s string) error

var (
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	durationType        = reflect.TypeFor[time.Duration]()
)

// converterFor returns the setter for a field type, or an error wrapping
// [ErrUnsupportedType].
func converterFor(t reflect.Type, sep string) (setter, error) {
	// A type that knows how to read itself wins over anything below, which is
	// what makes time.Time, net.IP, uuid.UUID and friends work unmodified.
	if reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return setViaText, nil
	}
	// Duration must be caught before the integer kinds it is built on.
	if t == durationType {
		return setDuration, nil
	}

	switch t.Kind() {
	case reflect.String:
		return func(v reflect.Value, s string) error { v.SetString(s); return nil }, nil

	case reflect.Bool:
		return func(v reflect.Value, s string) error {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return err
			}
			v.SetBool(b)
			return nil
		}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bits := t.Bits()
		return func(v reflect.Value, s string) error {
			n, err := strconv.ParseInt(s, 10, bits)
			if err != nil {
				return err
			}
			v.SetInt(n)
			return nil
		}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bits := t.Bits()
		return func(v reflect.Value, s string) error {
			n, err := strconv.ParseUint(s, 10, bits)
			if err != nil {
				return err
			}
			v.SetUint(n)
			return nil
		}, nil

	case reflect.Float32, reflect.Float64:
		bits := t.Bits()
		return func(v reflect.Value, s string) error {
			f, err := strconv.ParseFloat(s, bits)
			if err != nil {
				return err
			}
			v.SetFloat(f)
			return nil
		}, nil

	case reflect.Pointer:
		inner, err := converterFor(t.Elem(), sep)
		if err != nil {
			return nil, err
		}
		return func(v reflect.Value, s string) error {
			p := reflect.New(t.Elem())
			if err := inner(p.Elem(), s); err != nil {
				return err
			}
			v.Set(p)
			return nil
		}, nil

	case reflect.Slice:
		return sliceConverter(t, sep)

	case reflect.Map:
		return mapConverter(t, sep)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedType, t)
	}
}

// sliceConverter builds the setter for a slice field.
func sliceConverter(t reflect.Type, sep string) (setter, error) {
	// []byte carries the text itself rather than a list of numbers, which is
	// what every caller means by it.
	if t.Elem().Kind() == reflect.Uint8 {
		return func(v reflect.Value, s string) error { v.SetBytes([]byte(s)); return nil }, nil
	}

	elem, err := converterFor(t.Elem(), sep)
	if err != nil {
		return nil, err
	}
	return func(v reflect.Value, s string) error {
		parts := strings.Split(s, sep)
		out := reflect.MakeSlice(t, len(parts), len(parts))
		for i, p := range parts {
			if err := elem(out.Index(i), strings.TrimSpace(p)); err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
		}
		v.Set(out)
		return nil
	}, nil
}

// mapConverter builds the setter for a map field. Entries are separated like
// slice elements and each entry is "key:value".
func mapConverter(t reflect.Type, sep string) (setter, error) {
	keyConv, err := converterFor(t.Key(), sep)
	if err != nil {
		return nil, err
	}
	valConv, err := converterFor(t.Elem(), sep)
	if err != nil {
		return nil, err
	}

	return func(v reflect.Value, s string) error {
		m := reflect.MakeMap(t)
		for _, entry := range strings.Split(s, sep) {
			rawKey, rawVal, ok := strings.Cut(entry, ":")
			if !ok {
				return fmt.Errorf("entry %q is not in key:value form", entry)
			}
			kv := reflect.New(t.Key()).Elem()
			if err := keyConv(kv, strings.TrimSpace(rawKey)); err != nil {
				return fmt.Errorf("key %q: %w", rawKey, err)
			}
			vv := reflect.New(t.Elem()).Elem()
			if err := valConv(vv, strings.TrimSpace(rawVal)); err != nil {
				return fmt.Errorf("value for %q: %w", rawKey, err)
			}
			m.SetMapIndex(kv, vv)
		}
		v.Set(m)
		return nil
	}, nil
}

func setViaText(v reflect.Value, s string) error {
	if !v.CanAddr() {
		return fmt.Errorf("%w: %s is not addressable", ErrUnsupportedType, v.Type())
	}
	return v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
}

func setDuration(v reflect.Value, s string) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	v.SetInt(int64(d))
	return nil
}
