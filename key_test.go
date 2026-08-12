package envi_test

import (
	"regexp"
	"strings"
	"testing"

	envi "github.com/efureev/envi/v2"
)

func TestNormalizeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"APP_NAME", "APP_NAME"},
		{"app_name", "APP_NAME"},
		{"app-section_name.sub", "APP_SECTION_NAME.SUB"},
		{"a--b", "A_B"},
		{"a__b", "A_B"},
		{"-leading", "_LEADING"},
		{"trailing-", "TRAILING_"},
		{"MIX_ed-Case.v2", "MIX_ED_CASE.V2"},
		{"app.name", "APP.NAME"},
		{"a b", "A_B"},
		{"9lives", "9LIVES"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := envi.NormalizeKey(tc.in); got != tc.want {
				t.Errorf("NormalizeKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeKeyMatchesReference pins the hand-written normaliser to the
// semantics a regexp and a per-character split express. The reference below is
// that slow, obviously-correct formulation, kept only as an oracle: it is what
// the fast single-pass implementation has to agree with.
func TestNormalizeKeyMatchesReference(t *testing.T) {
	t.Parallel()

	reference := func(key string) string {
		key = strings.ToUpper(key)
		key = regexp.MustCompile(`[^\w.]`).ReplaceAllString(key, "_")

		chars := strings.Split(key, "")
		for i := 0; i < len(chars)-1; {
			if chars[i] != "_" {
				i++
				continue
			}
			if chars[i] == chars[i+1] {
				chars = append(chars[:i], chars[i+1:]...)
			} else {
				i++
			}
		}
		return strings.Join(chars, "")
	}

	inputs := []string{
		"", "x", "APP_NAME", "app_name", "app-section_name.sub",
		"a__b", "a--b", "-leading", "trailing-", "MIX_ed-Case.v2",
		"app.name", "a b", "9lives", "a___________b", "...", "___",
		"Ünïcødé", "tab\there", "new\nline",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if got, want := envi.NormalizeKey(in), reference(in); got != want {
				t.Errorf("NormalizeKey(%q) = %q, reference gives %q", in, got, want)
			}
		})
	}
}

// A key already in canonical form must come back untouched, which is what lets
// the common path skip allocating.
func TestNormalizeKeyIdempotent(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"", "A", "APP_NAME", "A.B_C9", "_", "APP_"} {
		if got := envi.NormalizeKey(envi.NormalizeKey(in)); got != envi.NormalizeKey(in) {
			t.Errorf("NormalizeKey not idempotent for %q", in)
		}
	}
}
