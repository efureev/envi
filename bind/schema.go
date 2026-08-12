package bind

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	envi "github.com/efureev/envi/v2"
)

// A field is one leaf of a struct, resolved down to the key it reads and the
// setter that fills it.
type field struct {
	index  []int  // path to the field, through nested structs
	name   string // Go path, for error messages
	key    string // key to look up, before any runtime prefix
	def    string
	hasDef bool
	req    bool
	set    setter
}

// A plan is everything binding needs to know about a struct type.
type plan struct {
	fields []field
}

type planKey struct {
	t   reflect.Type
	tag string
}

// planCache keeps one plan per struct type, so reflection is paid on the first
// value of a type and never again.
var planCache sync.Map // planKey -> *plan or error

func planFor(t reflect.Type, tag string) (*plan, error) {
	k := planKey{t: t, tag: tag}
	if v, ok := planCache.Load(k); ok {
		switch cached := v.(type) {
		case *plan:
			return cached, nil
		case error:
			return nil, cached
		}
	}

	p := &plan{}
	err := appendFields(p, t, tag, nil, "", "", map[reflect.Type]bool{t: true})
	if err != nil {
		planCache.Store(k, err)
		return nil, err
	}
	planCache.Store(k, p)
	return p, nil
}

// appendFields walks a struct type, descending into nested structs and
// recording a leaf for every field it can fill.
func appendFields(p *plan, t reflect.Type, tag string, index []int, namePrefix, keyPrefix string, seen map[reflect.Type]bool) error {
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		opts := parseTag(sf.Tag.Get(tag))
		if opts.skip {
			continue
		}

		idx := append(append(make([]int, 0, len(index)+1), index...), i)
		goName := namePrefix + sf.Name

		if nested, ok := structUnder(sf.Type); ok {
			// A cycle in the type graph would otherwise recurse forever.
			if seen[nested] {
				continue
			}
			seen[nested] = true

			childKeys := keyPrefix
			if opts.name != "" {
				childKeys = joinKey(keyPrefix, opts.name)
			}
			if err := appendFields(p, nested, tag, idx, goName+".", childKeys, seen); err != nil {
				return err
			}

			delete(seen, nested)
			continue
		}

		key := opts.name
		if key == "" {
			key = camelToSnake(sf.Name)
		}

		set, err := converterFor(sf.Type, opts.sep)
		if err != nil {
			return fmt.Errorf("bind: field %s: %w", goName, err)
		}

		p.fields = append(p.fields, field{
			index:  idx,
			name:   goName,
			key:    envi.NormalizeKey(joinKey(keyPrefix, key)),
			def:    opts.def,
			hasDef: opts.hasDef,
			req:    opts.req,
			set:    set,
		})
	}
	return nil
}

// structUnder reports the struct type a field descends into, if any. A struct
// that can read itself from text is a value, not a group of fields.
func structUnder(t reflect.Type) (reflect.Type, bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, false
	}
	if reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return nil, false
	}
	return t, true
}

// tagOpts is a parsed struct tag.
type tagOpts struct {
	name   string
	def    string
	hasDef bool
	req    bool
	sep    string
	skip   bool
}

// parseTag reads `env:"NAME,required,separator=;,default=..."`.
//
// A default may contain commas, so "default=" consumes the rest of the tag and
// must therefore come last.
func parseTag(tag string) tagOpts {
	o := tagOpts{sep: defaultSeparator}
	if tag == "-" {
		o.skip = true
		return o
	}
	if tag == "" {
		return o
	}

	name, rest, _ := strings.Cut(tag, ",")
	o.name = name

	for rest != "" {
		if after, ok := strings.CutPrefix(rest, "default="); ok {
			o.def, o.hasDef = after, true
			break
		}
		var part string
		part, rest, _ = strings.Cut(rest, ",")
		switch {
		case part == "required":
			o.req = true
		case strings.HasPrefix(part, "separator="):
			if sep := strings.TrimPrefix(part, "separator="); sep != "" {
				o.sep = sep
			}
		}
	}
	return o
}

// joinKey combines a prefix and a key with the block separator.
func joinKey(prefix, key string) string {
	switch {
	case prefix == "":
		return key
	case key == "":
		return prefix
	default:
		return prefix + "_" + key
	}
}

// camelToSnake turns a Go field name into the shape a .env key takes.
//
//	AppName  -> APP_NAME
//	DBHost   -> DB_HOST
//	HTTPPort -> HTTP_PORT
//
// The result still goes through [envi.NormalizeKey], which upper-cases it.
func camelToSnake(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)

	for i := range len(s) {
		c := s[i]
		if i > 0 && isUpper(c) {
			prev := s[i-1]
			var next byte
			if i+1 < len(s) {
				next = s[i+1]
			}
			// Break after a word, and before the last capital of a run that
			// starts a new one: HTTPPort splits between P and P.
			if isLower(prev) || isDigit(prev) || (isUpper(prev) && isLower(next)) {
				b.WriteByte('_')
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
