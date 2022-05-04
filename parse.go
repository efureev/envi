package envi

import (
	"errors"
	"regexp"
	"strings"
)

const (
	linePattern = `\A\s*(?:export\s+)?([\w\.]+)(?:\s*=\s*|:\s+?)('(?:\'|[^'])*'|"(?:\"|[^"])*"|[^#\n]+)?\s*(?:\s*\#(.*))?\z`

	// Pattern for detecting valid variable within a value
	//variablePattern = `(\\)?(\$)(\{?([A-Z0-9_]+)?\}?)`
)

var ErrEmptyString = errors.New("zero length string")
var ErrOnlyComment = errors.New("only comment")
var ErrWrongLineFormat = errors.New("line doesn't match format")

func parseLine(line string) (key, value, comment string, err error) {
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		err = ErrEmptyString
		return
	}

	if strings.HasPrefix(line, "#") {

		strs := strings.Split(line, "\n")

		commentPre := removeAdjacentDuplicatesOnly(strs[0], `#`)
		commentPre = strings.ReplaceAll(commentPre, `#`, ``)
		commentPre = strings.TrimSpace(commentPre)

		/*if _, ok := rowCommentMatchBlock(strs[0]); ok {
			//spew.Dump(strs[0], cmt, ok)
			//os.Exit(32)
			commentPre = strs[0]
			//println(commentPre)
		} else {
			commentPre = removeAdjacentDuplicatesOnly(strs[0], `#`)
			commentPre = strings.ReplaceAll(commentPre, `#`, ``)
			commentPre = strings.TrimSpace(commentPre)
		}*/

		if len(strs) == 1 {
			return ``, ``, commentPre, ErrOnlyComment
		}

		k, v, c, e := parseLine(strings.Join(strs[1:], "\n"))
		if err == nil {
			comment = commentPre
			if c != `` {
				comment += "\n" + c
			}
			return k, v, comment, e
		}

		if e == ErrOnlyComment {
			comment = commentPre + "\n" + c
			return k, v, comment, e
		}
	}

	rl := regexp.MustCompile(linePattern)
	rm := rl.FindStringSubmatch(line)

	if len(rm) == 0 {
		return ``, ``, ``, checkFormat(line)
	}

	key = rm[1]
	value = rm[2]
	comment = strings.TrimSpace(rm[3])

	// trim whitespace
	value = strings.TrimSpace(value)

	// determine if string has quote prefix
	hdq := strings.HasPrefix(value, `"`)

	// determine if string has single quote prefix
	//	hsq := strings.HasPrefix(value, `'`)

	// remove quotes '' or ""
	rq := regexp.MustCompile(`\A(['"])(.*)(['"])\z`)
	value = rq.ReplaceAllString(value, "$2")

	if hdq {
		value = strings.Replace(value, `\n`, "\n", -1)
		value = strings.Replace(value, `\r`, "\r", -1)

		// Unescape all characters except $ so variables can be escaped properly
		re := regexp.MustCompile(`\\([^$])`)
		value = re.ReplaceAllString(value, "$1")
	}

	/*rv := regexp.MustCompile(variablePattern)
	fv := func(s string) string {
		return varReplacement(s, hsq, env)
	}

	value = rv.ReplaceAllStringFunc(value, fv)*/
	value = parseVal(value, hdq)

	return
}

func isCommentedRow(line string) (*row, error) {
	line = strings.TrimSpace(line)

	if line == "" || !strings.HasPrefix(line, "#") {
		return nil, ErrEmptyString
	}

	line = removeAdjacentDuplicatesOnly(line, `#`)
	line = strings.ReplaceAll(line, `#`, ``)

	key, value, comment, err := parseLine(line)
	if err != nil {
		return nil, err
	}

	return NewRow(key, value).SetComment(comment).Commented(), nil
}
func checkFormat(line string) error {
	st := strings.TrimSpace(line)

	if (st == "") || strings.HasPrefix(st, "#") {
		return nil
	}

	if err := parseExport(st); err != nil {
		return err
	}

	return ErrWrongLineFormat
}

func parseExport(line string) error {
	if strings.HasPrefix(line, "export") {
		vs := strings.SplitN(line, " ", 2)

		if len(vs) > 1 {
			/*if _, ok := env[vs[1]]; !ok {
				return fmt.Errorf("line `%s` has an unset variable", line)
			}*/
		}
	}

	return nil
}

func parseVal(val string, ignoreNewlines bool) string {
	if strings.Contains(val, "=") && !ignoreNewlines {
		kv := strings.Split(val, "\r")

		if len(kv) > 1 {
			val = kv[0]

			for i := 1; i < len(kv); i++ {
				parseLine(kv[i])
			}
		}
	}

	return val
}
