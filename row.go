package envi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type row struct {
	commented    bool
	Key          string
	Value        string
	Comment      string
	blockComment string
	block        *Block
	shadows      []string
}

func (r row) Marshal() (string, error) {
	line := fmt.Sprintf(`%s=%s`, r.GetFullKey(), normalizeValue(r.Value))

	if r.commented {
		line = `# ` + line
	}

	if r.Comment != `` {
		comment := strings.ReplaceAll(r.Comment, "\n", "\n# ")
		line = `# ` + comment + "\n" + line
	}

	return line, nil
}

func formatRowLine(key, value string, commented bool) string {
	line := fmt.Sprintf(`%s=%s`, key, value)
	if commented {
		line = `# ` + line
	}
	return line
}

func (r row) MarshalSlice() (lines []string, err error) {
	line := formatRowLine(r.GetFullKey(), normalizeValue(r.Value), r.commented)

	if r.Comment != `` {
		comments := strings.Split(r.Comment, "\n")
		for k, c := range comments {
			comments[k] = `# ` + c
		}

		lines = append(lines, comments...)
	}

	if !r.commented {
		for _, s := range r.shadows {
			lines = append(lines, formatRowLine(r.GetFullKey(), normalizeValue(s), true))
		}
	}

	lines = append(lines, line)

	return lines, nil
}

func (r row) GetKey() string {
	return r.Key
}

func (r row) GetFullKey() string {
	key := r.Key
	if r.block != nil {
		key = r.block.Prefix + `_` + key
	}

	return key
}

func (r *row) Unmarshal(str string) error {
	key, value, comment, err := parseLine(str)
	if err != nil {
		return err
	}
	r.Key = key
	r.Value = value
	r.Comment = comment

	return nil
}

func (r *row) SetComment(str string) *row {
	r.Comment = str

	return r
}
func (r *row) Commented() *row {
	r.commented = true

	return r
}

func (r *row) Merge(rowToMerge row) {
	r.Value = rowToMerge.Value
	r.AddShadows(rowToMerge.shadows)

	if r.Comment == `` && rowToMerge.Comment != `` {
		r.Comment = rowToMerge.Comment
	}
}

func (r *row) HasShadows(shadow string) bool {
	for _, s := range r.shadows {
		if s == shadow {
			return true
		}
	}
	return false
}

func (r *row) AddShadow(shadow string) *row {
	if !r.HasShadows(shadow) {
		r.shadows = append(r.shadows, shadow)
	}

	return r
}

func (r *row) AddShadows(shadows []string) *row {
	for _, v := range shadows {
		r.AddShadow(v)
	}

	return r
}

func NewRow(key, value string) *row {
	return &row{
		Key: normalizeKey(key), Value: value,
	}
}

func normalizeKey(key string) string {
	key = strings.ToUpper(key)

	re := regexp.MustCompile("[^\\w.]")
	key = re.ReplaceAllString(key, "_")
	key = removeAdjacentDuplicatesOnly(key, `_`)

	return key
}

var (
	singleQuotesRegex = regexp.MustCompile(`\A'(.*)'\z`)
	doubleQuotesRegex = regexp.MustCompile(`\A"(.*)"\z`)
)

func normalizeValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) == 0 {
		return ``
	}

	singleQuotes := singleQuotesRegex.FindStringSubmatch(v)
	doubleQuotes := doubleQuotesRegex.FindStringSubmatch(v)

	if singleQuotes != nil || doubleQuotes != nil {
		v = v[1 : len(v)-1]
	} else {
		if v == `true` || v == `false` {
			return v
		}
		if d, err := strconv.Atoi(v); err == nil {
			return fmt.Sprintf(`%d`, d)
		}
	}

	return fmt.Sprintf(`"%s"`, doubleQuoteEscape(v))
}

const doubleQuoteSpecialChars = "\\\n\r\"$`"

func doubleQuoteEscape(line string) string {
	for _, c := range doubleQuoteSpecialChars {
		toReplace := "\\" + string(c)
		if c == '\n' {
			toReplace = `\n`
		}
		if c == '\r' {
			toReplace = `\r`
		}
		line = strings.Replace(line, string(c), toReplace, -1)
	}
	return line
}

func mergeRowMap(origin, adding map[string]*row) {
	for k, aR := range adding {
		if r, ok := origin[k]; ok {
			r.Merge(*aR)
		} else {
			origin[aR.Key] = aR
		}
	}
}
