package envi

import "strings"

// blockSeparator divides a block prefix from the rest of a key.
const blockSeparator = '_'

// NormalizeKey converts s into the canonical form used for keys throughout this
// package: upper case, with every byte outside [A-Z0-9._] replaced by an
// underscore and runs of underscores collapsed to one.
//
//	NormalizeKey("app-section.name") == "APP_SECTION.NAME"
//	NormalizeKey("a--b")             == "A_B"
//
// A key that is already canonical is returned unchanged and without allocating.
//
// Normalisation is byte-oriented: a multi-byte rune collapses to a single
// underscore, matching the ASCII-only word class the format uses.
func NormalizeKey(s string) string {
	if keyIsCanonical(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	prevUnderscore := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - 'a' + 'A')
			prevUnderscore = false
		case (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.':
			b.WriteByte(c)
			prevUnderscore = false
		default:
			// Underscores and every other byte fold into a single separator.
			if !prevUnderscore {
				b.WriteByte(blockSeparator)
			}
			prevUnderscore = true
		}
	}
	return b.String()
}

// keyIsCanonical reports whether s is already in the form NormalizeKey
// produces, letting the common case skip the builder entirely.
func keyIsCanonical(s string) bool {
	prevUnderscore := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.':
			prevUnderscore = false
		case c == blockSeparator:
			if prevUnderscore {
				return false
			}
			prevUnderscore = true
		default:
			return false
		}
	}
	return true
}

// splitKey divides a canonical key at its first underscore, which is how a row
// is assigned to a block. It returns an empty prefix when the key carries no
// underscore, meaning the row belongs at top level.
//
//	splitKey("APP_NAME")     == "APP", "NAME"
//	splitKey("APP_DB_HOST")  == "APP", "DB_HOST"
//	splitKey("HYPE")         == "",    "HYPE"
func splitKey(key string) (prefix, rest string) {
	i := strings.IndexByte(key, blockSeparator)
	if i <= 0 || i == len(key)-1 {
		// No separator, or nothing on one side of it: not a block member.
		return "", key
	}
	return key[:i], key[i+1:]
}
