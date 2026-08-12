package envi

import "strconv"

// appendValue renders v into dst according to style.
//
// This is the path for a row with no verbatim rendering to reproduce: either it
// was built in memory, or it has been changed since it was read. Reproducing
// input is handled a level up, in the encoder.
func appendValue(dst []byte, v string, style QuoteStyle) []byte {
	if style == QuoteAlways {
		return appendAlways(dst, v)
	}
	return appendMinimal(dst, v)
}

// appendMinimal writes v unquoted when that survives a re-read, and
// double-quoted otherwise.
func appendMinimal(dst []byte, v string) []byte {
	if needsQuoting(v) {
		return appendQuoted(dst, v)
	}
	return append(dst, v...)
}

// appendAlways writes v double-quoted, except for the two boolean literals and
// decimal integers, which the format carries bare.
func appendAlways(dst []byte, v string) []byte {
	if v == "true" || v == "false" || isDecimalInt(v) {
		return append(dst, v...)
	}
	return appendQuoted(dst, v)
}

// needsQuoting reports whether v would be read back as something other than
// itself if written without quotes.
func needsQuoting(v string) bool {
	if v == "" {
		return false
	}
	// Leading or trailing space would be trimmed on the way back in.
	if v[0] == ' ' || v[0] == '\t' || v[len(v)-1] == ' ' || v[len(v)-1] == '\t' {
		return true
	}
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '#', '"', '\'', '\n', '\r', '\\', '$', '`':
			return true
		}
	}
	return false
}

// appendQuoted writes v wrapped in double quotes, escaping in a single pass.
func appendQuoted(dst []byte, v string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch c {
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\\', '"', '$', '`':
			dst = append(dst, '\\', c)
		default:
			dst = append(dst, c)
		}
	}
	return append(dst, '"')
}

// isDecimalInt reports whether v is a plain base-10 integer, the form the
// format carries without quotes.
func isDecimalInt(v string) bool {
	if v == "" {
		return false
	}
	i := 0
	if v[0] == '-' || v[0] == '+' {
		if len(v) == 1 {
			return false
		}
		i = 1
	}
	for ; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	// Reject forms strconv would not round-trip, such as a leading zero.
	_, err := strconv.ParseInt(v, 10, 64)
	return err == nil
}
