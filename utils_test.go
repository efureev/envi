package envi

import (
	"strings"
	"testing"
)

func TestRemoveAdjacentDups(t *testing.T) {

	list := map[string][][]string{
		"geksforgeg": {{"g", "g", "e", "e", "k", "s", "f", "o", "r", "g", "e", "e", "e", "e", "g", "g", "g", "g", "g", "g"}},
		"g":          {{"g", "g", "g", "g", "g", "g", "g", "g", "g", "g"}, {"g", "g", "g"}, {"g", "g"}, {"g"}},
	}

	for exp, l := range list {
		for _, s := range l {
			out := removeAdjacentDups(s)

			if strings.Join(out, ``) != exp {
				t.Fatalf(`Must be equal: '%s' == '%s'`, strings.Join(out, ``), exp)
			}
		}
	}

}

func TestRemoveAdjacentDupsOnly(t *testing.T) {

	str := []string{"g", "g", "e", "e", "k", "s", "f", "o", "r", "g", "e", "e", "e", "e", "g", "g", "g", "g", "g", "g"}
	exp := []string{"g", "e", "e", "k", "s", "f", "o", "r", "g", "e", "e", "e", "e", "g"}

	out := removeAdjacentDupsOnly(str, `g`)

	if strings.Join(out, ``) != strings.Join(exp, ``) {
		t.Fatalf(`Must be equal: '%s' == '%s'`, strings.Join(out, ``), strings.Join(exp, ``))
	}

}

func TestRemoveAdjacentDuplicates(t *testing.T) {

	list := map[string][]string{
		"geksforgeg": {"ggeeeeksforgeeeeggg", "ggeeksforgeeeeg"},
		"g":          {"g", "ggggggggg", "ggg"},
	}

	for exp, l := range list {
		for _, s := range l {
			out := removeAdjacentDuplicates(s)

			if out != exp {
				t.Fatalf(`Must be equal: '%s' == '%s'`, out, exp)
			}
		}
	}
}
func TestRemoveAdjacentDuplicatesOnly(t *testing.T) {

	list := map[string][]string{
		"geeeeksforgeeeeg": {"ggeeeeksforgeeeeggg", "geeeeksforggggggggggeeeeggg"},
		"g":                {"g", "ggggggggg", "ggg"},
	}

	for exp, l := range list {
		for _, s := range l {
			out := removeAdjacentDuplicatesOnly(s, `g`)

			if out != exp {
				t.Fatalf(`Must be equal: '%s' == '%s'`, out, exp)
			}
		}
	}
}
